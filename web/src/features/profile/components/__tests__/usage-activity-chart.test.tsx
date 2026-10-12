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
import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { afterEach, expect, test, vi } from 'vitest'

import { UsageActivityChart } from '../usage-activity-chart'

const days = [
  { date: '2023-12-31', tokens: 100 },
  { date: '2024-01-01', tokens: 20 },
  { date: '2024-01-02', tokens: 30 },
  { date: '2024-01-08', tokens: 70 },
  { date: '2024-01-10', tokens: 10 },
  { date: '2024-01-11', tokens: 9000 },
]

afterEach(async () => {
  await i18next.changeLanguage('en')
})

test('switching daily, weekly and cumulative views exposes each mode’s actual totals', async () => {
  const user = userEvent.setup()
  render(<UsageActivityChart days={days} year={2024} today='2024-01-10' />)

  expect(screen.getByRole('button', { name: 'Daily' })).toHaveAttribute(
    'aria-pressed',
    'true'
  )
  expect(screen.getByLabelText('Selected chart data')).toHaveTextContent(
    'Jan 10, 2024: 10 tokens'
  )
  await user.click(screen.getByRole('button', { name: 'Weekly' }))
  expect(screen.getByRole('button', { name: 'Weekly' })).toHaveAttribute(
    'aria-pressed',
    'true'
  )
  expect(screen.getByRole('button', { name: 'Daily' })).toHaveAttribute(
    'aria-pressed',
    'false'
  )
  expect(screen.getByLabelText('Selected chart data')).toHaveTextContent(
    'Jan 8, 2024 – Jan 10, 2024: 80 tokens'
  )
  await user.click(screen.getByRole('button', { name: 'Cumulative' }))
  expect(screen.getByRole('button', { name: 'Cumulative' })).toHaveAttribute(
    'aria-pressed',
    'true'
  )
  expect(screen.getByLabelText('Selected chart data')).toHaveTextContent(
    'Jan 10, 2024: 230 tokens'
  )
})

test('chart arrow keys and Home/End expose zero-filled dates and stop at today', async () => {
  const user = userEvent.setup()
  render(<UsageActivityChart days={days} year={2024} today='2024-01-10' />)
  const chart = screen.getByRole('slider', { name: 'Explore chart data' })

  chart.focus()
  await user.keyboard('{ArrowLeft}')
  expect(chart).toHaveAttribute('aria-valuetext', 'Jan 9, 2024: 0 tokens')
  await user.keyboard('{Home}')
  expect(chart).toHaveAttribute('aria-valuetext', 'Jan 1, 2024: 20 tokens')
  await user.keyboard('{ArrowLeft}')
  expect(chart).toHaveAttribute('aria-valuenow', '0')
  await user.keyboard('{End}{ArrowRight}')
  expect(chart).toHaveAttribute('aria-valuetext', 'Jan 10, 2024: 10 tokens')
  expect(chart).toHaveAttribute('aria-valuemax', '9')
})

test('tapping a chart position reveals its date and exact token count', () => {
  render(<UsageActivityChart days={days} year={2024} today='2024-01-10' />)
  const chart = screen.getByRole('slider', { name: 'Explore chart data' })
  const svg = chart.querySelector('svg')
  expect(svg).not.toBeNull()
  if (!svg) return
  vi.spyOn(svg, 'getBoundingClientRect').mockReturnValue(
    new DOMRect(0, 0, 720, 216)
  )

  fireEvent.pointerDown(svg, { clientX: 75, pointerType: 'touch' })

  expect(chart).toHaveAttribute('aria-valuetext', 'Jan 2, 2024: 30 tokens')
  expect(screen.getByLabelText('Selected chart data')).toHaveTextContent(
    'Jan 2, 2024: 30 tokens'
  )
})

