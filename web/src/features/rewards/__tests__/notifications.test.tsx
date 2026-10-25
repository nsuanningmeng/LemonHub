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
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { RewardsNotification } from '../components/rewards-notification'
import { useRewardDialogStore } from '../lib/reward-dialog-store'
import type { UserReward } from '../types'

const luckyReward: UserReward = {
  id: 21,
  campaign_id: 3,
  user_id: 10,
  title: 'Autumn thanks',
  description: 'A gift for your next idea',
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
let pending: UserReward[]

beforeEach(() => {
  sessionStorage.clear()
  useRewardDialogStore.getState().clearReward()
  useAuthStore
    .getState()
    .auth.setUser({ id: 10, username: 'recipient', role: 1, quota: 2000000 })
  pending = [{ ...luckyReward }]
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url !== '/api/rewards') throw new Error(`Unexpected GET ${url}`)
    const items = pending.filter(
      (reward) => reward.user_id === useAuthStore.getState().auth.user?.id
    )
    return { data: { success: true, data: { items, total: items.length } } }
  })
})

afterEach(() => {
  cleanup()
  queryClient.clear()
  useAuthStore.getState().auth.reset()
  useRewardDialogStore.getState().clearReward()
  sessionStorage.clear()
})

function renderNotifications() {
  return render(
    <QueryClientProvider client={queryClient}>
      <RewardsNotification />
    </QueryClientProvider>
  )
}

describe('reward notifications', () => {
  it('reveals the server amount after opening a red packet and refreshes the wallet', async () => {
    const user = userEvent.setup()
    vi.spyOn(api, 'post').mockImplementation(async () => {
      pending = []
      return {
        data: {
          success: true,
          data: {
            reward: {
              ...luckyReward,
              status: 'claimed',
              quota_awarded: 750000,
              claimed_at: 1790001000,
            },
            quota: 2750000,
          },
        },
      }
    })
    renderNotifications()
    const dialog = await screen.findByRole('dialog', {
      name: 'You received a red packet',
    })
    expect(
      within(dialog).getByText('Your lucky amount will be revealed when opened')
    ).toBeVisible()
    expect(within(dialog).queryByText(/\$/)).not.toBeInTheDocument()
    await user.click(
      within(dialog).getByRole('button', { name: 'Open red packet' })
    )
    expect(
      await screen.findByText('Added to your account balance')
    ).toBeVisible()
    expect(screen.getByText('+$1.5')).toBeVisible()
    expect(useAuthStore.getState().auth.user?.quota).toBe(2750000)
    expect(api.post).toHaveBeenCalledWith('/api/rewards/21/claim')
    await user.click(screen.getByRole('button', { name: 'Done' }))
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
  })

  it('defers the current batch for this session and opens newly received rewards', async () => {
    const user = userEvent.setup()
    pending.push({ ...luckyReward, id: 20, title: 'Earlier gift' })
    const view = renderNotifications()
    await user.click(await screen.findByRole('button', { name: 'Claim later' }))
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    view.unmount()
    renderNotifications()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    pending.unshift({ ...luckyReward, id: 22, title: 'A fresh gift' })
    await act(async () => {
      await queryClient.invalidateQueries({ queryKey: ['rewards', 10] })
    })
    const dialog = await screen.findByRole('dialog', {
      name: 'You received a red packet',
    })
    expect(within(dialog).getByText('A fresh gift')).toBeVisible()
    expect(within(dialog).queryByText('Earlier gift')).not.toBeInTheDocument()
  })

  it('keeps the reward unclaimed after failure and allows retry without duplicate requests', async () => {
    const user = userEvent.setup()
    let completeRequest:
      | ((value: { data: { success: boolean; message: string } }) => void)
      | undefined
    const post = vi.spyOn(api, 'post').mockImplementation(
      () =>
        new Promise((resolve) => {
          completeRequest = resolve
        })
    )
    vi.spyOn(console, 'log').mockImplementation(() => {})
    renderNotifications()
    const openButton = await screen.findByRole('button', {
      name: 'Open red packet',
    })
    await user.click(openButton)
    expect(openButton).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Claim later' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Close' })).toBeDisabled()
    await user.click(openButton)
    expect(post).toHaveBeenCalledTimes(1)
    await act(async () => {
      completeRequest?.({
        data: {
          success: false,
          message: 'Unable to claim reward. Please try again.',
        },
      })
    })
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Unable to claim reward. Please try again.'
    )
    expect(openButton).toBeEnabled()
    expect(useAuthStore.getState().auth.user?.quota).toBe(2000000)
    post.mockResolvedValueOnce({
      data: {
        success: true,
        data: {
          reward: {
            ...luckyReward,
            status: 'claimed',
            quota_awarded: 600000,
            claimed_at: 1790001000,
          },
          quota: 2600000,
        },
      },
    })
    await user.click(openButton)
    expect(
      await screen.findByText('Added to your account balance')
    ).toBeVisible()
  })

  it('removes the previous account dialog and ignores its delayed wallet response', async () => {
    const user = userEvent.setup()
    let finish:
      | ((value: {
          data: {
            success: boolean
            data: { reward: UserReward; quota: number }
          }
        }) => void)
      | undefined
    vi.spyOn(api, 'post').mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve
        })
    )
    renderNotifications()
    await user.click(
      await screen.findByRole('button', { name: 'Open red packet' })
    )
    act(() => {
      useAuthStore
        .getState()
        .auth.setUser({ id: 11, username: 'another', role: 1, quota: 9000000 })
    })
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    await act(async () => {
      finish?.({
        data: {
          success: true,
          data: {
            reward: {
              ...luckyReward,
              status: 'claimed',
              quota_awarded: 750000,
            },
            quota: 2750000,
          },
        },
      })
    })
    expect(useAuthStore.getState().auth.user?.id).toBe(11)
    expect(useAuthStore.getState().auth.user?.quota).toBe(9000000)
    expect(screen.queryByText('Autumn thanks')).not.toBeInTheDocument()
  })
})
