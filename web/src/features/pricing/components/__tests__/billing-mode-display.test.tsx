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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import {
  getDynamicPricingSummary,
  isDynamicPricingModel,
} from '../../lib/dynamic-price'
import type { PricingModel } from '../../types'
import { ModelBillingModeBadge } from '../model-billing-mode-badge'
import { ModelCard } from '../model-card'

function model(expression: string): PricingModel {
  return {
    id: 1,
    model_name: 'test-model',
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: ['default'],
    billing_mode: 'tiered_expr',
    billing_expr: expression,
  }
}

test.each([
  ['tier("base", p * 3 + c * 15 + cr * 0.3)', 'Token-based'],
  ['p * 3 + c * 15', 'Token-based'],
  ['tier("request", 5)', 'Per Request'],
  ['max(p * 3, 5)', 'Dynamic Pricing'],
  [
    'hour("UTC") < 9 ? tier("off", p * 1) : tier("peak", p * 3)',
    'Dynamic Pricing',
  ],
  [
    '(tier("base", p * 3)) * (header("x-tier") == "fast" ? 2 : 1)',
    'Dynamic Pricing',
  ],
  ['mystery(p)', 'Dynamic Pricing'],
])(
  'badge %s changes presentation while retaining expression routing',
  (expression, label) => {
    const item = model(expression)
    render(<ModelBillingModeBadge model={item} />)
    expect(screen.getByText(label)).toBeInTheDocument()
    expect(isDynamicPricingModel(item)).toBe(true)
    expect(item.billing_mode).toBe('tiered_expr')
  }
)

describe('compact model card pricing', () => {
  let client: QueryClient

  beforeEach(() => {
    client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: {} },
    })
  })

  afterEach(() => {
    client.clear()
  })

  test.each([
    {
      name: 'time-dependent tiers',
      expression:
        'weekday("UTC") == 1 || hour("UTC") < 9 ? tier("discount", p * 1 + c * 2) : tier("base", p * 3 + c * 6)',
      outputPrice: '$2',
      condition: /weekday\("UTC"\)|hour\("UTC"\)/,
    },
    {
      name: 'long service-tier multipliers',
      expression:
        '(len <= 200000 ? tier("standard", p * 1 + c * 3) : tier("long", p * 2 + c * 6)) * (param("service_tier") == "priority" && len <= 272000 ? 2 : 1) * (param("service_tier") == "fast" && len <= 200000 ? 2 : 1) * (param("service_tier") == "flex" ? 0.5 : 1)',
      outputPrice: '$3',
      condition: /len <=|service_tier/,
    },
  ])(
    '$name keep prices and a dynamic badge without exposing raw conditions',
    async ({ expression, outputPrice, condition }) => {
      const user = userEvent.setup()
      const onDetails = vi.fn()
      render(
        <QueryClientProvider client={client}>
          <ModelCard model={model(expression)} onClick={onDetails} />
        </QueryClientProvider>
      )

      expect(screen.getByText('Input')).toHaveTextContent('Input $1')
      expect(screen.getByText('Output')).toHaveTextContent(
        `Output ${outputPrice}`
      )
      expect(screen.getByText('Dynamic Pricing')).toBeInTheDocument()
      expect(screen.queryByText(condition)).not.toBeInTheDocument()

      await user.click(screen.getByRole('button', { name: 'Details' }))
      expect(onDetails).toHaveBeenCalledOnce()
    }
  )

  test('unrecognized pricing keeps the special billing label without exposing its expression', () => {
    const expression = 'max(p * 3, 5)'
    render(
      <QueryClientProvider client={client}>
        <ModelCard model={model(expression)} onClick={() => {}} />
      </QueryClientProvider>
    )

    expect(screen.getByText('Special billing expression')).toBeInTheDocument()
    expect(screen.getByText('Dynamic Pricing')).toBeInTheDocument()
    expect(screen.queryByText(expression)).not.toBeInTheDocument()
  })
})

test('group/unit/recharge multipliers and cache coefficients retain their existing arithmetic', () => {
  const summary = getDynamicPricingSummary(
    model('tier("base", p * 3 + c * 15 + cr * 0.3)'),
    {
      tokenUnit: 'K',
      groupRatioMultiplier: 2,
      showRechargePrice: true,
      priceRate: 3,
      usdExchangeRate: 6,
    }
  )
  expect(
    summary?.entries.find((entry) => entry.variable.key === 'p')?.formatted
  ).toContain('0.003')
  expect(
    summary?.entries.find((entry) => entry.variable.key === 'cr')?.formatted
  ).toContain('0.0003')
  expect(summary?.tier?.inputPrice).toBe(3)
  expect(summary?.tier?.cacheReadPrice).toBe(0.3)
})