test('calendar hover and arrow navigation reveal exact day tokens without tabbable future dates', async () => {
  const user = userEvent.setup()
  render(<UsageActivityChart days={days} year={2024} today='2024-01-10' />)
  const calendar = screen.getByRole('group', { name: 'Activity calendar' })
  const firstDay = within(calendar).getByRole('button', {
    name: 'January 1, 2024: 20 tokens',
  })

  fireEvent.pointerEnter(firstDay)
  expect(screen.getByLabelText('Selected day')).toHaveTextContent(
    'January 1, 2024: 20 tokens'
  )
  await user.click(firstDay)
  await user.keyboard('{ArrowRight}')
  expect(
    within(calendar).getByRole('button', { name: 'January 8, 2024: 70 tokens' })
  ).toHaveFocus()
  await user.keyboard('{ArrowDown}')
  expect(screen.getByLabelText('Selected day')).toHaveTextContent(
    'January 9, 2024: 0 tokens'
  )
  await user.keyboard('{End}{ArrowRight}')
  expect(
    within(calendar).getByRole('button', {
      name: 'January 10, 2024: 10 tokens',
    })
  ).toHaveFocus()
  expect(
    within(calendar)
      .getAllByRole('button')
      .filter((button) => button.tabIndex === 0)
  ).toHaveLength(1)
  expect(
    within(calendar).queryByRole('button', { name: /January 11/ })
  ).not.toBeInTheDocument()
  expect(calendar).toHaveClass('overflow-x-auto')
})

test('hovering another calendar date preserves the keyboard tab stop and allows leaving the calendar', async () => {
  const user = userEvent.setup()
  render(
    <>
      <UsageActivityChart days={days} year={2024} today='2024-01-10' />
      <button type='button'>After calendar</button>
    </>
  )
  const calendar = screen.getByRole('group', { name: 'Activity calendar' })
  const first = within(calendar).getByRole('button', {
    name: 'January 1, 2024: 20 tokens',
  })
  const last = within(calendar).getByRole('button', {
    name: 'January 10, 2024: 10 tokens',
  })
  await user.click(first)

  fireEvent.pointerEnter(last)

  expect(screen.getByLabelText('Selected day')).toHaveTextContent(
    'January 10, 2024: 10 tokens'
  )
  expect(first).toHaveAttribute('tabindex', '0')
  expect(last).toHaveAttribute('tabindex', '-1')
  await user.tab()
  expect(screen.getByRole('button', { name: 'After calendar' })).toHaveFocus()
})

test('changing years resets the inspected date and preserves historical cumulative totals', async () => {
  const user = userEvent.setup()
  const view = render(
    <UsageActivityChart days={days} year={2023} today='2024-01-10' />
  )
  await user.click(screen.getByRole('button', { name: 'Cumulative' }))
  expect(screen.getByLabelText('Selected chart data')).toHaveTextContent(
    'Dec 31, 2023: 100 tokens'
  )

  view.rerender(
    <UsageActivityChart days={days} year={2024} today='2024-01-10' />
  )
  expect(screen.getByLabelText('Selected chart data')).toHaveTextContent(
    'Jan 10, 2024: 230 tokens'
  )
  expect(screen.getByLabelText('Selected day')).toHaveTextContent(
    'January 10, 2024: 10 tokens'
  )
})

test('empty usage keeps zero dates inspectable and leap day visible', async () => {
  const user = userEvent.setup()
  render(<UsageActivityChart days={[]} year={2024} today='2024-02-29' />)
  expect(
    screen.getByRole('button', { name: 'February 29, 2024: 0 tokens' })
  ).toBeInTheDocument()
  expect(screen.getByLabelText('Selected chart data')).toHaveTextContent(
    'Feb 29, 2024: 0 tokens'
  )
  await user.click(screen.getByRole('button', { name: 'Cumulative' }))
  expect(screen.getByLabelText('Selected chart data')).toHaveTextContent(
    'Feb 29, 2024: 0 tokens'
  )
})

test('legacy Chinese locale codes render localized dates and exact token counts', async () => {
  await i18next.changeLanguage('zhCN')
  render(<UsageActivityChart days={days} year={2024} today='2024-01-10' />)
  expect(
    screen.getByRole('slider', { name: 'Explore chart data' })
  ).toHaveAttribute('aria-valuetext', '2024年1月10日: 10 tokens')
})
