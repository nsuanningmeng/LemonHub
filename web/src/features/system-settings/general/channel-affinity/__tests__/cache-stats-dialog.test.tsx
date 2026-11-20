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

import { api } from '@/lib/api'

import { CacheStatsDialog } from '../cache-stats-dialog'

test('a rejected cache stats response can be retried for the same rule and key', async () => {
  const target = {
    rule_name: 'Example rule',
    using_group: 'default',
    key_hint: '',
    key_fp: 'example',
  }
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValueOnce({
      data: { success: false, message: 'Statistics unavailable' },
    })
    .mockResolvedValueOnce({
      data: { success: true, data: { hit: 1, total: 2 } },
    })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <CacheStatsDialog open onOpenChange={() => {}} target={target} />
    </QueryClientProvider>
  )
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Statistics unavailable'
  )
  expect(screen.queryByText('No data available')).toBeNull()
  await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
  expect(await screen.findByText('1/2 (50.00%)')).toBeVisible()
  expect(get).toHaveBeenLastCalledWith(
    '/api/log/channel_affinity_usage_cache',
    expect.objectContaining({ params: target })
  )
  get.mockRestore()
})
