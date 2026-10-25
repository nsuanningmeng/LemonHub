/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { PendingRewards } from '../components/pending-rewards'
import { RewardClaimDialog } from '../components/reward-claim-dialog'
import { RewardHistory } from '../components/reward-history'
import { RewardSummaryCards } from '../components/reward-summary'
import { RewardsNotification } from '../components/rewards-notification'
import { useRewardDialogStore } from '../lib/reward-dialog-store'
import type { RewardListParams, UserReward } from '../types'

const reward: UserReward = {
  id: 21,
  campaign_id: 3,
  user_id: 10,
  title: 'A timely gift',
  description: '',
  type: 'random',
  min_quota: 500000,
  max_quota: 1500000,
  quota_awarded: 0,
  created_at: 1790000000,
  claimed_at: 0,
  expires_at: 0,
  status: 'pending',
}
let queryClient: QueryClient

beforeEach(() => {
  sessionStorage.clear()
  useRewardDialogStore.getState().clearReward()
  useAuthStore
    .getState()
    .auth.setUser({ id: 10, username: 'recipient', role: 1, quota: 2000000 })
  queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: Infinity },
      mutations: { retry: false },
    },
  })
})

afterEach(() => {
  cleanup()
  queryClient.clear()
  useAuthStore.getState().auth.reset()
  useRewardDialogStore.getState().clearReward()
  sessionStorage.clear()
  vi.useRealTimers()
})

function renderClaim(item: UserReward) {
  return render(
    <QueryClientProvider client={queryClient}>
      <RewardClaimDialog
        reward={item}
        onDismiss={() => {}}
        onComplete={() => {}}
      />
    </QueryClientProvider>
  )
}

