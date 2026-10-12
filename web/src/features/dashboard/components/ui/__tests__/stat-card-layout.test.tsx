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
import { ChartNoAxesColumn } from 'lucide-react'
import { afterEach, expect, test } from 'vitest'

import { StatCard } from '../stat-card'

afterEach(cleanup)

test('large exact totals stay on one readable line without splitting digits', () => {
  render(
    <StatCard
      title='Requests'
      value='1,234,567'
      description='All requests'
      icon={ChartNoAxesColumn}
      compactMobile
    />
  )
  const value = screen.getByText('1,234,567')
  expect(value).toHaveClass('whitespace-nowrap', 'overflow-x-auto')
  expect(value).not.toHaveClass('break-all')
  expect(value).toHaveAttribute('title', '1,234,567')
})
