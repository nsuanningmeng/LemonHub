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

import type { PricingModel } from '../../types'
import { sortModels } from '../filters'

function model(name: string, overrides: Partial<PricingModel>): PricingModel {
  return {
    id: 1,
    model_name: name,
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 4,
    enable_groups: ['default'],
    ...overrides,
  }
}
const models = [
  model('ratio', {}),
  model('request', { quota_type: 1, model_price: 0.05 }),
  model('expression', {
    billing_mode: 'tiered_expr',
    billing_expr: 'tier("base", p * 1 + c * 10)',
  }),
  model('long', {
    billing_mode: 'tiered_expr',
    billing_expr:
      'len <= 200000 ? tier("standard", p * 3 + c * 15) : tier("long", p * 6 + c * 22.5)',
  }),
  model('input-only', {
    billing_mode: 'tiered_expr',
    billing_expr: 'tier("base", p * 0.1)',
  }),
  model('unparseable', {
    billing_mode: 'tiered_expr',
    billing_expr: 'max(p * 3, 5)',
  }),
]

test.each([
  [
    'input-price-low',
    ['input-only', 'expression', 'ratio', 'long', 'request', 'unparseable'],
  ],
  [
    'input-price-high',
    ['long', 'ratio', 'expression', 'input-only', 'request', 'unparseable'],
  ],
  [
    'output-price-low',
    ['ratio', 'expression', 'long', 'request', 'input-only', 'unparseable'],
  ],
  [
    'output-price-high',
    ['long', 'expression', 'ratio', 'request', 'input-only', 'unparseable'],
  ],
  [
    'request-price-low',
    ['request', 'ratio', 'expression', 'long', 'input-only', 'unparseable'],
  ],
  [
    'request-price-high',
    ['request', 'ratio', 'expression', 'long', 'input-only', 'unparseable'],
  ],
])(
  '%s compares configured prices and leaves unsupported or missing prices last',
  (sortBy, expected) => {
    expect(sortModels(models, sortBy).map((entry) => entry.model_name)).toEqual(
      expected
    )
    expect(models[0].model_name).toBe('ratio')
  }
)

test('an explicit zero price sorts before a positive price without treating missing prices as zero', () => {
  const values = [
    model('paid', {}),
    model('free', { model_ratio: 0 }),
    model('missing', { quota_type: 1 }),
  ]
  expect(
    sortModels(values, 'input-price-low').map((entry) => entry.model_name)
  ).toEqual(['free', 'paid', 'missing'])
})

test.each(['price-low', 'price-high'])(
  'legacy bookmark sort %s retains its input-price direction',
  (sortBy) => {
    const values = [
      model('middle', { model_ratio: 2 }),
      model('cheap', { model_ratio: 1 }),
      model('expensive', { model_ratio: 3 }),
    ]
    expect(sortModels(values, sortBy).map((entry) => entry.model_name)).toEqual(
      sortBy === 'price-low'
        ? ['cheap', 'middle', 'expensive']
        : ['expensive', 'middle', 'cheap']
    )
  }
)

test.each(['input', 'output'])(
  'an explicitly free %s expression price remains distinct from an absent price',
  (kind) => {
    const variable = kind === 'input' ? 'p' : 'c'
    const other = kind === 'input' ? 'c' : 'p'
    const values = [
      model('paid', {}),
      model('missing', {
        billing_mode: 'tiered_expr',
        billing_expr: `tier("base", ${other} * 3)`,
      }),
      model('free', {
        billing_mode: 'tiered_expr',
        billing_expr: `tier("base", ${variable} * 0 + ${other} * 3)`,
      }),
    ]
    expect(
      sortModels(values, `${kind}-price-low`).map((entry) => entry.model_name)
    ).toEqual(['free', 'paid', 'missing'])
    expect(
      sortModels(values, `${kind}-price-high`).map((entry) => entry.model_name)
    ).toEqual(['paid', 'free', 'missing'])
  }
)
