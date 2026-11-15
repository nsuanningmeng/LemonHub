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
import { describe, expect, it } from 'vitest'

import { addTimeToDate } from '../time'

describe('addTimeToDate', () => {
  it.each([
    [
      'common-year February',
      new Date(2026, 0, 31, 14, 20, 30),
      1,
      new Date(2026, 1, 28, 14, 20, 30),
    ],
    [
      'leap-year February',
      new Date(2028, 0, 31, 14, 20, 30),
      1,
      new Date(2028, 1, 29, 14, 20, 30),
    ],
    [
      'thirty-day month',
      new Date(2026, 2, 31, 14, 20, 30),
      1,
      new Date(2026, 3, 30, 14, 20, 30),
    ],
    [
      'year boundary',
      new Date(2026, 11, 31, 14, 20, 30),
      2,
      new Date(2027, 1, 28, 14, 20, 30),
    ],
    [
      'negative month',
      new Date(2026, 2, 31, 14, 20, 30),
      -1,
      new Date(2026, 1, 28, 14, 20, 30),
    ],
    [
      'ordinary date',
      new Date(2026, 0, 15, 14, 20, 30),
      1,
      new Date(2026, 1, 15, 14, 20, 30),
    ],
  ])(
    'clamps %s to the target month while preserving local time',
    (_label, baseDate, months, expected) => {
      const originalTimestamp = baseDate.getTime()

      expect(addTimeToDate(months, 0, 0, baseDate)).toEqual(expected)
      expect(baseDate.getTime()).toBe(originalTimestamp)
    }
  )

  it('adds days and hours after clamping the target month', () => {
    expect(addTimeToDate(1, 1, 2, new Date(2026, 0, 31, 23))).toEqual(
      new Date(2026, 2, 2, 1)
    )
  })

  it('returns no expiration when every duration is zero', () => {
    expect(addTimeToDate(0, 0, 0)).toBeUndefined()
  })
})
