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
import { describe, expect, test } from 'vitest'

import {
  getActivityDate,
  getMondayOffset,
  getUsageActivityDays,
  getUsageActivitySeries,
} from '../usage-activity'

describe('usage activity calendar', () => {
  test('fills a leap year with zero days when no activity is recorded', () => {
    const calendar = getUsageActivityDays([], 2024, '2026-10-12')

    expect(calendar).toHaveLength(366)
    expect(calendar[0]).toEqual({ date: '2024-01-01', tokens: 0 })
    expect(calendar[59]).toEqual({ date: '2024-02-29', tokens: 0 })
    expect(calendar.at(-1)).toEqual({ date: '2024-12-31', tokens: 0 })
    expect(calendar.every((day) => day.tokens === 0)).toBe(true)
  })

  test('omits February 29 in a non-leap century year', () => {
    const calendar = getUsageActivityDays([], 2100, '2100-12-31')

    expect(calendar).toHaveLength(365)
    expect(calendar[59].date).toBe('2100-03-01')
  })

  test('adds duplicate dates and rejects invalid dates and token counts', () => {
    const calendar = getUsageActivityDays(
      [
        { date: '2025-01-03', tokens: 7 },
        { date: '2025-01-01', tokens: 12 },
        { date: '2025-01-01', tokens: 8 },
        { date: '2025-01-01', tokens: -40 },
        { date: '2025-01-02', tokens: Number.NaN },
        { date: '2025-01-02', tokens: Number.POSITIVE_INFINITY },
        { date: '2025-01-02', tokens: Number.NEGATIVE_INFINITY },
        { date: '2025-02-29', tokens: 100 },
        { date: '2025-01-32', tokens: 100 },
        { date: '2025-1-01', tokens: 100 },
        { date: '2025-01-01T00:00:00Z', tokens: 100 },
        { date: 'not-a-date', tokens: 100 },
      ],
      2025,
      '2025-12-31'
    )

    expect(calendar.slice(0, 3)).toEqual([
      { date: '2025-01-01', tokens: 20 },
      { date: '2025-01-02', tokens: 0 },
      { date: '2025-01-03', tokens: 7 },
    ])
    expect(calendar.reduce((total, day) => total + day.tokens, 0)).toBe(27)
  })

  test('preserves today but clears every future date in the full-year calendar', () => {
    const calendar = getUsageActivityDays(
      [
        { date: '2026-01-01', tokens: 2 },
        { date: '2026-01-02', tokens: 3 },
        { date: '2026-12-31', tokens: 4 },
      ],
      2026,
      '2026-01-01'
    )

    expect(calendar).toHaveLength(365)
    expect(calendar[0]).toEqual({ date: '2026-01-01', tokens: 2 })
    expect(calendar.slice(1).every((day) => day.tokens === 0)).toBe(true)
  })
})

describe('usage activity series', () => {
  test('fills missing daily values and ends at today', () => {
    expect(
      getUsageActivitySeries(
        [
          { date: '2026-01-02', tokens: 7 },
          { date: '2026-01-03', tokens: 99 },
        ],
        2026,
        '2026-01-02',
        'daily'
      )
    ).toEqual([
      { date: '2026-01-01', endDate: '2026-01-01', tokens: 0 },
      { date: '2026-01-02', endDate: '2026-01-02', tokens: 7 },
    ])
  })

  test('starts weeks on Monday and clips cross-year and current weeks', () => {
    expect(
      getUsageActivitySeries(
        [
          { date: '2024-12-30', tokens: 100 },
          { date: '2025-01-01', tokens: 2 },
          { date: '2025-01-05', tokens: 3 },
          { date: '2025-01-06', tokens: 7 },
          { date: '2025-01-08', tokens: 11 },
          { date: '2025-01-09', tokens: 100 },
        ],
        2025,
        '2025-01-08',
        'weekly'
      )
    ).toEqual([
      { date: '2025-01-01', endDate: '2025-01-05', tokens: 5 },
      { date: '2025-01-06', endDate: '2025-01-08', tokens: 18 },
    ])
  })

  test('ends a past year at December 31 without counting the following January', () => {
    const series = getUsageActivitySeries(
      [
        { date: '2025-12-31', tokens: 5 },
        { date: '2026-01-01', tokens: 8 },
      ],
      2025,
      '2026-01-01',
      'weekly'
    )

    expect(series.at(-1)).toEqual({
      date: '2025-12-29',
      endDate: '2025-12-31',
      tokens: 5,
    })
  })

  test('carries valid historical totals into cumulative values and ignores future activity', () => {
    expect(
      getUsageActivitySeries(
        [
          { date: '2023-12-31', tokens: 5 },
          { date: '2024-02-29', tokens: 10 },
          { date: '2024-02-29', tokens: 2 },
          { date: '2023-02-29', tokens: 100 },
          { date: '2024-12-31', tokens: -10 },
          { date: '2024-12-31', tokens: Number.POSITIVE_INFINITY },
          { date: '2025-01-01', tokens: 3 },
          { date: '2025-01-03', tokens: 4 },
          { date: '2025-01-04', tokens: 100 },
          { date: '2026-01-01', tokens: 100 },
        ],
        2025,
        '2025-01-03',
        'cumulative'
      )
    ).toEqual([
      { date: '2025-01-01', endDate: '2025-01-01', tokens: 20 },
      { date: '2025-01-02', endDate: '2025-01-02', tokens: 20 },
      { date: '2025-01-03', endDate: '2025-01-03', tokens: 24 },
    ])
  })

  test.each(['daily', 'weekly', 'cumulative'] as const)(
    'returns no %s points when the selected year is in the future',
    (mode) => {
      expect(
        getUsageActivitySeries(
          [{ date: '2027-01-01', tokens: 10 }],
          2027,
          '2026-10-12',
          mode
        )
      ).toEqual([])
    }
  )
})

describe('activity date handling', () => {
  test('parses calendar dates at UTC midnight across daylight-saving changes', () => {
    expect(getActivityDate('2026-03-08').toISOString()).toBe(
      '2026-03-08T00:00:00.000Z'
    )
    expect(getActivityDate('2026-11-01').toISOString()).toBe(
      '2026-11-01T00:00:00.000Z'
    )
    expect(getMondayOffset('2026-10-12')).toBe(0)
    expect(getMondayOffset('2026-10-18')).toBe(6)
  })

  test('rejects invalid calendar dates instead of rolling them into another month', () => {
    expect(getActivityDate('2025-02-29').getTime()).toBeNaN()
    expect(getActivityDate('2026-13-01').getTime()).toBeNaN()
    expect(getActivityDate('2026-01-01T00:00:00Z').getTime()).toBeNaN()
    expect(getUsageActivityDays([], 2026, 'not-a-date')).toEqual([])
    expect(getUsageActivityDays([], Number.NaN, '2026-10-12')).toEqual([])
  })
})
