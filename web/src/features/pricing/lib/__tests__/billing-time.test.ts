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
import { describe, expect, test, vi } from 'vitest'

import { resolveBillingTimeZone } from '../billing-time'

describe('billing timezone support shared by display and estimation', () => {
  test.each([
    ['', 'UTC'],
    ['   ', 'UTC'],
    ['UTC', 'UTC'],
    ['Asia/Shanghai', 'Asia/Shanghai'],
    ['  America/New_York  ', 'America/New_York'],
    ['Etc/UTC', 'UTC'],
    ['Etc/GMT', 'UTC'],
    ['GMT', 'UTC'],
    ['US/Eastern', 'America/New_York'],
  ])(
    'supports exact canonical names and verified aliases: %j',
    (input, timeZone) => {
      expect(resolveBillingTimeZone(input)).toEqual({
        kind: 'supported',
        timeZone,
      })
    }
  )

  test.each([
    'asia/shanghai',
    'america/new_york',
    'AMERICA/NEW_YORK',
    'Invalid/Zone',
    '+08:00',
    '-0500',
  ])('marks Go-invalid %j for UTC evaluation and raw display', (input) => {
    expect(resolveBillingTimeZone(input)).toEqual({
      kind: 'invalid',
      timeZone: 'UTC',
    })
  })

  test.each(['Local', ' Local '])(
    'never substitutes browser-local time for %j',
    (input) => {
      expect(resolveBillingTimeZone(input)).toEqual({
        kind: 'unsupported',
        reason: 'server-local',
      })
    }
  )

  test.each(['US/Pacific', 'us/eastern'])(
    'does not guess whether unverified alias %j matches the server',
    (input) => {
      expect(resolveBillingTimeZone(input)).toEqual({
        kind: 'unsupported',
        reason: 'alias',
      })
    }
  )

  test('does not silently use UTC for a region timezone absent from browser tzdata', () => {
    const DateTimeFormat = Intl.DateTimeFormat
    vi.spyOn(Intl, 'DateTimeFormat').mockImplementation(function (
      locales?: Intl.LocalesArgument,
      options?: Intl.DateTimeFormatOptions
    ) {
      if (options?.timeZone === 'America/Coyhaique') {
        throw new RangeError('unsupported in this browser tzdata')
      }
      return new DateTimeFormat(locales, options)
    })

    expect(resolveBillingTimeZone('America/Coyhaique')).toEqual({
      kind: 'unsupported',
      reason: 'browser-unavailable',
    })
  })

  test('reports an unrecognized region path as unsupported without guessing whether Go accepts it', () => {
    expect(resolveBillingTimeZone('Asia/NotAZone')).toEqual({
      kind: 'unsupported',
      reason: 'browser-unavailable',
    })
  })
})
