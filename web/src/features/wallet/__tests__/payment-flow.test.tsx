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
import { AxiosError } from 'axios'
import { toast } from 'sonner'
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
const originalAdapter = api.defaults.adapter
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
  api.defaults.adapter = originalAdapter
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  localStorage.clear()
})

async function renderWallet(waitForAmount = true) {
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
  if (waitForAmount) {
    await screen.findByRole('spinbutton', { name: 'Custom Amount' })
  }
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

test('the account balance action moves keyboard focus to funding without creating a payment', async () => {
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: { message: 'success', data: '1.00' },
  })
  const user = userEvent.setup()
  await renderWallet()

  const balance = screen.getByRole('region', { name: 'Current Balance' })
  await user.click(within(balance).getByRole('button', { name: 'Add Funds' }))
  expect(
    screen.getByRole('spinbutton', { name: 'Custom Amount' })
  ).toHaveFocus()
  expect(post.mock.calls.some(([url]) => url === '/api/user/pay')).toBe(false)
})

test('order history is available from the wallet toolbar while absent subscription plans leave no empty panel', async () => {
  vi.spyOn(api, 'post').mockResolvedValue({
    data: { message: 'success', data: '1.00' },
  })
  const user = userEvent.setup()
  await renderWallet()

  await waitFor(() =>
    expect(
      screen.queryByRole('region', { name: 'Subscription Plans' })
    ).not.toBeInTheDocument()
  )
  await user.click(screen.getByRole('button', { name: 'Order History' }))
  expect(await screen.findByRole('dialog')).toBeVisible()
})

test('disabled Epay methods do not appear or receive quotes when Stripe remains enabled', async () => {
  const originalGet = vi.mocked(api.get).getMockImplementation()
  if (!originalGet) throw new Error('Missing API fixture')
  vi.mocked(api.get).mockImplementation(async (url, config) => {
    if (url === '/api/user/topup/info') {
      return {
        data: {
          success: true,
          data: {
            enable_online_topup: false,
            enable_stripe_topup: true,
            stripe_min_topup: 5,
            min_topup: 1,
            pay_methods: [
              { type: 'alipay', name: 'Alipay' },
              { type: 'stripe', name: 'Stripe' },
            ],
            amount_options: [5, 20],
          },
        },
      }
    }
    return originalGet(url, config)
  })
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { message: 'success', data: '5.00' } })
  await renderWallet()
  expect(screen.getByRole('button', { name: 'Stripe' })).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'Alipay' })
  ).not.toBeInTheDocument()
  await waitFor(() => expect(post).toHaveBeenCalled())
  expect(
    post.mock.calls.every(([url]) => url === '/api/user/stripe/amount')
  ).toBe(true)
})

test('an unavailable wallet balance is not presented as zero and can be retried', async () => {
  const originalGet = vi.mocked(api.get).getMockImplementation()
  if (!originalGet) throw new Error('Missing API fixture')
  vi.mocked(api.get).mockImplementation(async (url, config) => {
    if (url === '/api/user/self') {
      return { data: { success: false, message: 'Unavailable' } }
    }
    return originalGet(url, config)
  })
  vi.spyOn(api, 'post').mockResolvedValue({
    data: { message: 'success', data: '1.00' },
  })
  const user = userEvent.setup()
  await renderWallet()
  const balance = screen.getByRole('region', { name: 'Current Balance' })
  expect(within(balance).getByText('Failed to load')).toBeVisible()
  expect(within(balance).queryByText('$0')).not.toBeInTheDocument()
  expect(
    within(balance).queryByText('Ready for your next request.')
  ).not.toBeInTheDocument()
  vi.mocked(api.get).mockImplementation(originalGet)
  await user.click(within(balance).getByRole('button', { name: 'Retry' }))
  await waitFor(() =>
    expect(within(balance).getAllByText('$0')).toHaveLength(2)
  )
  expect(within(balance).queryByText('Failed to load')).not.toBeInTheDocument()
})

