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
import { afterEach, describe, expect, test, vi } from 'vitest'

import { evalExprLocally, type ExtraTokenValues } from '../tier-expr'

const extras: ExtraTokenValues = {
  cacheReadTokens: 0,
  cacheCreateTokens: 0,
  cacheCreate1hTokens: 0,
  cacheCreate12hTokens: 0,
  imageTokens: 0,
  imageOutputTokens: 0,
  audioInputTokens: 0,
  audioOutputTokens: 0,
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('billing expression preview security', () => {
  test.each([
    'globalThis.pricingPreviewProbe()',
    'window.pricingPreviewProbe()',
    'fetch("/api/user/self", { method: "DELETE" })',
    'tier.constructor("return globalThis.pricingPreviewProbe()")()',
    '({}).constructor.constructor("return globalThis.pricingPreviewProbe()")()',
    '(function () { return globalThis.pricingPreviewProbe() })()',
    '(() => globalThis.pricingPreviewProbe())()',
  ])('rejects executable JavaScript without side effects: %s', (expression) => {
    const probe = vi.fn(() => 1)
    const fetch = vi.fn(() => 1)
    vi.stubGlobal('pricingPreviewProbe', probe)
    vi.stubGlobal('fetch', fetch)

    const result = evalExprLocally(expression, 100, 10, extras)

    expect(probe).not.toHaveBeenCalled()
    expect(fetch).not.toHaveBeenCalled()
    expect(result.error).toBeTruthy()
    expect(result.cost).toBe(0)
  })

  test.each([
    'true ? tier("base", p) : globalThis.pricingPreviewProbe()',
    'true ? tier("base", p) : unknownTime("UTC")',
    'false && constructor("return 1")',
    'true ? tier("base", p) : tier.constructor("return 1")()',
  ])('validates rejected syntax even in an unused branch: %s', (expression) => {
    expect(evalExprLocally(expression, 100, 10, extras)).toMatchObject({
      cost: 0,
      matchedTier: '',
      error: expect.any(String),
    })
  })

  test.each([
    'p = 1',
    '[p, c][0]',
    '({ cost: p }).cost',
    'let x = p; x',
    'Math.max(p, c)',
    'p; 42',
    'constructor("return 1")',
    'toString()',
    '__proto__',
  ])('rejects syntax outside the preview subset: %s', (expression) => {
    expect(evalExprLocally(expression, 100, 10, extras).error).toBeTruthy()
  })

  test.each(['1 / 0', '0 / 0', '1e308 * 1e308', '"100"', 'true', '-1'])(
    'reports invalid numeric costs instead of showing a price: %s',
    (expression) => {
      expect(evalExprLocally(expression, 100, 10, extras)).toMatchObject({
        cost: 0,
        matchedTier: '',
        error: expect.any(String),
      })
    }
  )

  test('preserves arithmetic precedence, unary signs, math functions and exponent literals', () => {
    expect(
      evalExprLocally(
        'tier("math", max(p * 2 + c, min(500, 2e3)) + abs(-5) + ceil(1.1) + floor(1.9) + +2 - 6 / 3)',
        100,
        10,
        extras
      )
    ).toEqual({ cost: 508, matchedTier: 'math', error: null })
  })

  test('preserves remainder and string comparisons in tier conditions', () => {
    expect(
      evalExprLocally(
        'p % 3 == 1 && "base" != "free" ? tier("base", 2) : tier("free", 0)',
        100,
        10,
        extras
      )
    ).toEqual({ cost: 2, matchedTier: 'base', error: null })
  })

  test('preserves lazy conditions and boolean short circuiting', () => {
    expect(
      evalExprLocally(
        '!(p < 100) && (true || hour("Local") > 0) ? tier("selected", 12) : tier("unused", hour("Local"))',
        100,
        10,
        extras
      )
    ).toEqual({ cost: 12, matchedTier: 'selected', error: null })
  })

  test('supports the documented v1 prefix but rejects unknown versions', () => {
    expect(
      evalExprLocally('v1:tier("base", p * 2 + c)', 100, 10, extras)
    ).toEqual({ cost: 210, matchedTier: 'base', error: null })
    expect(
      evalExprLocally('v2:tier("base", p * 2)', 100, 10, extras).error
    ).toBeTruthy()
  })

  test('returns an explicit error for expressions too large or too deeply nested to preview', () => {
    for (const expression of [
      `${'('.repeat(300)}p${')'.repeat(300)}`,
      `${'p + '.repeat(10000)}c`,
    ]) {
      expect(evalExprLocally(expression, 100, 10, extras)).toMatchObject({
        cost: 0,
        error: expect.stringMatching(/too (large|deep|complex)/i),
      })
    }
  })
})
