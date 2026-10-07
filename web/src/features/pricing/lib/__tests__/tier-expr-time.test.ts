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

import { evalExprLocally, type ExtraTokenValues } from '../tier-expr'

const EXTRA_TOKENS: ExtraTokenValues = {
  cacheReadTokens: 100,
  cacheCreateTokens: 0,
  cacheCreate1hTokens: 0,
  imageTokens: 0,
  imageOutputTokens: 0,
  audioInputTokens: 0,
  audioOutputTokens: 0,
}

const WEEKDAY_PEAK_EXPR =
  '(tier("base", p * 1.5 + c * 4.5 + cr * 0.15)) * ((weekday("Asia/Shanghai") >= 1 && weekday("Asia/Shanghai") < 6 && ((hour("Asia/Shanghai") >= 9 && hour("Asia/Shanghai") < 12) || (hour("Asia/Shanghai") >= 14 && hour("Asia/Shanghai") < 18))) ? 2 : 1)'

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
})

afterEach(() => {
  vi.useRealTimers()
})

describe('time functions in the local token estimator', () => {
  test.each([
    ['Monday before peak', '2026-10-05T00:59:59Z', 615],
    ['Monday at 09:00', '2026-10-05T01:00:00Z', 1230],
    ['Monday before noon', '2026-10-05T03:59:59Z', 1230],
    ['Monday at 12:00', '2026-10-05T04:00:00Z', 615],
    ['Monday at 14:00', '2026-10-05T06:00:00Z', 1230],
    ['Monday before 18:00', '2026-10-05T09:59:59Z', 1230],
    ['Monday at 18:00', '2026-10-05T10:00:00Z', 615],
    ['Sunday during weekday peak hours', '2026-10-04T01:00:00Z', 615],
    ['Saturday during weekday peak hours', '2026-10-03T06:00:00Z', 615],
  ])('evaluates nested weekday and OR windows at %s', (_label, now, cost) => {
    vi.setSystemTime(new Date(now))

    expect(evalExprLocally(WEEKDAY_PEAK_EXPR, 100, 100, EXTRA_TOKENS)).toEqual({
      cost,
      matchedTier: 'base',
      error: null,
    })
  })

  test.each([
    ['UTC', '1970-01-01T00:00:00Z', [0, 0, 4, 1, 1]],
    ['UTC', '2026-10-05T00:00:00Z', [0, 0, 1, 10, 5]],
    ['Asia/Shanghai', '2026-10-04T16:05:00Z', [0, 5, 1, 10, 5]],
    ['UTC', '2026-10-04T16:05:00Z', [16, 5, 0, 10, 4]],
    ['Asia/Shanghai', '2026-12-31T16:00:00Z', [0, 0, 5, 1, 1]],
    ['UTC', '2024-02-29T23:59:00Z', [23, 59, 4, 2, 29]],
    ['America/New_York', '2026-03-08T06:59:00Z', [1, 59, 0, 3, 8]],
    ['America/New_York', '2026-03-08T07:00:00Z', [3, 0, 0, 3, 8]],
    ['America/New_York', '2026-11-01T05:30:00Z', [1, 30, 0, 11, 1]],
    ['America/New_York', '2026-11-01T06:30:00Z', [1, 30, 0, 11, 1]],
  ])(
    'uses Gregorian local components for %s at %s',
    (timezone, now, values) => {
      vi.setSystemTime(new Date(now))

      const results = ['hour', 'minute', 'weekday', 'month', 'day'].map(
        (name) => evalExprLocally(`${name}("${timezone}")`, 0, 0, EXTRA_TOKENS)
      )

      expect(results.map((result) => result.error)).toEqual([
        null,
        null,
        null,
        null,
        null,
      ])
      expect(results.map((result) => result.cost)).toEqual(values)
    }
  )

  test.each([
    '',
    '   ',
    'Invalid/Zone',
    '  Invalid/Zone  ',
    '+08:00',
    '-0500',
    '+08',
    'asia/shanghai',
    'america/new_york',
    'AMERICA/NEW_YORK',
  ])('uses UTC for empty or invalid timezone %j', (timezone) => {
    vi.setSystemTime(new Date('2026-10-04T23:07:00Z'))
    const expression = `hour("${timezone}") * 100 + minute("${timezone}")`

    expect(evalExprLocally(expression, 0, 0, EXTRA_TOKENS)).toEqual({
      cost: 2307,
      matchedTier: '',
      error: null,
    })
  })

  test('trims a valid timezone before calculating the date', () => {
    vi.setSystemTime(new Date('2026-10-04T23:07:00Z'))

    expect(
      evalExprLocally(
        'weekday("  Asia/Shanghai  ") * 100 + hour("  Asia/Shanghai  ")',
        0,
        0,
        EXTRA_TOKENS
      )
    ).toEqual({ cost: 107, matchedTier: '', error: null })
  })

  test.each(['Local', '  Local  '])(
    'returns an explicit limitation for server-local timezone %j',
    (timezone) => {
      vi.setSystemTime(new Date('2026-10-04T23:07:00Z'))

      expect(
        evalExprLocally(`hour("${timezone}")`, 0, 0, EXTRA_TOKENS)
      ).toEqual({
        cost: 0,
        matchedTier: '',
        error:
          'Server-local time cannot be estimated in the browser. Use an IANA time zone.',
      })
    }
  )

  test('reports unsupported aliases instead of guessing their server timezone', () => {
    vi.setSystemTime(new Date('2026-10-04T23:07:00Z'))

    expect(evalExprLocally('hour("US/Pacific")', 0, 0, EXTRA_TOKENS)).toEqual({
      cost: 0,
      matchedTier: '',
      error: 'This time zone cannot be estimated reliably in this browser.',
    })
  })

  test.each([
    ['Etc/UTC', 2307],
    ['Etc/GMT', 2307],
    ['GMT', 2307],
    ['US/Eastern', 1907],
  ])('supports the verified exact alias %s', (timezone, cost) => {
    vi.setSystemTime(new Date('2026-10-04T23:07:00Z'))

    expect(
      evalExprLocally(
        `hour("${timezone}") * 100 + minute("${timezone}")`,
        0,
        0,
        EXTRA_TOKENS
      )
    ).toEqual({ cost, matchedTier: '', error: null })
  })

  test.each([
    ['2026-10-05T20:59:59Z', 100],
    ['2026-10-05T21:00:00Z', 50],
    ['2026-10-06T00:00:00Z', 50],
    ['2026-10-06T05:59:59Z', 50],
    ['2026-10-06T06:00:00Z', 100],
  ])('keeps an overnight range half-open at %s', (now, cost) => {
    vi.setSystemTime(new Date(now))

    expect(
      evalExprLocally(
        'tier("night", p) * (hour("UTC") >= 21 || hour("UTC") < 6 ? 0.5 : 1)',
        100,
        0,
        EXTRA_TOKENS
      )
    ).toEqual({ cost, matchedTier: 'night', error: null })
  })

  test('all time functions use one instant even if the clock crosses midnight during evaluation', () => {
    vi.setSystemTime(new Date('2026-10-04T23:59:59Z'))
    const formatToParts = Intl.DateTimeFormat.prototype.formatToParts
    vi.spyOn(Intl.DateTimeFormat.prototype, 'formatToParts').mockImplementation(
      function (this: Intl.DateTimeFormat, date?: number | Date) {
        const parts = formatToParts.call(this, date)
        vi.setSystemTime(Date.now() + 2000)
        return parts
      }
    )

    expect(
      evalExprLocally(
        'hour("UTC") * 100 + minute("Asia/Shanghai")',
        0,
        0,
        EXTRA_TOKENS
      )
    ).toEqual({ cost: 2359, matchedTier: '', error: null })
  })

  test('a time condition can select an explicit zero price and preserve its tier', () => {
    vi.setSystemTime(new Date('2026-10-04T01:00:00Z'))

    expect(
      evalExprLocally(
        'weekday("UTC") == 0 ? tier("free", 0) : tier("paid", p * 3)',
        100,
        0,
        EXTRA_TOKENS
      )
    ).toEqual({ cost: 0, matchedTier: 'free', error: null })
  })

  test('a new estimate observes the current time instead of reusing a previous snapshot', () => {
    vi.setSystemTime(new Date('2026-10-05T00:59:59Z'))
    expect(evalExprLocally('hour("UTC")', 0, 0, EXTRA_TOKENS).cost).toBe(0)

    vi.setSystemTime(new Date('2026-10-05T01:00:00Z'))
    expect(evalExprLocally('hour("UTC")', 0, 0, EXTRA_TOKENS).cost).toBe(1)
  })

  test('adding time functions preserves full input len and separately supplied token categories', () => {
    vi.setSystemTime(new Date('2026-10-05T01:00:00Z'))
    const extras = {
      ...EXTRA_TOKENS,
      cacheCreateTokens: 20,
      cacheCreate1hTokens: 30,
    }

    expect(
      evalExprLocally(
        'weekday("UTC") == 1 && len == 250 ? tier("cached", p * 2 + c * 4 + cr * 0.5 + cc + cc1h * 2) : tier("other", 0)',
        100,
        10,
        extras
      )
    ).toEqual({ cost: 370, matchedTier: 'cached', error: null })
  })

  test('unavailable functions still return an expression error', () => {
    vi.setSystemTime(new Date('2026-10-05T01:00:00Z'))

    expect(
      evalExprLocally('unknownTime("UTC") + p', 100, 0, EXTRA_TOKENS)
    ).toEqual({
      cost: 0,
      matchedTier: '',
      error: 'unknownTime is not defined',
    })
  })
})
