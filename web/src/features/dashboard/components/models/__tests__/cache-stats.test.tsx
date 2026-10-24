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
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { getUserQuotaDates } from '@/features/dashboard/api'
import { calculateDashboardStats } from '@/features/dashboard/lib/stats'

import { LogStatCards } from '../log-stat-cards'

vi.mock('@/features/dashboard/api', () => ({ getUserQuotaDates: vi.fn() }))
afterEach(cleanup)

test('aggregates cache usage without adding it to the already inclusive total', () => {
  expect(
    calculateDashboardStats([
      {
        created_at: 1,
        token_used: 900,
        cache_read_tokens: 300,
        cache_creation_tokens: 100,
      },
      { created_at: 2, token_used: 100 },
      { created_at: 3, cache_read_tokens: -1, cache_creation_tokens: Infinity },
    ])
  ).toMatchObject({
    totalTokens: 1000,
    totalCacheReadTokens: 300,
    totalCacheCreationTokens: 100,
  })
})

test('shows cache totals and explains that historical data is not backfilled', async () => {
  vi.mocked(getUserQuotaDates).mockResolvedValue({
    success: true,
    data: [
      {
        created_at: 1,
        token_used: 900,
        cache_read_tokens: 300,
        cache_creation_tokens: 100,
      },
    ],
  })
  render(<LogStatCards />)
  await waitFor(() =>
    expect(screen.getByRole('group', { name: 'Cache Read' })).toHaveAttribute(
      'aria-busy',
      'false'
    )
  )
  expect(
    within(screen.getByRole('group', { name: 'Cache Read' })).getByText('300')
  ).toBeVisible()
  expect(
    within(screen.getByRole('group', { name: 'Cache Write' })).getByText('100')
  ).toBeVisible()
  expect(
    within(screen.getByRole('group', { name: 'Total Tokens' })).getByText('900')
  ).toBeVisible()
  expect(
    screen.getByText(
      'Cache usage is recorded only after the upgrade; historical data is not backfilled.'
    )
  ).toBeVisible()
})

test('does not present failed cache requests as zero usage', async () => {
  vi.mocked(getUserQuotaDates).mockRejectedValue(new Error('offline'))
  render(<LogStatCards />)
  await waitFor(() =>
    expect(screen.getByRole('group', { name: 'Cache Read' })).toHaveAttribute(
      'aria-busy',
      'false'
    )
  )
  expect(
    within(screen.getByRole('group', { name: 'Cache Read' })).getByText('--')
  ).toBeVisible()
})
