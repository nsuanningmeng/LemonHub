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
  buildRequestRuleExpr,
  combineBillingExpr,
  getRequestRuleMatchOptions,
  MATCH_EQ,
  MATCH_LTE,
  normalizeCondition,
  type ParamHeaderCondition,
  type RequestRuleGroup,
  requestRuleGroupsFromTrace,
  splitBillingExprAndRequestRules,
  tryParseRequestRuleExpr,
} from '../billing-expr'

function fastGroup(
  overrides: Partial<ParamHeaderCondition> = {}
): RequestRuleGroup {
  return {
    conditions: [
      { source: 'param', path: 'service_tier', mode: MATCH_EQ, value: 'fast' },
      {
        source: 'token',
        path: 'len',
        mode: MATCH_LTE,
        value: '272000',
        ...overrides,
      },
    ],
    multiplier: '2.5',
  }
}

describe('token restrictions in request multipliers', () => {
  test('preserves the full-input limit alongside a Fast service-tier condition', () => {
    const group = fastGroup()
    const expr = buildRequestRuleExpr([group])

    expect(expr).toBe(
      '(param("service_tier") == "fast" && len <= 272000 ? 2.5 : 1)'
    )
    expect(tryParseRequestRuleExpr(expr)).toEqual([group])
  })

  test.each([
    '(len == 272000 ? 2 : 1)',
    '(p > 0 ? 2 : 1)',
    '(c >= 1e3 ? 2 : 1)',
    '(len < 272001 ? 2 : 1)',
    '(len <= 272000 ? 2 : 1)',
  ])(
    'round-trips the numeric token comparison without changing it: %s',
    (expr) => {
      const groups = tryParseRequestRuleExpr(expr)

      expect(groups).not.toBeNull()
      expect(buildRequestRuleExpr(groups ?? [])).toBe(expr)
    }
  )

  test('splits and rebuilds a tiered expression without losing its Fast limit', () => {
    const base =
      'len <= 272000 ? tier("standard", p * 5 + c * 30 + cr * 0.5) : tier("long_context", p * 10 + c * 45 + cr * 1)'
    const rules =
      '(param("service_tier") == "priority" && len <= 272000 ? 2.5 : 1) * (param("service_tier") == "fast" && len <= 272000 ? 2.5 : 1) * (param("service_tier") == "flex" ? 0.5 : 1)'
    const expr = combineBillingExpr(base, rules)
    const split = splitBillingExprAndRequestRules(expr)

    expect(split).toEqual({ billingExpr: base, requestRuleExpr: rules })
    const parsed = tryParseRequestRuleExpr(split.requestRuleExpr)
    expect(parsed).not.toBeNull()
    expect(
      combineBillingExpr(split.billingExpr, buildRequestRuleExpr(parsed ?? []))
    ).toBe(expr)
  })

  test('keeps the context restriction when displaying a settled rule trace', () => {
    const groups = requestRuleGroupsFromTrace([
      {
        cond: 'param("service_tier") == "fast" && len <= 272000',
        multiplier: 2.5,
        matched: true,
      },
    ])

    expect(groups[0].conditions).toEqual(fastGroup().conditions)
    expect(groups[0].matched).toBe(true)
  })

  test('normalizes token conditions without changing their source or numeric limit', () => {
    expect(normalizeCondition(fastGroup().conditions[1])).toEqual({
      source: 'token',
      path: 'len',
      mode: MATCH_LTE,
      value: '272000',
    })
    expect(
      getRequestRuleMatchOptions('token').map((option) => option.value)
    ).toEqual(['eq', 'gt', 'gte', 'lt', 'lte'])
  })

  test.each([
    { path: 'cc' },
    { path: 'len || true' },
    { path: '' },
    { value: '"272000"' },
    { value: 'fast' },
    { value: 'true' },
    { value: '' },
    { value: '1e999' },
    { mode: 'contains' },
  ])(
    'does not broaden Fast billing when its token restriction is invalid: %o',
    (overrides) => {
      expect(buildRequestRuleExpr([fastGroup(overrides)])).toBe('')
    }
  )

  test.each([
    '(cc <= 272000 ? 2 : 1)',
    '(len == "272000" ? 2 : 1)',
    '(len <= true ? 2 : 1)',
    '(len <= 1e999 ? 2 : 1)',
    '(len <= 272000 || param("service_tier") == "fast" ? 2 : 1)',
    '(len + p <= 272000 ? 2 : 1)',
  ])(
    'keeps unsupported or invalid token expressions in raw mode: %s',
    (expr) => {
      expect(tryParseRequestRuleExpr(expr)).toBeNull()
    }
  )
})
