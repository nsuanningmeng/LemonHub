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
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { Pricing } from '../index'
import type { PricingData } from '../types'

// Canvas is outside jsdom; this test exercises the real catalog and request state.
vi.mock('@visactor/react-vchart', () => ({ VChart: () => null }))
vi.mock('@visactor/vchart', () => ({
  ThemeManager: { setCurrentTheme: () => undefined },
}))

const payload: PricingData = {
  success: true,
  data: [
    {
      id: 1,
      model_name: 'chat-model',
      vendor_id: 1,
      quota_type: 1,
      billing_mode: 'tiered_expr',
      billing_expr: 'tier("standard", p * 1 + c * 2)',
      model_ratio: 1,
      completion_ratio: 2,
      enable_groups: ['default'],
    },
    {
      id: 2,
      model_name: 'image-model',
      vendor_id: 1,
      quota_type: 0,
      billing_mode: 'tiered_expr',
      billing_expr: 'tier("request", 0.1)',
      model_ratio: 1,
      completion_ratio: 1,
      model_price: 0.1,
      enable_groups: ['default'],
    },
  ],
  vendors: [{ id: 1, name: 'Example' }],
  group_ratio: { default: 1 },
  usable_group: { default: { desc: 'Default', ratio: 1 } },
  supported_endpoint: {},
  auto_groups: [],
}
const clients: QueryClient[] = []
afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  localStorage.clear()
})

async function renderCatalog() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  clients.push(client)
  client.setQueryData(['status'], {})
  client.setQueryData(['notice'], { success: true, data: '' })
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  const root = createRootRoute()
  const route = createRoute({
    getParentRoute: () => root,
    path: 'pricing/',
    component: Pricing,
  })
  const router = createRouter({
    routeTree: root.addChildren([route]),
    history: createMemoryHistory({ initialEntries: ['/pricing/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return userEvent.setup()
}

test.each(['http', 'response'])(
  'a failed %s catalog request shows a retry action instead of claiming that no models match',
  async (failure) => {
    let pricingAttempts = 0
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (url === '/api/pricing') {
        pricingAttempts++
        if (pricingAttempts === 1) {
          if (failure === 'http') throw new Error('Service unavailable')
          return { data: { success: false, message: 'Service unavailable' } }
        }
        return { data: payload }
      }
      return { data: { success: true, data: { models: [] } } }
    })
    const user = await renderCatalog()
    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('Unable to load the model catalog.')
    expect(screen.queryByText('No models found')).not.toBeInTheDocument()
    await user.click(within(alert).getByRole('button', { name: 'Retry' }))
    expect(
      await screen.findByRole('article', { name: 'chat-model' })
    ).toBeVisible()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  }
)

test('active billing expressions keep catalog categories and sidebar selection synchronized when either control changes or resets the filter', async () => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data:
      url === '/api/pricing'
        ? payload
        : { success: true, data: { models: [] } },
  }))
  const user = await renderCatalog()
  await screen.findByRole('article', { name: 'chat-model' })
  const categories = within(
    screen.getByRole('region', { name: 'Browse models' })
  )
  const sidebar = within(
    screen.getByRole('complementary', { name: 'Catalog filters' })
  )
  await user.click(categories.getByRole('button', { name: /^Per Request/ }))
  const billingDisclosure = sidebar.getByRole('button', {
    name: 'Pricing Type',
  })
  expect(billingDisclosure).toHaveTextContent('Per Request')
  await user.click(billingDisclosure)
  expect(sidebar.getByRole('button', { name: /^Per Request/ })).toHaveAttribute(
    'aria-pressed',
    'true'
  )
  expect(
    screen.queryByRole('article', { name: 'chat-model' })
  ).not.toBeInTheDocument()
  expect(screen.getByRole('article', { name: 'image-model' })).toBeVisible()
  await user.click(sidebar.getByRole('button', { name: /^Token-based/ }))
  expect(
    categories.getByRole('button', { name: /^Token-based/ })
  ).toHaveAttribute('aria-pressed', 'true')
  expect(
    screen.queryByRole('article', { name: 'image-model' })
  ).not.toBeInTheDocument()
  await user.click(sidebar.getByRole('button', { name: 'Reset' }))
  expect(
    categories.getByRole('button', { name: /^All Models/ })
  ).toHaveAttribute('aria-pressed', 'true')
  expect(screen.getAllByRole('article')).toHaveLength(2)
})
