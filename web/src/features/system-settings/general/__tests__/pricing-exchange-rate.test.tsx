import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
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
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { PricingSection } from '../pricing-section'

const clients: QueryClient[] = []
const containers: HTMLDivElement[] = []
afterEach(() => {
  clients.splice(0).forEach((c) => c.clear())
  containers.splice(0).forEach((c) => c.remove())
})
async function setup() {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  clients.push(client)
  const actions = document.createElement('div')
  document.body.append(actions)
  containers.push(actions)
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true } })
  const route = createRootRoute({
    component: () => (
      <QueryClientProvider client={client}>
        <SettingsPageProvider actionsContainer={actions}>
          <PricingSection
            defaultValues={{
              QuotaPerUnit: 500000,
              USDExchangeRate: 7,
              DisplayInCurrencyEnabled: true,
              DisplayTokenStatEnabled: true,
              general_setting: {
                quota_display_type: 'CUSTOM',
                custom_currency_symbol: '$',
                custom_currency_exchange_rate: 8,
              },
            }}
          />
        </SettingsPageProvider>
      </QueryClientProvider>
    ),
  })
  const router = createRouter({
    routeTree: route,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(<RouterProvider router={router} />)
  return put
}
test('both exchange rates accept and save 6.7081 exactly', async () => {
  const put = await setup()
  for (const name of ['USD Exchange Rate', 'Units per USD']) {
    const input = screen.getByRole<HTMLInputElement>('spinbutton', { name })
    const previousValue = input.value
    input.value = '6.7081'
    expect(input).toBeValid()
    input.value = previousValue
    fireEvent.change(input, { target: { value: '6.7081' } })
    expect(input).toBeValid()
  }
  fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() => expect(put).toHaveBeenCalledTimes(2))
  expect(put).toHaveBeenCalledWith('/api/option/', {
    key: 'USDExchangeRate',
    value: '6.7081',
  })
  expect(put).toHaveBeenCalledWith('/api/option/', {
    key: 'general_setting.custom_currency_exchange_rate',
    value: '6.7081',
  })
})
test.each(['USD Exchange Rate', 'Units per USD'])(
  '%s rejects zero and negatives',
  async (name) => {
    const put = await setup()
    for (const value of ['0', '-0.0001']) {
      fireEvent.change(screen.getByRole('spinbutton', { name }), {
        target: { value },
      })
      fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
      await screen.findByText('Exchange rate must be greater than 0')
      expect(put).not.toHaveBeenCalled()
    }
  }
)
test('nonnumeric custom input cannot save and USD preserves its previous valid value', async () => {
  const put = await setup()
  const usd = screen.getByRole('spinbutton', { name: 'USD Exchange Rate' })
  fireEvent.change(usd, { target: { value: 'NaN' } })
  expect(usd).toHaveValue(7)
  fireEvent.change(screen.getByRole('spinbutton', { name: 'Units per USD' }), {
    target: { value: 'NaN' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
  await screen.findByText('Exchange rate is required')
  expect(put).not.toHaveBeenCalled()
})
