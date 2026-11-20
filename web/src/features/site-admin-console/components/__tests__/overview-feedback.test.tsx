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
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { OverviewTab } from '../overview-tab'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

test('a failed dashboard response can be retried before the balance is displayed', async () => {
  const user = userEvent.setup()
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  let available = false
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url.includes('/wallet/logs')) {
      return { data: { success: true, data: { items: [], total: 0 } } }
    }
    return {
      data: available
        ? {
            success: true,
            data: {
              name: 'Example site',
              wallet_balance: 12500,
              wallet_warn_threshold: 0,
              status: 1,
              discount_rate: 10000,
              domains: [],
            },
          }
        : { success: false, message: 'Site offline' },
    }
  })
  try {
    render(
      <QueryClientProvider client={client}>
        <OverviewTab />
      </QueryClientProvider>
    )
    expect(await screen.findByRole('alert')).toBeVisible()
    expect(screen.queryByText('Current Balance')).not.toBeInTheDocument()
    available = true
    await user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('Example site')).toBeVisible()
    expect(
      screen.getByRole('spinbutton', { name: /Wallet Warn Threshold/ })
    ).toHaveValue(0)
  } finally {
    cleanup()
    client.clear()
  }
})
