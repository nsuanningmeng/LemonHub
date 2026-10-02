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
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { DEFAULT_DASHBOARD_CHART_PREFERENCES } from '@/features/dashboard/constants'
import { getDefaultDays } from '@/features/dashboard/lib/filters'
import { getRollingDateRange } from '@/lib/time'

import { ModelsFilter } from '../models-filter-dialog'

const getAnimationsDescriptor = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'getAnimations'
)

beforeEach(() => {
  Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
})

afterEach(() => {
  cleanup()
  localStorage.clear()
  if (getAnimationsDescriptor) {
    Object.defineProperty(
      HTMLElement.prototype,
      'getAnimations',
      getAnimationsDescriptor
    )
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, 'getAnimations')
  }
})

test('the weekly default applies the existing 29-day quick range', async () => {
  const user = userEvent.setup()
  const onFilterChange = vi.fn()
  const end = new Date('2026-10-02T12:00:00Z')
  const range = getRollingDateRange(getDefaultDays('week'), end)
  render(
    <ModelsFilter
      preferences={DEFAULT_DASHBOARD_CHART_PREFERENCES}
      currentFilters={{
        start_timestamp: range.start,
        end_timestamp: range.end,
        time_granularity: 'week',
      }}
      onFilterChange={onFilterChange}
      onReset={() => undefined}
    />
  )
  await user.click(screen.getByRole('button', { name: 'Filter' }))
  expect(await screen.findByRole('button', { name: '29 Days' })).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Apply Filters' }))
  expect(onFilterChange).toHaveBeenCalledWith({
    start_timestamp: new Date('2026-09-03T12:00:00Z'),
    end_timestamp: end,
    time_granularity: 'week',
  })
})
