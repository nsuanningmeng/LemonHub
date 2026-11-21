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
import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import WebsiteTraffic from '../website-traffic'

const counts = { page_views: 7, unique_visitors: 2, new_visitors: 1 }
const weekResponse = {
  data: {
    success: true,
    data: {
      summary: counts,
      daily: [
        {
          date: '2026-11-20',
          page_views: 3,
          unique_visitors: 2,
          new_visitors: 1,
        },
        {
          date: '2026-11-21',
          page_views: 4,
          unique_visitors: 1,
          new_visitors: 0,
        },
      ],
    },
  },
}

let client: QueryClient
function renderTraffic() {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <WebsiteTraffic />
    </QueryClientProvider>
  )
}

afterEach(() => {
  cleanup()
  client.clear()
})

test('shows the distinct visitor total for the period and the latest day first', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue(weekResponse)
  renderTraffic()
  expect(screen.getByRole('status', { name: 'Loading' })).toBeVisible()

  const visitors = await screen.findByRole('group', { name: 'Unique visitors' })
  expect(within(visitors).getByText('2')).toBeVisible()
  expect(
    screen.getAllByRole('definition').map((metric) => metric.textContent)
  ).toEqual(['1', '3.5'])
  const table = screen.getByRole('table', { name: 'Daily traffic (UTC)' })
  const days = within(table).getAllByRole('rowheader')
  expect(days.map((day) => day.textContent)).toEqual([
    '2026-11-21',
    '2026-11-20',
  ])
  expect(get).toHaveBeenCalledWith('/api/site-analytics', {
    params: { days: 7 },
  })
})

test('selecting another period replaces the report with its own empty state', async () => {
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValueOnce(weekResponse)
    .mockResolvedValueOnce({
      data: {
        success: true,
        data: {
          summary: { page_views: 0, unique_visitors: 0, new_visitors: 0 },
          daily: [
            {
              date: '2026-11-22',
              page_views: 0,
              unique_visitors: 0,
              new_visitors: 0,
            },
          ],
        },
      },
    })
  renderTraffic()
  await screen.findByRole('table')
  const today = screen.getByRole('button', { name: 'Today' })
  await userEvent.click(today)
  expect(
    await screen.findByText('No page views recorded in this period.')
  ).toBeVisible()
  expect(today).toHaveAttribute('aria-pressed', 'true')
  expect(screen.queryByRole('table')).toBeNull()
  expect(
    screen.getAllByRole('definition').map((metric) => metric.textContent)
  ).toEqual(['0', '—'])
  expect(get).toHaveBeenLastCalledWith('/api/site-analytics', {
    params: { days: 1 },
  })
})

test('a failed report shows a retry that recovers the data', async () => {
  vi.spyOn(api, 'get')
    .mockResolvedValueOnce({ data: { success: false, message: 'unavailable' } })
    .mockResolvedValueOnce(weekResponse)
  renderTraffic()
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Failed to load website traffic'
  )
  expect(screen.queryByRole('group', { name: 'Page views' })).toBeNull()
  await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
  expect(await screen.findByRole('table')).toBeVisible()
})
