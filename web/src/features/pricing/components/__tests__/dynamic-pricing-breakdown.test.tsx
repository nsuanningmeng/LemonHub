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
import { render, screen } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import { DynamicPricingBreakdown } from '../dynamic-pricing-breakdown'

describe('DynamicPricingBreakdown', () => {
  test('normalizes matched tier labels in both mobile and desktop views', () => {
    render(
      <DynamicPricingBreakdown
        billingExpr='tier("Input ≤ 1K", p * 0.001 + c * 0.002)'
        matchedTierLabel=' Input <= 1K '
      />
    )

    expect(screen.getAllByText('Matched')).toHaveLength(2)
  })

  test('shows a token-length restriction together with its Fast surcharge', () => {
    render(
      <DynamicPricingBreakdown billingExpr='(tier("standard", p * 5 + c * 30 + cr * 0.5)) * (param("service_tier") == "fast" && len <= 272000 ? 2.5 : 1)' />
    )

    expect(
      screen.getByText(/service_tier.*fast.*Full input length.*272000/)
    ).toBeInTheDocument()
    expect(screen.getByText('2.5x')).toBeInTheDocument()
  })
})

const timeTierExpression =
  'hour("Asia/Shanghai") >= 9 && hour("Asia/Shanghai") < 18 ? tier("peak", p * 3 + c * 15 + cr * 0.3) : tier("off-peak", p * 1.5 + c * 7.5 + cr * 0.15)'

test.each([false, true])(
  'time tiers preserve guards and matched tier in compact=%s layouts',
  (compact) => {
    render(
      <DynamicPricingBreakdown
        billingExpr={timeTierExpression}
        matchedTierLabel='off-peak'
        compact={compact}
      />
    )
    expect(
      screen.getAllByText(/Hour.*09:00.*18:00.*Asia\/Shanghai/)
    ).toHaveLength(2)
    expect(screen.getAllByText(/!.*hour.*Asia\/Shanghai/)).toHaveLength(2)
    expect(screen.getAllByText('Matched')).toHaveLength(2)
    for (const rate of [3, 15, 0.3, 1.5, 7.5, 0.15]) {
      expect(screen.getAllByText(`$${rate.toFixed(4)}`)).toHaveLength(2)
    }
    expect(
      screen.queryByText('Special billing expression')
    ).not.toBeInTheDocument()
  }
)

test('unsupported price structures fall back to the complete expression, including conditions', () => {
  const expression =
    'hour("UTC") >= 9 ? tier("peak", max(p * 3, 5)) : tier("off", p * 1)'
  render(<DynamicPricingBreakdown billingExpr={expression} compact />)
  expect(screen.getByText(expression)).toBeInTheDocument()
  expect(screen.queryByText('Tiered price table')).not.toBeInTheDocument()
})

test('a fixed linear expression keeps the breakdown route with a truthful fixed-price heading', () => {
  render(<DynamicPricingBreakdown billingExpr='tier("base", p * 3 + c * 15)' />)
  expect(screen.getByText('Token-based')).toBeInTheDocument()
  expect(
    screen.getByText('Prices are defined by this billing expression')
  ).toBeInTheDocument()
  expect(
    screen.queryByText('Prices vary by usage tier and request conditions')
  ).not.toBeInTheDocument()
})

test('OR paths retain the complete predicate instead of flattening it into an unconditional tier', () => {
  const condition =
    'weekday("UTC") == 1 && (hour("UTC") >= 9 && hour("UTC") < 12 || hour("UTC") >= 14 && hour("UTC") < 18)'
  render(
    <DynamicPricingBreakdown
      billingExpr={`${condition} ? tier("work", p * 3) : tier("other", p * 1)`}
    />
  )
  expect(screen.getAllByText(condition)).toHaveLength(2)
  expect(screen.getAllByText(`!(${condition})`)).toHaveLength(2)
})

test('unsupported request multipliers preserve the complete raw expression even with a recorded empty trace', () => {
  const expression = '(tier("base", p * 3)) * mystery(header("x-private"))'
  render(<DynamicPricingBreakdown billingExpr={expression} requestRules={[]} />)
  expect(screen.getByText(expression)).toBeInTheDocument()
  expect(screen.queryByText('Tiered price table')).not.toBeInTheDocument()
})

test.each(['Local', 'Invalid/Zone', 'asia/shanghai'])(
  'unsupported tier timezone %s retains the full raw contract',
  (zone) => {
    const expression = `hour("${zone}") >= 9 ? tier("peak", p * 3) : tier("off", p * 1)`
    render(<DynamicPricingBreakdown billingExpr={expression} />)
    expect(screen.getByText(expression)).toBeInTheDocument()
    expect(screen.queryByText('Tiered price table')).not.toBeInTheDocument()
  }
)
