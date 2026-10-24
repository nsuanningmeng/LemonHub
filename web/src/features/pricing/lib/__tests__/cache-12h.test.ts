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
import { expect, test } from 'vitest'

import { getTieredBillingSummary } from '@/features/usage-logs/lib/format'

import { parseTiersFromExpr } from '../billing-expr'
import { getDynamicPriceEntries } from '../dynamic-price'
import {
  evalExprLocally,
  generateExprFromVisualConfig,
  tryParseVisualConfig,
} from '../tier-expr'

const tokens = {
  cacheReadTokens: 0,
  cacheCreateTokens: 10,
  cacheCreate1hTokens: 20,
  cacheCreate12hTokens: 30,
  imageTokens: 0,
  imageOutputTokens: 0,
  audioInputTokens: 0,
  audioOutputTokens: 0,
}

test('preserves an explicit 12-hour cache price through visual editing', () => {
  const expr = 'tier("base", p * 5 + c * 20 + cc * 6.25 + cc12h * 12.5)'
  const parsed = tryParseVisualConfig(expr)
  expect(parsed?.tiers[0].cache_create_12h_unit_cost).toBe(12.5)
  expect(generateExprFromVisualConfig(parsed)).toBe(expr)
  expect(parseTiersFromExpr(expr)[0].cacheCreate12hPrice).toBe(12.5)
})

test.each([
  ['p + c + cc + cc1h + cc12h', 70],
  ['p + c + cc', 112],
  ['p + c', 112],
  ['len == 70 ? tier("cc12h", p + c + cc) : 0', 112],
  ['p + c + cc + cc12h * 0', 52],
])(
  'estimates cache fallback using actual variable references: %s',
  (expr, expected) => {
    const result = evalExprLocally(expr, 10, 0, tokens)
    expect(result.error).toBeNull()
    expect(result.cost).toBe(expected)
  }
)

test('preserves an explicitly free 12-hour price when regenerating an expression', () => {
  const expr = 'tier("base", p * 5 + c * 20 + cc * 6.25 + cc12h * 0)'
  expect(generateExprFromVisualConfig(tryParseVisualConfig(expr))).toBe(expr)
})

test.each([false, true])(
  'distinguishes an omitted 12-hour price from explicit free pricing (%s)',
  (explicitFree) => {
    const expression = `tier("base", p * 5 + c * 20 + cc * 6.25${explicitFree ? ' + cc12h * 0' : ''})`
    const entries = getDynamicPriceEntries(parseTiersFromExpr(expression)[0], {
      tokenUnit: 'M',
    })
    const longCache = entries.find((entry) => entry.key === 'cc12h')
    expect(longCache?.value).toBe(explicitFree ? 0 : undefined)
    expect(entries.find((entry) => entry.key === 'cc')?.shortLabel).toBe(
      explicitFree ? 'Cache Write (30m)' : 'Cache Write'
    )
    const summary = getTieredBillingSummary({
      billing_mode: 'tiered_expr',
      expr_b64: btoa(expression),
      matched_tier: 'base',
      cache_creation_tokens: 10,
    })
    expect(
      summary?.priceEntries.find(
        (entry) => entry.field === 'cacheCreate12hPrice'
      )?.price
    ).toBe(explicitFree ? 0 : undefined)
  }
)

test.each(['cc', 'cc1h', 'cr'])(
  'keeps a legacy explicit free %s coefficient in the raw editor when visual editing would remove it',
  (name) => {
    expect(
      tryParseVisualConfig(`tier("base", p * 5 + c * 20 + ${name} * 0)`)
    ).toBeNull()
  }
)
