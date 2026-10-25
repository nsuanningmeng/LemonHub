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
import { afterEach, beforeEach, describe, expect, test } from 'vitest'

import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { formatRewardClaimedAmount } from '../lib/claimed-amount'

const originalConfig = useSystemConfigStore.getState().config

beforeEach(() => {
  useSystemConfigStore.getState().setConfig({
    currency: { ...DEFAULT_CURRENCY_CONFIG, quotaPerUnit: 100 },
  })
})

afterEach(() => {
  useSystemConfigStore.getState().setConfig(originalConfig)
})

describe('exact cumulative reward claim formatting', () => {
  test('prefers the exact integer string over an already rounded numeric total', () => {
    expect(
      formatRewardClaimedAmount(
        {
          claimed_quota: 9007199254740992,
          claimed_quota_exact: '9007199254740993',
        },
        'en-US'
      )
    ).toBe('$90,071,992,547,409.93')
  })

  test.each([
    ['90071992547409935', '$90,071,992,547,409.94'],
    ['999999999999999995', '$1,000,000,000,000,000'],
  ])(
    'rounds cumulative quota %s half-up without losing digits',
    (quota, expected) => {
      useSystemConfigStore.getState().setConfig({
        currency: { ...DEFAULT_CURRENCY_CONFIG, quotaPerUnit: 1000 },
      })
      expect(
        formatRewardClaimedAmount(
          { claimed_quota: null, claimed_quota_exact: quota },
          'en-US'
        )
      ).toBe(expected)
    }
  )

  test('converts CNY with the decimal exchange rate before rounding', () => {
    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaPerUnit: 100,
        quotaDisplayType: 'CNY',
        usdExchangeRate: 1.25,
      },
    })
    expect(
      formatRewardClaimedAmount(
        { claimed_quota: null, claimed_quota_exact: '900719925474099300' },
        'en-US'
      )
    ).toBe('¥11,258,999,068,426,241.25')
  })

  test('uses the custom currency symbol and configured custom rate', () => {
    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaPerUnit: 100,
        quotaDisplayType: 'CUSTOM',
        customCurrencySymbol: '€',
        customCurrencyExchangeRate: 0.75,
      },
    })
    expect(
      formatRewardClaimedAmount(
        { claimed_quota: null, claimed_quota_exact: '900719925474099300' },
        'en-US'
      )
    ).toBe('€ 6,755,399,441,055,744.75')
  })

  test('token mode preserves every digit of the raw quota total', () => {
    useSystemConfigStore.getState().setConfig({
      currency: { ...DEFAULT_CURRENCY_CONFIG, quotaDisplayType: 'TOKENS' },
    })
    expect(
      formatRewardClaimedAmount(
        { claimed_quota: null, claimed_quota_exact: '9007199254740993' },
        'en-US'
      )
    ).toBe('9007199254740993')
  })

  test('localized currency output preserves exact decimals and grouping', () => {
    expect(
      formatRewardClaimedAmount(
        { claimed_quota: null, claimed_quota_exact: '9007199254740993' },
        'de-DE'
      )
    ).toBe('90.071.992.547.409,93 $')
  })

  test('legacy safe totals and zero remain compatible without an exact field', () => {
    expect(formatRewardClaimedAmount({ claimed_quota: 375 }, 'en-US')).toBe(
      '$3.75'
    )
    expect(formatRewardClaimedAmount({ claimed_quota: 0 }, 'en-US')).toBe('$0')
  })

  test('USD ignores a local exchange rate and supports decimal quota units', () => {
    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaPerUnit: 0.25,
        usdExchangeRate: 7,
      },
    })
    expect(
      formatRewardClaimedAmount(
        { claimed_quota: 1, claimed_quota_exact: '1' },
        'en-US'
      )
    ).toBe('$4')
  })

  test('scientific-notation rates preserve the standard minimum nonzero display', () => {
    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaPerUnit: 1,
        quotaDisplayType: 'CUSTOM',
        customCurrencySymbol: '¤',
        customCurrencyExchangeRate: 1e-7,
      },
    })
    expect(formatRewardClaimedAmount({ claimed_quota: 1 }, 'en-US')).toBe(
      '¤ 0.0001'
    )
  })

  test.each(['', '-1', '1.5', '1e3'])(
    'invalid exact quota %j is never displayed as a zero claim total',
    (quota) => {
      expect(
        formatRewardClaimedAmount(
          { claimed_quota: 0, claimed_quota_exact: quota },
          'en-US'
        )
      ).toBe('-')
    }
  )

  test('missing exact data never rounds an unsafe legacy total or invents zero', () => {
    expect(
      formatRewardClaimedAmount({ claimed_quota: 9007199254740992 }, 'en-US')
    ).toBe('-')
    expect(formatRewardClaimedAmount({ claimed_quota: null }, 'en-US')).toBe(
      '-'
    )
  })
})
