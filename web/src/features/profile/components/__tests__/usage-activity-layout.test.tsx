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
import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { UsageActivityChart } from '../usage-activity-chart'

afterEach(() => vi.unstubAllGlobals())

test('chart canvas follows its container while labels stay readable at narrow and wide widths', () => {
  let resize: ((width: number) => void) | undefined
  vi.stubGlobal(
    'ResizeObserver',
    class implements ResizeObserver {
      private callback: ResizeObserverCallback
      constructor(callback: ResizeObserverCallback) {
        this.callback = callback
      }
      observe(target: Element) {
        resize = (width) =>
          this.callback(
            [
              {
                target,
                contentRect: new DOMRect(0, 0, width, 216),
              } as ResizeObserverEntry,
            ],
            this
          )
        resize(272)
      }
      unobserve() {}
      disconnect() {}
    }
  )
  render(
    <UsageActivityChart
      days={[{ date: '2024-01-02', tokens: 30 }]}
      year={2024}
      today='2024-01-10'
    />
  )
  const chart = screen.getByRole('slider', { name: 'Explore chart data' })
  const svg = chart.querySelector('svg')
  expect(svg).toHaveAttribute('viewBox', '0 0 272 216')
  expect(svg?.querySelector('text')).toHaveAttribute('font-size', '11')
  if (!svg) return
  vi.spyOn(svg, 'getBoundingClientRect').mockReturnValue(
    new DOMRect(0, 0, 272, 216)
  )
  fireEvent.pointerDown(svg, { clientX: 73, pointerType: 'touch' })
  expect(chart).toHaveAttribute('aria-valuetext', 'Jan 2, 2024: 30 tokens')

  act(() => resize?.(800))

  expect(svg).toHaveAttribute('viewBox', '0 0 800 216')
  expect(svg.querySelector('text')).toHaveAttribute('font-size', '11')
})

test('long translated activity modes can wrap within a narrow card', () => {
  render(<UsageActivityChart days={[]} year={2024} today='2024-01-10' />)
  expect(screen.getByRole('group', { name: 'Activity view' })).toHaveClass(
    'flex-wrap'
  )
})
