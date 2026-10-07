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
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import type { QuotaDataItem } from '../../types'
import { processChartData, processUserChartData } from '../charts'

interface ChartRow {
  Timestamp: number
  Time: string
  Model?: string
  User?: string
  rawQuota?: number
  Count?: number
}

function values(spec: Record<string, unknown>): ChartRow[] {
  return (spec.data as { values: ChartRow[] }[])[0].values
}

function datum(
  date: string,
  count = 1,
  quota = 500000,
  model = 'alpha',
  username = 'alice'
): QuotaDataItem {
  return {
    created_at: Date.parse(date) / 1000,
    model_name: model,
    username,
    count,
    quota,
    token_used: count * 10,
  }
}

beforeEach(() => {
  vi.stubEnv('TZ', 'UTC')
})
afterEach(() => {
  vi.unstubAllEnvs()
})

describe('chronological dashboard buckets', () => {
  test.each(['hour', 'day', 'week'] as const)(
    '%s model and user series retain numeric order across New Year',
    (granularity) => {
      const rows = [
        '2025-12-27',
        '2025-12-28',
        '2025-12-29',
        '2025-12-30',
        '2025-12-31',
        '2026-01-01',
        '2026-01-02',
        '2026-01-03',
      ].map((date, index) => datum(`${date}T00:00:00Z`, index + 1))
      const result = processChartData([...rows].reverse(), granularity)
      const user = processUserChartData([...rows].reverse(), granularity)
      for (const spec of [
        result.spec_line,
        result.spec_area,
        result.spec_model_line,
        user.spec_user_trend,
      ]) {
        expect(values(spec).map((row) => row.Time.slice(0, 5))).toEqual([
          '12-27',
          '12-28',
          '12-29',
          '12-30',
          '12-31',
          '01-01',
          '01-02',
          '01-03',
        ])
        expect(values(spec).map((row) => row.Timestamp)).toEqual(
          rows.map((row) => Number(row.created_at))
        )
      }
      expect(result.totalCountDisplay).toBe('36')
    }
  )

  test('identical month-day labels in different years have distinct band identities', () => {
    const rows = [
      datum('2026-01-01T00:00:00Z', 5, 1500000),
      datum('2025-01-01T00:00:00Z', 2, 500000),
    ]
    const result = processChartData(rows)
    const user = processUserChartData(rows)
    for (const spec of [
      result.spec_line,
      result.spec_area,
      result.spec_model_line,
      user.spec_user_trend,
    ]) {
      expect(values(spec)).toHaveLength(2)
      expect(values(spec).map((row) => row.Time)).toEqual(['01-01', '01-01'])
      expect(spec.xField).toBe('Timestamp')
      expect(new Set(values(spec).map((row) => row.Timestamp)).size).toBe(2)
      expect(values(spec).map((row) => row.Timestamp)).toEqual([
        1735689600, 1767225600,
      ])
    }
    expect(values(result.spec_line).map((row) => row.rawQuota)).toEqual([
      500000, 1500000,
    ])
    expect(values(result.spec_model_line).map((row) => row.Count)).toEqual([
      2, 5,
    ])
  })

  test('week display keeps both daily buckets from a one-day query without fabricating history', () => {
    const rows = [
      datum('2026-09-14T22:00:00Z', 3, 11671440),
      datum('2026-09-15T05:00:00Z', 5, 28625679),
    ]
    const result = processChartData(rows, 'week')
    for (const spec of [
      result.spec_line,
      result.spec_area,
      result.spec_model_line,
    ]) {
      expect(values(spec).map((row) => row.Time)).toEqual([
        '09-14 - 09-20',
        '09-15 - 09-21',
      ])
    }
    expect(
      values(result.spec_line).reduce(
        (sum, row) => sum + (row.rawQuota ?? 0),
        0
      )
    ).toBe(40297119)
    expect(
      values(result.spec_area).reduce(
        (sum, row) => sum + (row.rawQuota ?? 0),
        0
      )
    ).toBe(40297119)
    expect(
      values(result.spec_model_line).reduce(
        (sum, row) => sum + (row.Count ?? 0),
        0
      )
    ).toBe(8)
    expect(result.totalCountDisplay).toBe('8')
    expect(result.totalQuotaDisplay).toBe('$80.59')
  })

  test('single real buckets remain visible and empty results remain empty', () => {
    const rows = [datum('2026-09-15T05:00:00Z', 2)]
    const model = processChartData(rows, 'hour')
    const user = processUserChartData(rows, 'hour')
    for (const spec of [
      model.spec_area,
      model.spec_model_line,
      user.spec_user_trend,
    ]) {
      expect(values(spec)).toHaveLength(1)
      expect(spec.point.visible).toBe(true)
    }
    for (const spec of [
      processChartData([]).spec_line,
      processChartData([]).spec_area,
      processChartData([]).spec_model_line,
      processUserChartData([]).spec_user_trend,
    ]) {
      expect(values(spec)).toEqual([])
    }
  })

  test('local hour buckets keep half-hour-zone boundaries and repeated DST hours separate', () => {
    vi.stubEnv('TZ', 'Asia/Kolkata')
    const india = processChartData(
      [datum('2026-01-01T00:10:00Z'), datum('2026-01-01T00:40:00Z')],
      'hour'
    )
    expect(
      values(india.spec_line).map((row) => [row.Timestamp, row.Time])
    ).toEqual([
      [Date.parse('2025-12-31T23:30:00Z') / 1000, '01-01 05:00'],
      [Date.parse('2026-01-01T00:30:00Z') / 1000, '01-01 06:00'],
    ])
    vi.stubEnv('TZ', 'America/New_York')
    const fold = [datum('2026-11-01T05:30:00Z'), datum('2026-11-01T06:30:00Z')]
    for (const spec of [
      processChartData(fold, 'hour').spec_line,
      processUserChartData(fold, 'hour').spec_user_trend,
    ]) {
      expect(values(spec).map((row) => [row.Timestamp, row.Time])).toEqual([
        [Date.parse('2026-11-01T05:00:00Z') / 1000, '11-01 01:00'],
        [Date.parse('2026-11-01T06:00:00Z') / 1000, '11-01 01:00'],
      ])
    }
  })

  test('local-day aggregation and top-model Other groups preserve returned totals', () => {
    const rows = Array.from({ length: 22 }, (_, index) =>
      datum(
        '2026-01-01T01:00:00Z',
        index + 1,
        (index + 1) * 500000,
        `model-${index + 1}`
      )
    )
    rows.push(datum('2026-01-01T23:00:00Z', 2, 1000000, 'model-1'))
    const result = processChartData(rows, 'day')
    expect(values(result.spec_line).every((row) => row.Time === '01-01')).toBe(
      true
    )
    expect(
      new Set(values(result.spec_line).map((row) => row.Timestamp)).size
    ).toBe(1)
    expect(
      values(result.spec_line).reduce(
        (sum, row) => sum + (row.rawQuota ?? 0),
        0
      )
    ).toBe(127500000)
    expect(
      values(result.spec_area).reduce(
        (sum, row) => sum + (row.rawQuota ?? 0),
        0
      )
    ).toBe(127500000)
    expect(
      values(result.spec_model_line).reduce(
        (sum, row) => sum + (row.Count ?? 0),
        0
      )
    ).toBe(255)
    expect(
      values(result.spec_rank_bar).reduce(
        (sum, row) => sum + (row.Count ?? 0),
        0
      )
    ).toBe(255)
    expect(result.totalCountDisplay).toBe('255')
  })
})
