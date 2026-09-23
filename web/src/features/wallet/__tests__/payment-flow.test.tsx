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
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  afterAll,
  afterEach,
  beforeAll,
  beforeEach,
  expect,
  test,
  vi,
} from 'vitest'

import { api } from '@/lib/api'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { Wallet } from '..'
import type { AmountRequest } from '../types'

const originalGetAnimations = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'getAnimations'
)
let queryClient: QueryClient

beforeAll(() => {
  Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
})

afterAll(() => {
  if (originalGetAnimations) {
    Object.defineProperty(
      HTMLElement.prototype,
      'getAnimations',
      originalGetAnimations
    )
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, 'getAnimations')
  }
})

beforeEach(() => {
  localStorage.clear()
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    const fixtures: Record<string, unknown> = {
      '/api/status': { price: 1, quota_display_type: 'USD' },
      '/api/user/self': { quota: 0, used_quota: 0, request_count: 0 },
      '/api/user/aff': 'wallet-test',
      '/api/subscription/plans': [],
      '/api/subscription/self': {
        subscriptions: [],
        all_subscriptions: [],
      },
      '/api/user/topup/self?p=1&page_size=10': { items: [], total: 0 },
      '/api/user/topup/info': {
        enable_online_topup: true,
        min_topup: 1,
        pay_methods: [{ type: 'alipay', name: 'Alipay' }],
        amount_options: [100, 1000],
      },
    }
    if (!(url in fixtures)) throw new Error(`Unexpected GET ${url}`)
    return { data: { success: true, data: fixtures[url] } }
  })
})

afterEach(() => {
  cleanup()
  queryClient.clear()
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  localStorage.clear()
})

async function renderWallet() {
  const rootRoute = createRootRoute({ component: Wallet })
  const router = createRouter({
    routeTree: rootRoute,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  await screen.findByRole('spinbutton', { name: 'Custom Amount' })
}

test('a rejected quote shows its server reason without opening payment confirmation or creating an order', async () => {
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: { message: 'error', data: 'top-up quota limit exceeded' },
  })
  const user = userEvent.setup()
  await renderWallet()

  const amount = screen.getByRole('spinbutton', { name: 'Custom Amount' })
  await user.clear(amount)
  await user.type(amount, '1000')
  await user.click(screen.getByRole('button', { name: 'Alipay' }))

  expect(await screen.findByRole('alert')).toHaveTextContent(
    'This top-up exceeds the wallet balance limit. Reduce the amount or contact the administrator.'
  )
  expect(
    screen.queryByRole('alertdialog', { name: 'Confirm Payment' })
  ).not.toBeInTheDocument()
  expect(post.mock.calls.some(([url]) => url === '/api/user/pay')).toBe(false)
})

test('changing the amount after a rejected quote restores confirmation and submits a large quoted payment', async () => {
  const post = vi.spyOn(api, 'post').mockImplementation(async (url, body) => {
    if (url === '/api/user/amount') {
      return {
        data:
          (body as AmountRequest).amount === 30000
            ? { message: 'success', data: '30000.00' }
            : { message: 'error', data: 'top-up quota limit exceeded' },
      }
    }
    if (url === '/api/user/pay') {
      return {
        data: {
          message: 'success',
          url: 'https://payments.example.com/checkout',
          data: {
            out_trade_no: 'wallet-order-30000',
            money: '30000.00',
            type: 'alipay',
          },
        },
      }
    }
    throw new Error(`Unexpected POST ${url}`)
  })
  const submissions: {
    action: string
    method: string
    fields: Record<string, FormDataEntryValue>
  }[] = []
  vi.spyOn(HTMLFormElement.prototype, 'submit').mockImplementation(
    function (this: HTMLFormElement) {
      submissions.push({
        action: this.action,
        method: this.method,
        fields: Object.fromEntries(new FormData(this)),
      })
    }
  )
  const user = userEvent.setup()
  await renderWallet()
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'This top-up exceeds the wallet balance limit. Reduce the amount or contact the administrator.'
  )

  const amount = screen.getByRole('spinbutton', { name: 'Custom Amount' })
  await user.clear(amount)
  await user.type(amount, '30000')
  await waitFor(() =>
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  )
  await user.click(screen.getByRole('button', { name: 'Alipay' }))

  const confirmation = await screen.findByRole('alertdialog', {
    name: 'Confirm Payment',
  })
  expect(within(confirmation).getByText('30,000')).toBeInTheDocument()
  await user.click(
    within(confirmation).getByRole('button', { name: 'Confirm Payment' })
  )

  await waitFor(() =>
    expect(submissions).toEqual([
      {
        action: 'https://payments.example.com/checkout',
        method: 'post',
        fields: {
          out_trade_no: 'wallet-order-30000',
          money: '30000.00',
          type: 'alipay',
        },
      },
    ])
  )
  expect(post).toHaveBeenCalledWith(
    '/api/user/pay',
    { amount: 30000, payment_method: 'alipay' },
    expect.anything()
  )
  await waitFor(() =>
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  )
})
