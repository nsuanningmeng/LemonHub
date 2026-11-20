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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'

import { WorkspaceActivityChart } from '../workspace-activity-chart'

const props = {
  series: { usage: [500000, 1000000], requests: [5, 9] },
  start: 1735689600,
  end: 1735776000,
  loading: false,
  failed: false,
  retry: vi.fn(),
}

test('switching the chart metric using the keyboard displays the selected request total', async () => {
  const user = userEvent.setup()
  render(<WorkspaceActivityChart {...props} />)
  expect(
    screen.getByRole('img', { name: 'Usage by time of day' })
  ).toBeVisible()
  expect(screen.getByText('$3')).toBeVisible()
  const requests = screen.getByRole('button', {
    name: 'Requests',
    pressed: false,
  })
  requests.focus()
  await user.keyboard('{Enter}')
  expect(requests).toHaveAttribute('aria-pressed', 'true')
  expect(
    screen.getByRole('img', { name: 'Requests by time of day' })
  ).toBeVisible()
  expect(screen.getByText('14')).toBeVisible()
})

test('failed data does not show an apparent zero-usage chart and offers retry', async () => {
  const user = userEvent.setup()
  const retry = vi.fn()
  render(<WorkspaceActivityChart {...props} failed retry={retry} />)
  expect(screen.queryByRole('img')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  expect(retry).toHaveBeenCalledOnce()
})

test('an empty period is visible while zero-cost requests still have a chart', () => {
  const { rerender } = render(
    <WorkspaceActivityChart {...props} series={{ usage: [0], requests: [0] }} />
  )
  expect(screen.getByText('No activity in this period')).toBeVisible()
  rerender(
    <WorkspaceActivityChart
      {...props}
      series={{ usage: [0], requests: [10] }}
    />
  )
  expect(
    screen.getByRole('img', { name: 'Usage by time of day' })
  ).toBeVisible()
})
