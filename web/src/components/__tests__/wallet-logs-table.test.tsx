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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'

import { WalletLogsTable } from '../wallet-logs-table'

test('a failed ledger read offers retry and a successful empty page then shows its empty state', async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const fetchLogs = vi
    .fn()
    .mockRejectedValueOnce(new Error('Ledger unavailable'))
    .mockResolvedValueOnce({ items: [], total: 0 })
  render(
    <QueryClientProvider client={client}>
      <WalletLogsTable queryKey='wallet-ledger' fetchLogs={fetchLogs} />
    </QueryClientProvider>
  )
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Ledger unavailable'
  )
  expect(screen.queryByText('No Wallet Logs Found')).toBeNull()
  await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
  expect(await screen.findByText('No Wallet Logs Found')).toBeVisible()
  expect(fetchLogs).toHaveBeenCalledTimes(2)
  expect(fetchLogs).toHaveBeenLastCalledWith({
    p: 1,
    page_size: 10,
    type: undefined,
  })
})
