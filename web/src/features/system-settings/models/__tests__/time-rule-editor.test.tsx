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

import { combineBillingExpr } from '@/features/pricing/lib/billing-expr'

import { TieredPricingEditor } from '../tiered-pricing-editor'

const BASE_EXPR = 'tier("base", p * 2 + c * 4)'

function BillingEditorFixture(props: { rules: string }) {
  const [billingExpr, setBillingExpr] = useState(BASE_EXPR)
  const [requestRuleExpr, setRequestRuleExpr] = useState(props.rules)
  return (
    <>
      <TieredPricingEditor
        billingExpr={billingExpr}
        requestRuleExpr={requestRuleExpr}
        onBillingExprChange={setBillingExpr}
        onRequestRuleExprChange={setRequestRuleExpr}
      />
      <output aria-label='Stored billing expression'>
        {combineBillingExpr(billingExpr, requestRuleExpr)}
      </output>
    </>
  )
}

describe('time rule editor', () => {
  test.each([
    '(hour("UTC") >= 9 || hour("UTC") < 12 ? 2 : 1)',
    '(hour("UTC") >= 9 && hour("UTC") < 24 ? 2 : 1)',
    '(hour("UTC") >= 21 || hour("UTC") < 6 && param("service_tier") == "fast" ? 2 : 1)',
  ])(
    'preserves unsupported stored time conditions in raw mode: %s',
    (rules) => {
      render(<BillingEditorFixture rules={rules} />)

      const original = combineBillingExpr(BASE_EXPR, rules)
      expect(
        screen.getByRole('status', { name: 'Stored billing expression' })
      ).toHaveTextContent(original)
      expect(screen.getByRole('textbox')).toHaveValue(original)
      expect(screen.getByRole('combobox')).toHaveTextContent(
        'Expression editor'
      )
    }
  )

  test('refuses a visual-mode switch that would discard an explicit OR condition', async () => {
    const user = userEvent.setup()
    const rules = '(hour("UTC") >= 9 || hour("UTC") < 12 ? 2 : 1)'
    render(<BillingEditorFixture rules={rules} />)

    await user.click(screen.getByRole('combobox'))
    await user.click(screen.getByRole('option', { name: 'Visual editor' }))

    const original = combineBillingExpr(BASE_EXPR, rules)
    expect(screen.getByRole('textbox')).toHaveValue(original)
    expect(
      screen.getByRole('status', { name: 'Stored billing expression' })
    ).toHaveTextContent(original)
  })

  test('preserves a daytime range through visual and raw editor switches', async () => {
    const user = userEvent.setup()
    const rules =
      '(hour("Asia/Shanghai") >= 9 && hour("Asia/Shanghai") < 12 ? 2 : 1)'
    render(<BillingEditorFixture rules={rules} />)

    expect(
      screen.getByText(
        'Start ≤ end: within the day; start > end: across midnight'
      )
    ).toBeInTheDocument()
    await user.click(screen.getAllByRole('combobox')[0])
    await user.click(screen.getByRole('option', { name: 'Expression editor' }))

    const original = combineBillingExpr(BASE_EXPR, rules)
    expect(screen.getByRole('textbox')).toHaveValue(original)
    await user.click(screen.getByRole('combobox'))
    await user.click(screen.getByRole('option', { name: 'Visual editor' }))
    expect(
      screen.getByRole('status', { name: 'Stored billing expression' })
    ).toHaveTextContent(original)
    expect(screen.getAllByRole('combobox')[0]).toHaveTextContent(
      'Visual editor'
    )
  })
})
