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
import { expect, test, vi } from 'vitest'

import { filterByQuotaType } from '../../lib/filters'
import type { PricingModel } from '../../types'
import { PricingSidebar } from '../pricing-sidebar'

test.each<{
  name: string
  pricing: Partial<PricingModel>
  category: 'token' | 'request'
}>([
  {
    name: 'nested constant tiers remain per request',
    pricing: {
      billing_mode: 'tiered_expr',
      billing_expr:
        'hour("UTC") < 12 ? (len < 1000 ? tier("short", 5) : tier("long", 10)) : tier("night", 2)',
    },
    category: 'request',
  },
  {
    name: 'mixed nested tiers override a legacy request category',
    pricing: {
      quota_type: 1,
      billing_mode: 'tiered_expr',
      billing_expr:
        'hour("UTC") < 12 ? (len < 1000 ? tier("short", 5) : tier("long", p * 2 + c * 8)) : tier("night", 2)',
    },
    category: 'token',
  },
  {
    name: 'token tiers with request multipliers override a legacy request category',
    pricing: {
      quota_type: 1,
      billing_mode: 'tiered_expr',
      billing_expr:
        '(len < 1000 ? tier("short", p * 2) : tier("long", p * 4)) * (header("x-fast") == "true" ? 2 : 1)',
    },
    category: 'token',
  },
  {
    name: 'unsupported pricing leaves do not infer a category from a partial tree',
    pricing: {
      quota_type: 1,
      billing_mode: 'tiered_expr',
      billing_expr:
        'len < 1000 ? tier("short", p * 2) : tier("long", mystery(p))',
    },
    category: 'request',
  },
  { name: 'legacy token pricing', pricing: {}, category: 'token' },
  {
    name: 'legacy request pricing',
    pricing: { quota_type: 1 },
    category: 'request',
  },
  {
    name: 'constant expression overrides a token quota type',
    pricing: {
      billing_mode: 'tiered_expr',
      billing_expr: 'tier("request", 5)',
    },
    category: 'request',
  },
  {
    name: 'free constant expression remains request based',
    pricing: {
      billing_mode: 'tiered_expr',
      billing_expr: 'v1:tier("free", 0)',
    },
    category: 'request',
  },
  {
    name: 'token expression overrides a request quota type',
    pricing: {
      quota_type: 1,
      billing_mode: 'tiered_expr',
      billing_expr: 'tier("base", p * 2 + c * 8)',
    },
    category: 'token',
  },
  {
    name: 'unrecognized expression retains its existing category',
    pricing: {
      quota_type: 1,
      billing_mode: 'tiered_expr',
      billing_expr: 'mystery(p)',
    },
    category: 'request',
  },
  {
    name: 'inactive expression does not override fixed pricing',
    pricing: { quota_type: 1, billing_expr: 'tier("base", p * 2)' },
    category: 'request',
  },
  {
    name: 'tier labels are not interpreted as pricing syntax',
    pricing: {
      billing_mode: 'tiered_expr',
      billing_expr: 'tier("fixed(5)", p * 2)',
    },
    category: 'token',
  },
])('$name has matching results and sidebar counts', ({ pricing, category }) => {
  const model: PricingModel = {
    id: 1,
    model_name: 'test-model',
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: ['default'],
    ...pricing,
  }
  render(
    <PricingSidebar
      quotaTypeFilter='all'
      endpointTypeFilter='all'
      vendorFilter='all'
      groupFilter='all'
      tagFilter='all'
      onQuotaTypeChange={vi.fn()}
      onEndpointTypeChange={vi.fn()}
      onVendorChange={vi.fn()}
      onGroupChange={vi.fn()}
      onTagChange={vi.fn()}
      vendors={[]}
      groups={[]}
      tags={[]}
      models={[model]}
      hasActiveFilters={false}
      onClearFilters={vi.fn()}
    />
  )
  for (const [value, label] of [
    ['token', 'Token-based'],
    ['request', 'Per Request'],
  ]) {
    const matches = value === category
    expect(filterByQuotaType([model], value)).toEqual(matches ? [model] : [])
    expect(
      screen.getByRole('button', {
        name: new RegExp(`^${label}\\s*${matches ? 1 : 0}$`),
      })
    ).toBeVisible()
  }
  expect(filterByQuotaType([model], 'all')).toEqual([model])
  expect(filterByQuotaType([model], 'unknown')).toEqual([])
})
