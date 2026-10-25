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
import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { PendingRewards } from '../components/pending-rewards'
import { RewardsNotification } from '../components/rewards-notification'
import { Rewards } from '../index'
import { useRewardDialogStore } from '../lib/reward-dialog-store'
import type { RewardListParams, UserReward } from '../types'

const reward: UserReward = {
  id: 1,
  campaign_id: 1,
  user_id: 10,
  title: 'Recharge thank-you',
  description: '',
  type: 'fixed',
  min_quota: 500000,
  max_quota: 500000,
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
    defaultOptions: { queries: { retry: false } },
  })
})

afterEach(() => {
  cleanup()
  queryClient.clear()
  useAuthStore.getState().auth.reset()
  useRewardDialogStore.getState().clearReward()
  sessionStorage.clear()
})

describe('my rewards page', () => {
  it('shows category-specific empty states and allows keyboard tab selection', async () => {
    const user = userEvent.setup()
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (url === '/api/rewards/summary') {
        return {
          data: {
            success: true,
            data: {
              pending_count: 0,
              fixed_count: 0,
              random_count: 0,
              fixed_quota: 0,
            },
          },
        }
      }
      return { data: { success: true, data: { items: [], total: 0 } } }
    })
    render(
      <QueryClientProvider client={queryClient}>
        <Rewards />
      </QueryClientProvider>
    )
    expect(
      await screen.findByText('No reward credits waiting to be claimed')
    ).toBeVisible()
    expect(await screen.findByText('No rewards claimed yet')).toBeVisible()
    const credits = screen.getByRole('tab', { name: 'Reward credits' })
    await user.click(credits)
    await user.keyboard('{ArrowRight}{Enter}')
    expect(screen.getByRole('tab', { name: 'Red packets' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    expect(
      await screen.findByText('No red packets waiting to be opened')
    ).toBeVisible()
  })

  it('allows reopening a deferred fixed reward from its card and shows its amount before claiming', async () => {
    const user = userEvent.setup()
    sessionStorage.setItem('rewards:dismissed:10', '[1]')
    vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
      if (url === '/api/rewards/summary') {
        return {
          data: {
            success: true,
            data: {
              pending_count: 1,
              fixed_count: 1,
              random_count: 0,
              fixed_quota: 500000,
            },
          },
        }
      }
      const request = config as { params: RewardListParams } | undefined
      const items = request?.params.status === 'claimed' ? [] : [reward]
      return { data: { success: true, data: { items, total: items.length } } }
    })
    render(
      <QueryClientProvider client={queryClient}>
        <Rewards />
        <RewardsNotification />
      </QueryClientProvider>
    )
    const card = await screen.findByRole('article')
    expect(within(card).getByText('$1')).toBeVisible()
    await user.click(within(card).getByRole('button', { name: 'Claim reward' }))
    const dialog = await screen.findByRole('dialog', {
      name: 'You received a reward',
    })
    expect(within(dialog).getByText('$1')).toBeVisible()
    expect(
      within(dialog).queryByText(
        'Your lucky amount will be revealed when opened'
      )
    ).not.toBeInTheDocument()
    await user.keyboard('{Escape}')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('loads the next claim-history page and disables next at the end', async () => {
    const user = userEvent.setup()
    vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
      if (url === '/api/rewards/summary') {
        return {
          data: {
            success: true,
            data: {
              pending_count: 0,
              fixed_count: 0,
              random_count: 0,
              fixed_quota: 0,
            },
          },
        }
      }
      const request = config as { params: RewardListParams } | undefined
      if (request?.params.status !== 'claimed') {
        return { data: { success: true, data: { items: [], total: 0 } } }
      }
      const title =
        request.params.p === 2 ? 'Second page reward' : 'First page reward'
      return {
        data: {
          success: true,
          data: {
            items: [
              {
                ...reward,
                title,
                status: 'claimed',
                quota_awarded: 500000,
                claimed_at: 1790002000,
              },
            ],
            total: 11,
          },
        },
      }
    })
    render(
      <QueryClientProvider client={queryClient}>
        <Rewards />
      </QueryClientProvider>
    )
    expect(await screen.findByText('First page reward')).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Next' }))
    expect(await screen.findByText('Second page reward')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Previous' })).toBeEnabled()
  })

  it('returns to the remaining page after claiming the last reward on page two', async () => {
    const user = userEvent.setup()
    const finalReward = { ...reward, id: 13, title: 'Final page reward' }
    let pending = [
      ...Array.from({ length: 12 }, (_, index) => ({
        ...reward,
        id: index + 1,
        title: `Remaining reward ${index + 1}`,
      })),
      finalReward,
    ]
    sessionStorage.setItem(
      'rewards:dismissed:10',
      JSON.stringify(pending.map((item) => item.id))
    )
    vi.spyOn(api, 'get').mockImplementation(async (_url, config) => {
      const request = config as { params: RewardListParams } | undefined
      const page = Number(request?.params.p)
      const pageSize = Number(request?.params.page_size)
      const items = pending.slice((page - 1) * pageSize, page * pageSize)
      return { data: { success: true, data: { items, total: pending.length } } }
    })
    vi.spyOn(api, 'post').mockImplementation(async () => {
      pending = pending.filter((item) => item.id !== finalReward.id)
      return {
        data: {
          success: true,
          data: {
            reward: {
              ...finalReward,
              status: 'claimed',
              quota_awarded: 500000,
              claimed_at: 1790001000,
            },
            quota: 2500000,
          },
        },
      }
    })
    render(
      <QueryClientProvider client={queryClient}>
        <PendingRewards type='fixed' />
        <RewardsNotification />
      </QueryClientProvider>
    )
    expect(await screen.findByText('Remaining reward 1')).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Next' }))
    expect(await screen.findByText('Final page reward')).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Claim reward' }))
    const dialog = await screen.findByRole('dialog', {
      name: 'You received a reward',
    })
    await user.click(
      within(dialog).getByRole('button', { name: 'Claim reward' })
    )
    await user.click(await screen.findByRole('button', { name: 'Done' }))
    expect(await screen.findByText('Remaining reward 1')).toBeVisible()
    expect(screen.getAllByRole('article')).toHaveLength(12)
    expect(
      screen.queryByText('No reward credits waiting to be claimed')
    ).not.toBeInTheDocument()
    expect(screen.queryByText('Page 2 of 1')).not.toBeInTheDocument()
  })
})