describe('reward expiry', () => {
  it('shows a subtle local deadline and disables an open red packet exactly when it expires', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date(2026, 9, 25, 10, 0))
    const expiresAt = Math.floor(new Date(2026, 9, 25, 10, 1).getTime() / 1000)
    const post = vi.spyOn(api, 'post')
    renderClaim({ ...reward, expires_at: expiresAt })
    const deadline = screen.getByText('Expires at 2026-10-25 10:01')
    expect(deadline).toBeVisible()
    expect(deadline).toHaveClass('text-xs')
    expect(
      screen.getByRole('button', { name: 'Open red packet' })
    ).toBeEnabled()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(60_000)
    })
    expect(screen.getByRole('dialog', { name: 'Reward expired' })).toBeVisible()
    const expiredButton = screen.getByRole('button', { name: 'Expired' })
    expect(expiredButton).toBeDisabled()
    fireEvent.click(expiredButton)
    expect(post).not.toHaveBeenCalled()
    expect(
      screen.getByText('This reward has expired and can no longer be claimed.')
    ).toBeVisible()
  })

  it('keeps long deadlines valid past the browser timer limit and expires at the actual deadline', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date(2026, 9, 25, 10, 0))
    const duration = 50 * 24 * 60 * 60 * 1000
    renderClaim({ ...reward, expires_at: (Date.now() + duration) / 1000 })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2_147_483_647)
    })
    expect(
      screen.getByRole('button', { name: 'Open red packet' })
    ).toBeEnabled()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(duration - 2_147_483_647)
    })
    expect(screen.getByRole('button', { name: 'Expired' })).toBeDisabled()
  })

  it('preserves legacy rewards without a deadline', async () => {
    vi.useFakeTimers()
    renderClaim(reward)
    expect(screen.queryByText(/^Expires at /)).not.toBeInTheDocument()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(24 * 60 * 60 * 1000)
    })
    expect(
      screen.getByRole('button', { name: 'Open red packet' })
    ).toBeEnabled()
  })

  it('keeps a successful claim visible after its original deadline passes', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date(2026, 9, 25, 10, 0))
    const item = { ...reward, expires_at: Date.now() / 1000 + 60 }
    vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        success: true,
        data: {
          reward: {
            ...item,
            status: 'claimed',
            quota_awarded: 750000,
            claimed_at: Date.now() / 1000,
          },
          quota: 2750000,
        },
      },
    })
    renderClaim(item)
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Open red packet' }))
      await vi.advanceTimersByTimeAsync(0)
    })
    expect(screen.getByText('Added to your account balance')).toBeVisible()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(60_000)
    })
    expect(screen.getByRole('dialog', { name: 'Reward claimed' })).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Expired' })
    ).not.toBeInTheDocument()
  })

  it('refreshes pending lists and summary when the server rejects an expired reward despite a future client deadline', async () => {
    const user = userEvent.setup()
    const item = { ...reward, expires_at: Math.floor(Date.now() / 1000) + 3600 }
    let serverExpired = false
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (url === '/api/rewards/summary') {
        return {
          data: {
            success: true,
            data: {
              pending_count: serverExpired ? 0 : 1,
              fixed_count: 0,
              random_count: serverExpired ? 0 : 1,
              fixed_quota: 0,
            },
          },
        }
      }
      const items = serverExpired ? [] : [item]
      return { data: { success: true, data: { items, total: items.length } } }
    })
    vi.spyOn(api, 'post').mockImplementation(async () => {
      serverExpired = true
      return { data: { success: false, message: 'reward expired' } }
    })
    render(
      <QueryClientProvider client={queryClient}>
        <RewardSummaryCards />
        <PendingRewards type='random' />
        <RewardsNotification />
      </QueryClientProvider>
    )
    const dialog = await screen.findByRole('dialog', {
      name: 'You received a red packet',
    })
    await user.click(
      within(dialog).getByRole('button', { name: 'Open red packet' })
    )
    expect(
      await screen.findByRole('dialog', { name: 'Reward expired' })
    ).toBeVisible()
    expect(screen.getByRole('button', { name: 'Expired' })).toBeDisabled()
    await user.keyboard('{Escape}')
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    expect(
      await screen.findByText('No red packets waiting to be opened')
    ).toBeVisible()
    expect(
      within(screen.getByRole('region', { name: 'My red packets' })).getByText(
        '0'
      )
    ).toBeVisible()
    expect(useAuthStore.getState().auth.user?.quota).toBe(2000000)
  })

  it('skips expired notifications left in a fresh cache and opens the next valid reward', async () => {
    queryClient.setQueryData(['rewards', 10, 'notifications'], {
      items: [
        {
          ...reward,
          title: 'Expired cached gift',
          expires_at: Math.floor(Date.now() / 1000) - 1,
        },
        {
          ...reward,
          id: 22,
          title: 'Current gift',
          expires_at: Math.floor(Date.now() / 1000) + 3600,
        },
      ],
      total: 2,
    })
    render(
      <QueryClientProvider client={queryClient}>
        <RewardsNotification />
      </QueryClientProvider>
    )
    const dialog = await screen.findByRole('dialog', {
      name: 'You received a red packet',
    })
    expect(within(dialog).getByText('Current gift')).toBeVisible()
    expect(
      within(dialog).queryByText('Expired cached gift')
    ).not.toBeInTheDocument()
  })

  it('lists expired rewards with their deadline and no credited amount while preserving claimed history', async () => {
    const user = userEvent.setup()
    const expiresAt = new Date(2026, 9, 25, 10, 1).getTime() / 1000
    vi.spyOn(api, 'get').mockImplementation(async (_url, config) => {
      const request = config as { params: RewardListParams } | undefined
      const expired = request?.params.status === 'expired'
      const item = {
        ...reward,
        title: expired ? 'Missed gift' : 'Claimed gift',
        expires_at: expiresAt,
        status: expired ? 'expired' : 'claimed',
        claimed_at: expiresAt - 60,
        quota_awarded: expired ? 0 : 750000,
      }
      return { data: { success: true, data: { items: [item], total: 1 } } }
    })
    render(
      <QueryClientProvider client={queryClient}>
        <RewardHistory />
      </QueryClientProvider>
    )
    expect(await screen.findByText('Claimed gift')).toBeVisible()
    expect(screen.getByText('+$1.5')).toBeVisible()
    await user.click(screen.getByRole('tab', { name: 'Expired' }))
    expect(await screen.findByText('Missed gift')).toBeVisible()
    expect(screen.getByText('2026-10-25 10:01')).toBeVisible()
    expect(screen.getByText('—')).toBeVisible()
    expect(screen.queryByText(/\+\$/)).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Open red packet' })
    ).not.toBeInTheDocument()
    await user.click(screen.getByRole('tab', { name: 'Claimed' }))
    expect(await screen.findByText('Claimed gift')).toBeVisible()
    expect(screen.getByText('+$1.5')).toBeVisible()
  })
})
