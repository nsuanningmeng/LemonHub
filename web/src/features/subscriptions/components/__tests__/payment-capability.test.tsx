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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { Subscriptions } from '../../index'

const clients: QueryClient[] = []
const unconfirmed =
  'Subscription plan creation and changes are locked until the administrator confirms compliance terms in Payment Gateway settings.'
const failed = 'Unable to load payment compliance status. Please retry.'
afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  localStorage.clear()
})
function page() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  render(
    <QueryClientProvider client={client}>
      <Subscriptions />
    </QueryClientProvider>
  )
}
function capability(confirmed: boolean, rejected = false) {
  return vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (
      url === '/api/option/' ||
      (url === '/api/subscription/admin/payment-compliance' && rejected)
    ) {
      throw { response: { status: 403 } }
    }
    if (url === '/api/subscription/admin/payment-compliance') {
      return {
        data: {
          success: true,
          data: { confirmed, terms_version: 'server-v2' },
        },
      }
    }
    return { data: { success: true, data: [] } }
  })
}
test('an administrator uses the server capability without root options or a hardcoded terms version', async () => {
  const get = capability(true)
  page()
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Create Plan' })).toBeEnabled()
  )
  expect(screen.queryByText(unconfirmed)).not.toBeInTheDocument()
  expect(get.mock.calls.some(([url]) => url === '/api/option/')).toBe(false)
  await userEvent.click(screen.getByRole('button', { name: 'Create Plan' }))
  await screen.findByRole('dialog')
})
test('a confirmed API response of false keeps subscription mutations locked', async () => {
  capability(false)
  page()
  await screen.findByText(unconfirmed)
  expect(screen.getByRole('button', { name: 'Create Plan' })).toBeDisabled()
})
test('a real permission error is shown as a loading failure and Retry can restore the capability', async () => {
  const get = capability(true, true)
  page()
  await screen.findByText(failed)
  expect(screen.queryByText(unconfirmed)).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Create Plan' })).toBeDisabled()
  get.mockImplementation(async (url) => ({
    data: {
      success: true,
      data:
        url === '/api/subscription/admin/payment-compliance'
          ? { confirmed: true, terms_version: 'server-v2' }
          : [],
    },
  }))
  await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Create Plan' })).toBeEnabled()
  )
})
