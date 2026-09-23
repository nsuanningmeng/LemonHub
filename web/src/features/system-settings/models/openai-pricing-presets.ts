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
import type { RequestRuleGroup } from '@/features/pricing/lib/billing-expr'

type OpenAIPricingPreset = {
  key: string
  label: string
  expr: string
  requestRules?: RequestRuleGroup[]
}

// USD / 1M tokens, checked 2026-09-23:
// https://developers.openai.com/api/docs/pricing
// https://developers.openai.com/api/docs/guides/latest-model?model=gpt-5.4
// https://openai.com/api-fast-mode/ (GPT-5.4 / GPT-5.5 exclude long context)
// GPT-5.6 Sol promotional rates are guaranteed at least through 2026-11-21.
// These presets price the requested tier; upstream filtering/defaults/downgrades
// still require the gateway to reconcile the actual response service_tier.
function openAIServiceTierRules(
  fastMultiplier = '2',
  fastMaxInput?: number
): RequestRuleGroup[] {
  return [
    { value: 'priority', multiplier: fastMultiplier },
    { value: 'fast', multiplier: fastMultiplier },
    { value: 'flex', multiplier: '0.5' },
  ].map(({ value, multiplier }) => {
    const conditions: RequestRuleGroup['conditions'] = [
      { source: 'param', path: 'service_tier', mode: 'eq', value },
    ]
    if (value !== 'flex' && fastMaxInput !== undefined) {
      conditions.push({
        source: 'token',
        path: 'len',
        mode: 'lte',
        value: String(fastMaxInput),
      })
    }
    return { conditions, multiplier }
  })
}

export const OPENAI_PRICING_PRESETS: OpenAIPricingPreset[] = [
  {
    key: 'gpt-5.4-tiers',
    label: 'GPT-5.4 Fast/Flex',
    // Fast falls back to Standard above 272K.
    expr: 'len <= 272000 ? tier("standard", p * 2.5 + c * 15 + cr * 0.25) : tier("long_context", p * 5 + c * 22.5 + cr * 0.5)',
    requestRules: openAIServiceTierRules('2', 272000),
  },
  {
    key: 'gpt-5.5-tiers',
    label: 'GPT-5.5 Fast/Flex',
    // GPT-5.5 Fast is 2.5x and excludes long context, unlike GPT-5.6 / GPT-6.
    expr: 'len <= 272000 ? tier("standard", p * 5 + c * 30 + cr * 0.5) : tier("long_context", p * 10 + c * 45 + cr * 1)',
    requestRules: openAIServiceTierRules('2.5', 272000),
  },
  {
    key: 'gpt-5.6-sol-tiers',
    label: 'GPT-5.6 Sol Fast/Flex',
    expr: 'len <= 272000 ? tier("standard", p * 4 + c * 20 + cr * 0.4 + cc * 5) : tier("long_context", p * 8 + c * 30 + cr * 0.8 + cc * 10)',
    requestRules: openAIServiceTierRules(),
  },
  {
    key: 'gpt-6-astra-tiers',
    label: 'GPT-6 Astra Fast/Flex',
    expr: 'len <= 272000 ? tier("standard", p * 10 + c * 50 + cr * 1 + cc * 12.5) : tier("long_context", p * 20 + c * 75 + cr * 2 + cc * 25)',
    requestRules: openAIServiceTierRules(),
  },
  {
    key: 'gpt-6-sol-tiers',
    label: 'GPT-6 Sol Fast/Flex',
    expr: 'len <= 272000 ? tier("standard", p * 2 + c * 10 + cr * 0.2 + cc * 2.5) : tier("long_context", p * 4 + c * 15 + cr * 0.4 + cc * 5)',
    requestRules: openAIServiceTierRules(),
  },
  {
    key: 'gpt-6-luna-tiers',
    label: 'GPT-6 Luna Fast/Flex',
    expr: 'len <= 272000 ? tier("standard", p * 0.1 + c * 0.5 + cr * 0.01 + cc * 0.125) : tier("long_context", p * 0.2 + c * 0.75 + cr * 0.02 + cc * 0.25)',
    requestRules: openAIServiceTierRules(),
  },
]
