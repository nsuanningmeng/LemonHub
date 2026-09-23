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
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, test } from 'vitest'

import {
  buildRequestRuleExpr,
  combineBillingExpr,
} from '@/features/pricing/lib/billing-expr'
import { evalExprLocally } from '@/features/pricing/lib/tier-expr'

import { OPENAI_PRICING_PRESETS } from '../openai-pricing-presets'
import { TieredPricingEditor } from '../tiered-pricing-editor'

const BASE_EXPR = 'tier("base", p * 2 + c * 4)'
const LEGACY_GPT_54_EXPR =
  'len <= 272000 ? tier("standard", p * 2.5 + c * 15 + cr * 0.25) : tier("long_context", p * 5 + c * 22.5 + cr * 0.5)'
const LEGACY_GPT_54_RULES =
  '(param("service_tier") == "priority" ? 2 : 1) * (param("service_tier") == "flex" ? 0.5 : 1)'

function BillingEditorFixture(props: {
  initialExpr?: string
  initialRules?: string
}) {
  const [billingExpr, setBillingExpr] = useState(props.initialExpr ?? BASE_EXPR)
  const [requestRuleExpr, setRequestRuleExpr] = useState(
    props.initialRules ?? ''
  )
  const [savedExpr, setSavedExpr] = useState('')

  return (
    <>
      <TieredPricingEditor
        billingExpr={billingExpr}
        requestRuleExpr={requestRuleExpr}
        onBillingExprChange={setBillingExpr}
        onRequestRuleExprChange={setRequestRuleExpr}
      />
      <button
        type='button'
        onClick={() =>
          setSavedExpr(combineBillingExpr(billingExpr, requestRuleExpr))
        }
      >
        Save pricing
      </button>
      <output aria-label='Saved billing expression'>{savedExpr}</output>
    </>
  )
}

