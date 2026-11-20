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
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AxiosError, CanceledError, type AxiosAdapter } from 'axios'
import { toast } from 'sonner'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { RewardClaimDialog } from '../components/reward-claim-dialog'
import type { UserReward } from '../types'

const reward: UserReward = {
  id: 21,
  campaign_id: 3,
  user_id: 10,
  title: 'A thank-you reward',
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
const originalAdapter = api.defaults.adapter
let client: QueryClient

beforeEach(() => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 10, username: 'recipient', role: 1, quota: 2000000 })
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  api.defaults.adapter = originalAdapter
  useAuthStore.getState().auth.reset()
  vi.restoreAllMocks()
})

function renderClaim() {
  render(
    <QueryClientProvider client={client}>
      <RewardClaimDialog
        reward={reward}
        onDismiss={() => {}}
        onComplete={() => {}}
      />
    </QueryClientProvider>
  )
}

test.each([
  { failure: 'business rejection', message: 'Reward claim rejected' },
  { failure: 'network error', message: 'Network Error' },
])(
  '$failure reports once, keeps the inline reason and allows retry',
  async ({ failure, message }) => {
    const user = userEvent.setup()
    const notify = vi.spyOn(toast, 'error').mockImplementation(() => 0)
    const request = vi.fn<AxiosAdapter>(async (config) => ({
      config,
      status: 200,
      statusText: 'OK',
      headers: {},
      data: {
        success: true,
        data: {
          reward: { ...reward, status: 'claimed', quota_awarded: 500000 },
          quota: 2500000,
        },
      },
    }))
    request.mockImplementationOnce(async (config) => {
      if (failure === 'network error') {
        throw new AxiosError(message, AxiosError.ERR_NETWORK, config)
      }
      return {
        config,
        status: 200,
        statusText: 'OK',
        headers: {},
        data: { success: false, message },
      }
    })
    api.defaults.adapter = request
    renderClaim()

    const claim = screen.getByRole('button', { name: 'Claim reward' })
    await user.click(claim)
    expect(await screen.findByRole('alert')).toHaveTextContent(message)
    expect(notify).toHaveBeenCalledWith(message)
    expect(notify).toHaveBeenCalledTimes(1)
    expect(claim).toBeEnabled()
    expect(useAuthStore.getState().auth.user?.quota).toBe(2000000)

    await user.click(claim)
    expect(
      await screen.findByText('Added to your account balance')
    ).toBeVisible()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(request).toHaveBeenCalledTimes(2)
    expect(request.mock.calls[1][0]).toMatchObject({
      url: '/api/rewards/21/claim',
      method: 'post',
    })
    expect(useAuthStore.getState().auth.user?.quota).toBe(2500000)
    expect(notify).toHaveBeenCalledTimes(1)
  }
)

test('a canceled claim stays unclaimed without an error toast', async () => {
  const user = userEvent.setup()
  const notify = vi.spyOn(toast, 'error').mockImplementation(() => 0)
  api.defaults.adapter = vi.fn<AxiosAdapter>(async (config) => {
    throw new CanceledError('Authentication session changed', config)
  })
  renderClaim()

  await user.click(screen.getByRole('button', { name: 'Claim reward' }))
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Authentication session changed'
  )
  expect(notify).not.toHaveBeenCalled()
  expect(screen.getByRole('button', { name: 'Claim reward' })).toBeEnabled()
  expect(useAuthStore.getState().auth.user?.quota).toBe(2000000)
})