test('failed recharge configuration stays an error and restores payment methods after retry', async () => {
  const originalGet = vi.mocked(api.get).getMockImplementation()
  if (!originalGet) throw new Error('Missing API fixture')
  vi.mocked(api.get).mockImplementation(async (url, config) => {
    if (url === '/api/user/topup/info') throw new Error('Unavailable')
    return originalGet(url, config)
  })
  vi.spyOn(console, 'error').mockImplementation(() => undefined)
  vi.spyOn(api, 'post').mockResolvedValue({
    data: { message: 'success', data: '1.00' },
  })
  const user = userEvent.setup()
  await renderWallet(false)
  const recharge = screen.getByRole('region', { name: 'Add Funds' })
  expect(await within(recharge).findByText('Failed to load')).toBeVisible()
  expect(
    within(recharge).queryByText('Online top-up is currently unavailable.')
  ).not.toBeInTheDocument()
  expect(within(recharge).queryByRole('textbox')).not.toBeInTheDocument()
  vi.mocked(api.get).mockImplementation(originalGet)
  await user.click(within(recharge).getByRole('button', { name: 'Retry' }))
  expect(
    await within(recharge).findByRole('button', { name: 'Alipay' })
  ).toBeVisible()
  expect(within(recharge).queryByText('Failed to load')).not.toBeInTheDocument()
})

test.each(['/api/subscription/plans', '/api/subscription/self'])(
  'a failed %s response keeps the subscription error visible until a successful retry',
  async (failedEndpoint) => {
    const originalGet = vi.mocked(api.get).getMockImplementation()
    if (!originalGet) throw new Error('Missing API fixture')
    vi.mocked(api.get).mockImplementation(async (url, config) => {
      if (url === failedEndpoint) {
        return { data: { success: false, message: 'Unavailable' } }
      }
      return originalGet(url, config)
    })
    vi.spyOn(api, 'post').mockResolvedValue({
      data: { message: 'success', data: '1.00' },
    })
    const user = userEvent.setup()
    await renderWallet()
    expect(await screen.findByText('Failed to load')).toBeVisible()
    const subscriptions = screen.getByRole('region', {
      name: 'Subscription Plans',
    })
    expect(
      within(subscriptions).queryByText('No plans available')
    ).not.toBeInTheDocument()
    expect(
      within(subscriptions).queryByText('No Active')
    ).not.toBeInTheDocument()
    vi.mocked(api.get).mockImplementation(originalGet)
    await user.click(
      within(subscriptions).getByRole('button', { name: 'Retry' })
    )
    await waitFor(() =>
      expect(
        screen.queryByRole('region', { name: 'Subscription Plans' })
      ).not.toBeInTheDocument()
    )
  }
)

test.each([
  ['/api/user/self', 'Current Balance'],
  ['/api/user/topup/info', 'Add Funds'],
  ['/api/subscription/plans', 'Subscription Plans'],
  ['/api/subscription/self', 'Subscription Plans'],
])(
  'HTTP 500 from %s stays local and retries without duplicate notifications',
  async (endpoint, regionName) => {
    const fixture = vi.mocked(api.get).getMockImplementation()
    if (!fixture) throw new Error('Missing API fixture')
    vi.mocked(api.get).mockRestore()
    let recovered = false
    api.defaults.adapter = async (config) => {
      if (config.url === endpoint && !recovered) {
        throw new AxiosError(
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
      }
      const response = await fixture(config.url ?? '', config)
      if (!response || typeof response !== 'object' || !('data' in response)) {
        throw new Error('Invalid API fixture response')
      }
      return {
        data: response.data,
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      }
    }
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const notify = vi.spyOn(toast, 'error')
    vi.spyOn(api, 'post').mockResolvedValue({
      data: { message: 'success', data: '1.00' },
    })
    const user = userEvent.setup()
    await renderWallet(false)
    expect(await screen.findByText('Failed to load')).toBeVisible()
    const region = screen.getByRole('region', { name: regionName })
    expect(within(region).getByRole('button', { name: 'Retry' })).toBeVisible()
    expect(notify).not.toHaveBeenCalled()
    recovered = true
    await user.click(within(region).getByRole('button', { name: 'Retry' }))
    await waitFor(() =>
      expect(screen.queryByText('Failed to load')).not.toBeInTheDocument()
    )
    expect(notify).not.toHaveBeenCalled()
  }
)

test('keyboard preset selection and custom entry expose the active amount', async () => {
  vi.spyOn(api, 'post').mockResolvedValue({
    data: { message: 'success', data: '100' },
  })
  const user = userEvent.setup()
  await renderWallet()
  const preset = screen.getByRole('button', { name: /^100 Pay 100$/ })
  expect(preset).toHaveAttribute('aria-pressed', 'false')
  preset.focus()
  await user.keyboard('{Enter}')
  expect(preset).toHaveAttribute('aria-pressed', 'true')
  const amount = screen.getByRole('spinbutton', { name: 'Custom Amount' })
  await user.clear(amount)
  await user.type(amount, '20')
  expect(preset).toHaveAttribute('aria-pressed', 'false')
})
