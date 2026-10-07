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
import { cleanup, render, screen } from '@testing-library/react'
import { expect, test, vi } from 'vitest'

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

test('compact model card shows the actual rate condition while remaining expression-priced', () => {
  const guard = 'weekday("UTC") == 1 || hour("UTC") < 9'
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValue({ data: { success: true, data: {} } })
  render(
    <QueryClientProvider client={client}>
      <ModelCard
        model={model(
          `${guard} ? tier("discount", p * 1 + c * 2) : tier("base", p * 3 + c * 6)`
        )}
        onClick={() => {}}
      />
    </QueryClientProvider>
  )
  expect(screen.getByText(guard)).toBeInTheDocument()
  expect(screen.getByText('Dynamic Pricing')).toBeInTheDocument()
  cleanup()
  client.clear()
  get.mockRestore()
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
