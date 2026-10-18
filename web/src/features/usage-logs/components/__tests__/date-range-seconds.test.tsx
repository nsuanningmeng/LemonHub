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
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'

import { CompactDateTimeRangePicker } from '../compact-date-time-range-picker'

test('editing a log date range preserves existing seconds and accepts a second-precision boundary', async () => {
  const onChange = vi.fn()
  const start = new Date(2026, 8, 1, 8, 10, 15)
  const end = new Date(2026, 8, 1, 9, 20, 35)
  render(
    <CompactDateTimeRangePicker start={start} end={end} onChange={onChange} />
  )
  await userEvent.click(screen.getByRole('button', { name: /2026-09-01/ }))
  const startInput = screen.getByLabelText<HTMLInputElement>('Start Time')
  const endInput = screen.getByLabelText<HTMLInputElement>('End Time')
  expect(new Date(startInput.value).getSeconds()).toBe(15)
  expect(new Date(endInput.value).getSeconds()).toBe(35)
  expect(startInput).toHaveAttribute('step', '1')
  fireEvent.change(endInput, { target: { value: '2026-09-01T09:20:49' } })
  await userEvent.click(screen.getByRole('button', { name: 'Confirm' }))
  expect(onChange).toHaveBeenCalledWith({
    start,
    end: new Date(2026, 8, 1, 9, 20, 49),
  })
})
