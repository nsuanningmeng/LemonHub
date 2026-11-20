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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { api } from '@/lib/api'

import { SubscriptionsProvider } from '../subscriptions-provider'
import { SubscriptionsTable } from '../subscriptions-table'

const clients: QueryClient[] = []
const plan = {
  plan: {
    id: 1,
    title: 'Monthly studio',
    price_amount: 10,
    duration_unit: 'month',
    duration_value: 1,
    quota_reset_period: 'monthly',
    enabled: true,
    sort_order: 1,
    max_purchase_per_user: 1,
    total_amount: 5000000,
  },
}

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  localStorage.clear()
})

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <SubscriptionsProvider>
          <SubscriptionsTable />
        </SubscriptionsProvider>
      </TooltipProvider>
    </QueryClientProvider>
  )
}

it.each(['business failure', 'network failure'])(
  'shows an error for %s and restores the plan list after Retry',
  async (failure) => {
    let unavailable = true
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (url === '/api/subscription/admin/payment-compliance') {
        return {
          data: {
            success: true,
            data: { confirmed: true, terms_version: 'v1' },
          },
        }
      }
      if (unavailable) {
        if (failure === 'network failure') throw new Error('Plans unavailable')
        return { data: { success: false, message: 'Plans unavailable' } }
      }
      return { data: { success: true, data: [plan] } }
    })
    renderPage()
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Plans unavailable'
    )
    expect(
      screen.queryByText('No subscription plans yet')
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('navigation', { name: 'Pagination' })
    ).not.toBeInTheDocument()
    unavailable = false
    await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
    await screen.findByText('Monthly studio')
    await waitFor(() =>
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    )
  }
)

it('keeps a successful empty plan list distinct from a loading failure', async () => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data: {
      success: true,
      data:
        url === '/api/subscription/admin/payment-compliance'
          ? { confirmed: true, terms_version: 'v1' }
          : [],
    },
  }))
  renderPage()
  await screen.findByText('No subscription plans yet')
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Retry' })
  ).not.toBeInTheDocument()
})
