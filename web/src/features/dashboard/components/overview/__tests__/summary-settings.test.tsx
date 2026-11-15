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
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import {
  act,
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AxiosError } from 'axios'
import { toast } from 'sonner'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { SummaryCards } from '../summary-cards'

const originalAdapter = api.defaults.adapter
let client: QueryClient

beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  client.setQueryData(['status'], {})
  useSystemConfigStore
    .getState()
    .setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG } })
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'member',
    role: 1,
    quota: 5000000,
    used_quota: 1000000,
    request_count: 42,
  })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: [
        { created_at: Math.floor(Date.now() / 1000), quota: 500000, count: 3 },
      ],
    },
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  api.defaults.adapter = originalAdapter
  useAuthStore.getState().auth.reset()
  useSystemConfigStore
    .getState()
    .setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG } })
})

function renderSummary() {
  const router = createRouter({
    routeTree: createRootRoute({ component: SummaryCards }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

test('changing quota display currency updates historical totals without changing account data', async () => {
  renderSummary()
  const historical = await screen.findByRole('article', {
    name: 'Historical Usage',
  })
  await waitFor(() => expect(within(historical).getByText('$2')).toBeVisible())
  await act(async () => {
    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: 'CNY',
        usdExchangeRate: 7,
      },
    })
  })
  expect(within(historical).getByText('¥14')).toBeVisible()
  expect(screen.queryByText('$2')).not.toBeInTheDocument()
  await act(async () => {
    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: 'TOKENS',
        displayInCurrency: false,
      },
    })
  })
  expect(within(historical).queryByText(/¥|\$/)).not.toBeInTheDocument()
  expect(within(historical).getByText('1000k')).toBeVisible()
})

test('failed usage requests do not claim zero usage and a healthy runway', async () => {
  vi.mocked(api.get).mockRejectedValue(
    new Error('Service temporarily unavailable')
  )
  renderSummary()
  expect(await screen.findByText('Failed to fetch usage')).toBeVisible()
  expect(screen.queryByText('Healthy')).not.toBeInTheDocument()
  expect(screen.queryByText('No recent usage')).not.toBeInTheDocument()
  expect(screen.getByText('$10')).toBeVisible()
})

test('an HTTP 500 keeps summary recovery local without replacing the workspace or sending duplicate notifications', async () => {
  vi.mocked(api.get).mockRestore()
  const notify = vi.spyOn(toast, 'error')
  let recovered = false
  let failure: AxiosError | undefined
  api.defaults.adapter = async (config) => {
    if (!recovered) {
      failure = new AxiosError(
        'Unavailable',
        'ERR_BAD_RESPONSE',
        config,
        undefined,
        {
          status: 500,
          statusText: 'Server error',
          headers: {},
          config,
          data: { message: 'Unavailable' },
        }
      )
      throw failure
    }
    return {
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
      data: { success: true, data: [] },
    }
  }
  const user = userEvent.setup()
  renderSummary()
  expect(await screen.findByText('Failed to fetch usage')).toBeVisible()
  const queryError = client.getQueryCache().find({
    queryKey: ['dashboard', 'overview', 'summary-sparklines'],
    exact: false,
  })?.state.error
  expect(queryError).not.toBeInstanceOf(AxiosError)
  expect(queryError?.cause).toBe(failure)
  expect(notify).not.toHaveBeenCalled()
  recovered = true
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  expect(await screen.findByText('No recent usage')).toBeVisible()
  expect(notify).not.toHaveBeenCalled()
})