describe('OpenAI pricing preset selection', () => {
  test('selecting each expanded GPT template saves its complete expression and replaces previous multipliers', async () => {
    const user = userEvent.setup()
    render(
      <BillingEditorFixture initialRules='(header("x-old-pricing") == "yes" ? 7 : 1)' />
    )

    await user.click(screen.getByRole('button', { name: 'More templates...' }))

    for (const label of [
      'GPT-5.4 Fast/Flex',
      'GPT-5.5 Fast/Flex',
      'GPT-5.6 Sol Fast/Flex',
      'GPT-6 Astra Fast/Flex',
      'GPT-6 Sol Fast/Flex',
      'GPT-6 Luna Fast/Flex',
    ]) {
      const preset = OPENAI_PRICING_PRESETS.find((item) => item.label === label)
      if (!preset) throw new Error(`Missing OpenAI pricing preset: ${label}`)

      await user.click(screen.getByRole('button', { name: label }))
      await user.click(screen.getByRole('button', { name: 'Save pricing' }))

      expect(
        screen.getByRole('status', { name: 'Saved billing expression' })
          .textContent
      ).toBe(
        combineBillingExpr(
          preset.expr,
          buildRequestRuleExpr(preset.requestRules ?? [])
        )
      )
    }
  })

  test('switching GPT-5.6 Sol between visual and expression editors preserves priority, fast and flex rules when saved', async () => {
    const user = userEvent.setup()
    render(<BillingEditorFixture />)

    await user.click(screen.getByRole('button', { name: 'More templates...' }))
    await user.click(
      screen.getByRole('button', { name: 'GPT-5.6 Sol Fast/Flex' })
    )
    await user.click(screen.getByRole('button', { name: 'Save pricing' }))
    const savedExpr = screen.getByRole('status', {
      name: 'Saved billing expression',
    }).textContent
    expect(savedExpr).toContain('param("service_tier") == "priority"')
    expect(savedExpr).toContain('param("service_tier") == "fast"')
    expect(savedExpr).toContain('param("service_tier") == "flex"')

    await user.click(screen.getAllByRole('combobox')[0])
    await user.click(screen.getByRole('option', { name: 'Expression editor' }))
    expect(screen.getByRole('textbox')).toHaveValue(savedExpr)

    await user.click(screen.getByRole('combobox'))
    await user.click(screen.getByRole('option', { name: 'Visual editor' }))
    expect(screen.getAllByRole('combobox')[0]).toHaveTextContent(
      'Visual editor'
    )
    await user.click(screen.getByRole('button', { name: 'Save pricing' }))
    expect(
      screen.getByRole('status', { name: 'Saved billing expression' })
        .textContent
    ).toBe(savedExpr)
  })

  test.each(['gpt-5.4-tiers', 'gpt-5.5-tiers'])(
    '%s preserves the Fast context limit through visual and expression editing',
    async (key) => {
      const user = userEvent.setup()
      render(<BillingEditorFixture initialRules={LEGACY_GPT_54_RULES} />)

      await user.click(
        screen.getByRole('button', { name: 'More templates...' })
      )
      const preset = OPENAI_PRICING_PRESETS.find((item) => item.key === key)
      if (!preset) throw new Error(`Missing ${key} pricing preset`)
      await user.click(screen.getByRole('button', { name: preset.label }))
      expect(screen.getAllByText('Full input length').length).toBeGreaterThan(0)
      expect(screen.queryByText(/Expression error/)).not.toBeInTheDocument()
      const expression = combineBillingExpr(
        preset.expr,
        buildRequestRuleExpr(preset.requestRules ?? [])
      )
      await user.click(screen.getAllByRole('combobox')[0])
      await user.click(
        screen.getByRole('option', { name: 'Expression editor' })
      )
      expect(screen.getByRole('textbox')).toHaveValue(expression)

      await user.click(screen.getByRole('combobox'))
      await user.click(screen.getByRole('option', { name: 'Visual editor' }))
      expect(screen.getAllByRole('combobox')[0]).toHaveTextContent(
        'Visual editor'
      )
      await user.click(screen.getByRole('button', { name: 'Save pricing' }))
      expect(
        screen.getByRole('status', { name: 'Saved billing expression' })
          .textContent
      ).toBe(expression)
    }
  )

  test('opening the new template list without selecting a template leaves a saved legacy GPT-5.4 configuration unchanged', async () => {
    const user = userEvent.setup()
    render(
      <BillingEditorFixture
        initialExpr={LEGACY_GPT_54_EXPR}
        initialRules={LEGACY_GPT_54_RULES}
      />
    )

    await user.click(screen.getByRole('button', { name: 'More templates...' }))
    await user.click(screen.getByRole('button', { name: 'Save pricing' }))

    expect(
      screen.getByRole('status', { name: 'Saved billing expression' })
        .textContent
    ).toBe(combineBillingExpr(LEGACY_GPT_54_EXPR, LEGACY_GPT_54_RULES))
  })
})

describe('published OpenAI token prices', () => {
  test.each([
    ['gpt-5.4-tiers', 0, 4050, 1502350],
    ['gpt-5.5-tiers', 0, 8100, 3004700],
    ['gpt-5.6-sol-tiers', 100, 6580, 2404160],
    ['gpt-6-astra-tiers', 100, 16450, 6010400],
    ['gpt-6-sol-tiers', 100, 3290, 1202080],
    ['gpt-6-luna-tiers', 100, 164.5, 60104],
  ] as const)(
    '%s estimates ordinary input, cache reads, cache writes and output at the model rates',
    (key, cacheCreateTokens, shortCost, longCost) => {
      const preset = OPENAI_PRICING_PRESETS.find((item) => item.key === key)
      if (!preset) throw new Error(`Missing ${key} pricing preset`)
      const extras = {
        cacheReadTokens: 200,
        cacheCreateTokens,
        cacheCreate1hTokens: 0,
        imageTokens: 0,
        imageOutputTokens: 0,
        audioInputTokens: 0,
        audioOutputTokens: 0,
      }
      expect(evalExprLocally(preset.expr, 1000, 100, extras)).toEqual({
        cost: shortCost,
        matchedTier: 'standard',
        error: null,
      })
      expect(evalExprLocally(preset.expr, 300000, 100, extras)).toEqual({
        cost: longCost,
        matchedTier: 'long_context',
        error: null,
      })
    }
  )
})
