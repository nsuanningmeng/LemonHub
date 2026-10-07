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
  classifyBillingExpression,
  parseTiersFromExpr,
  splitBillingExprAndRequestRules,
} from '../billing-expr'

const peakCondition = 'hour("Asia/Shanghai") >= 9 && hour("Asia/Shanghai") < 18'
const peakExpression = `${peakCondition} ? tier("高峰时段", p * 3.5 + cr * 0.15 + c * 9.5) : tier("空闲时段", p * 2 + cr * 0.1 + c * 5)`

describe('complete billing tier parsing', () => {
  test('time tiers preserve all six prices and both actual branch guards', () => {
    const tiers = parseTiersFromExpr(peakExpression)
    expect(tiers).toHaveLength(2)
    expect(tiers[0]).toMatchObject({
      label: '高峰时段',
      inputPrice: 3.5,
      outputPrice: 9.5,
      cacheReadPrice: 0.15,
      conditionText: peakCondition,
    })
    expect(tiers[1]).toMatchObject({
      label: '空闲时段',
      inputPrice: 2,
      outputPrice: 5,
      cacheReadPrice: 0.1,
      conditionText: `!(${peakCondition})`,
    })
  })

  test('nested weekday and overnight branches retain parent and else predicates', () => {
    const weekday = 'weekday("UTC") >= 1 && weekday("UTC") <= 5'
    const overnight = 'hour("UTC") >= 21 || hour("UTC") < 6'
    const tiers = parseTiersFromExpr(
      `${weekday} ? (${overnight} ? tier("night", p * 1 + c * 2) : tier("day", p * 3 + c * 4)) : tier("weekend", p * 5 + c * 6)`
    )
    expect(tiers.map((tier) => tier.conditionText)).toEqual([
      `(${weekday}) && (${overnight})`,
      `(${weekday}) && !(${overnight})`,
      `!(${weekday})`,
    ])
    expect(tiers.map((tier) => tier.inputPrice)).toEqual([1, 3, 5])
  })

  test.each([
    'tier("partial", p * 2 + c * 3) + 1000000',
    'tier("partial", max(p * 2, c * 3))',
    'tier("partial", p * 2 + 1000000)',
    'tier("partial", p * 2 + c * p)',
    'unknown("UTC") > 3 ? tier("partial", p * 2) : tier("other", p * 1)',
    'hour("Unknown/Zone") >= 9 ? tier("partial", p * 2) : tier("other", p * 1)',
    'hour("asia/shanghai") >= 9 ? tier("partial", p * 2) : tier("other", p * 1)',
    'hour("AMERICA/NEW_YORK") >= 9 ? tier("partial", p * 2) : tier("other", p * 1)',
    'hour("Local") >= 9 ? tier("partial", p * 2) : tier("other", p * 1)',
    '(tier("partial", p * 2)) * (hour("asia/shanghai") >= 9 ? 2 : 1)',
    '(tier("partial", p * 2)) * (hour("Local") >= 9 ? 2 : 1)',
    'p > 3 ? tier("partial", p * 2)',
    'tier("partial", p * 2) trailing',
    'v2:tier("partial", p * 2)',
  ])(
    'unsupported complete expression falls back instead of extracting a tier: %s',
    (expr) => {
      expect(parseTiersFromExpr(expr)).toEqual([])
    }
  )

  test('repeated linear terms add instead of silently keeping the first coefficient', () => {
    expect(
      parseTiersFromExpr('tier("linear", p * 2 + p * 3 + c * 0)')[0]
    ).toMatchObject({ inputPrice: 5, outputPrice: 0 })
  })

  test('parentheses and multiplication characters inside labels cannot split the formula', () => {
    const base = 'tier("literal ) * ( ? : label", p * 2 + c * 3)'
    const rule = '(header("plan") == "premium ) * (" ? 2 : 1)'
    expect(splitBillingExprAndRequestRules(`(${base}) * ${rule}`)).toEqual({
      billingExpr: base,
      requestRuleExpr: rule,
    })
    expect(parseTiersFromExpr(base)[0]).toMatchObject({
      label: 'literal ) * ( ? : label',
      inputPrice: 2,
      outputPrice: 3,
    })
  })

  test('a request multiplier in one ternary branch cannot become a global price rule', () => {
    const expr =
      'len > 10 ? tier("large", p * 3) : tier("small", p * 2) * (header("plan") == "fast" ? 2 : 1)'
    expect(splitBillingExprAndRequestRules(expr)).toEqual({
      billingExpr: expr,
      requestRuleExpr: '',
    })
    expect(parseTiersFromExpr(expr)).toEqual([])
  })

  test('versioned request rules keep their complete multiplier for consumers', () => {
    const base = 'tier("standard", p * 2 + c * 3)'
    const rule = '(header("plan") == "fast" ? 2 : 1)'
    expect(splitBillingExprAndRequestRules(`v1:(${base}) * ${rule}`)).toEqual({
      billingExpr: `v1:${base}`,
      requestRuleExpr: rule,
    })
    expect(parseTiersFromExpr(`v1:(${base}) * ${rule}`)).toHaveLength(1)
  })

  test('a recognized peak guard exposes one exact time range and its fallback stays complete', () => {
    const tiers = parseTiersFromExpr(peakExpression)
    expect(tiers[0].displayConditions).toEqual([
      {
        source: 'time',
        timeFunc: 'hour',
        timezone: 'Asia/Shanghai',
        mode: 'range',
        value: '',
        rangeStart: '9',
        rangeEnd: '18',
      },
    ])
    expect(tiers[1].displayConditions).toBeUndefined()
    expect(tiers[1].conditionText).toBe(`!(${peakCondition})`)
  })

  test('linear prices safely support parentheses, constant scaling and duplicate terms', () => {
    const tiers = parseTiersFromExpr(
      'v1:tier("scaled", (2 * p + p) / 2 + c * 4e-1 + cr * 0)'
    )
    expect(tiers[0]).toMatchObject({
      inputPrice: 1.5,
      outputPrice: 0.4,
      cacheReadPrice: 0,
    })
  })

  test('escaped labels and guard operators remain data instead of syntax', () => {
    const label = 'quoted " ? : \\ text'
    expect(
      parseTiersFromExpr(`tier(${JSON.stringify(label)}, p * 2)`)[0].label
    ).toBe(label)
  })

  test('an empty timezone keeps the raw predicate but displays backend UTC', () => {
    const tiers = parseTiersFromExpr(
      'hour("") >= 9 ? tier("day", p * 2) : tier("other", p * 1)'
    )
    expect(tiers[0].conditionText).toBe('hour("") >= 9')
    expect(tiers[0].displayConditions).toEqual([
      {
        source: 'time',
        timeFunc: 'hour',
        timezone: 'UTC',
        mode: 'gte',
        value: '9',
        rangeStart: '',
        rangeEnd: '',
      },
    ])
  })
})

describe('billing expression display classification', () => {
  test.each([
    ['tier("base", p * 2 + c * 3 + cr * 0)', 'token'],
    ['v1:p * 2 + c * 3', 'token'],
    ['tier("free", p * 0 + c * 0)', 'token'],
    ['tier("call", 1000000)', 'request'],
    ['0', 'request'],
    [peakExpression, 'dynamic'],
    ['(tier("base", p * 2)) * (header("plan") == "fast" ? 2 : 1)', 'dynamic'],
    ['tier("nonlinear", max(p * 2, 5))', 'dynamic'],
    ['tier("mixed", p * 2 + 1000000)', 'dynamic'],
    ['tier("request", param("n") * 1000000)', 'dynamic'],
    ['tier("time", hour("UTC") * 2)', 'dynamic'],
    ['tier("invalid", p * -2)', 'dynamic'],
    ['tier("overflow", p * 1e309)', 'dynamic'],
    ['v99:tier("future", p * 2)', 'dynamic'],
    ['tier("partial", p * 2) unknown', 'dynamic'],
  ] as const)(
    '%s displays as %s without changing its execution mode',
    (expr, expected) => {
      expect(classifyBillingExpression(expr)).toBe(expected)
    }
  )
})
