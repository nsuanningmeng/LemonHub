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

import { Tickets } from '../index'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

test('lets a user retry a failed ticket query before showing an empty inbox', async () => {
  let unavailable = true
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/ticket/config') {
      return { data: { success: true, data: { enabled: true, types: [] } } }
    }
    return {
      data: unavailable
        ? { success: false }
        : { success: true, data: { items: [], total: 0 } },
    }
  })
  const user = userEvent.setup()
  try {
    render(
      <QueryClientProvider client={client}>
        <Tickets />
      </QueryClientProvider>
    )
    expect(await screen.findByRole('alert')).toBeVisible()
    expect(
      screen.queryByText('You have no tickets yet.')
    ).not.toBeInTheDocument()
    unavailable = false
    await user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('You have no tickets yet.')).toBeVisible()
  } finally {
    cleanup()
    client.clear()
  }
})

test('the status filter shows its translated label before the options are opened', async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { staleTime: Infinity } },
  })
  client.setQueryData(['ticket', 'config'], { enabled: true, types: [] })
  client.setQueryData(['tickets', 1, 'all'], { items: [], total: 0 })
  try {
    render(
      <QueryClientProvider client={client}>
        <Tickets />
      </QueryClientProvider>
    )
    expect(
      await screen.findByRole('combobox', { name: 'Status' })
    ).toHaveTextContent('All statuses')
  } finally {
    cleanup()
    client.clear()
  }
})
