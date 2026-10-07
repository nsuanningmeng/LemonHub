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
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { useState } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { combineBillingExpr } from '@/features/pricing/lib/billing-expr'
import zhTranslations from '@/i18n/locales/zh.json'

import { TieredPricingEditor } from '../tiered-pricing-editor'

const WEEKDAY_PEAK_EXPR =
  '(tier("base", p * 1.5 + c * 4.5 + cr * 0.15)) * ((weekday("Asia/Shanghai") >= 1 && weekday("Asia/Shanghai") < 6 && ((hour("Asia/Shanghai") >= 9 && hour("Asia/Shanghai") < 12) || (hour("Asia/Shanghai") >= 14 && hour("Asia/Shanghai") < 18))) ? 2 : 1)'

function TimePricingEditorFixture(props: { initialExpr?: string }) {
  const [billingExpr, setBillingExpr] = useState(
    props.initialExpr ?? 'tier("base", p * 1.5 + c * 4.5)'
  )
  const [requestRuleExpr, setRequestRuleExpr] = useState('')
  const [saved, setSaved] = useState('')
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
          setSaved(combineBillingExpr(billingExpr, requestRuleExpr))
        }
      >
        Save pricing
      </button>
      <output aria-label='Saved pricing expression'>{saved}</output>
    </>
  )
}

afterEach(() => {
  vi.useRealTimers()
})

test('entering nested weekday peak pricing previews the price and saves the raw expression without losing OR conditions', async () => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date('2026-10-05T01:00:00Z'))
  const user = userEvent.setup()
  render(<TimePricingEditorFixture />)

  await user.click(screen.getAllByRole('combobox')[0])
  await user.click(screen.getByRole('option', { name: 'Expression editor' }))
  await user.clear(screen.getByRole('textbox'))
  await user.click(screen.getByRole('textbox'))
  await user.paste(WEEKDAY_PEAK_EXPR)
  const [input, output, cacheRead] = screen.getAllByRole('spinbutton')
  await user.clear(input)
  await user.type(input, '100')
  await user.clear(output)
  await user.type(output, '100')
  await user.clear(cacheRead)
  await user.type(cacheRead, '100')

  expect(screen.queryByText(/Expression error/)).not.toBeInTheDocument()
  expect(
    screen.getByText(`Estimated quota cost: ${(1230).toLocaleString()}`)
  ).toBeInTheDocument()
  expect(screen.getByText('Hit tier: base')).toBeInTheDocument()

  await user.click(screen.getByRole('combobox'))
  await user.click(screen.getByRole('option', { name: 'Visual editor' }))
  expect(screen.getByRole('textbox')).toHaveValue(WEEKDAY_PEAK_EXPR)
  await user.click(screen.getByRole('button', { name: 'Save pricing' }))
  expect(
    screen.getByRole('status', { name: 'Saved pricing expression' }).textContent
  ).toBe(WEEKDAY_PEAK_EXPR)
})

test('an existing timezone error follows language changes without changing the expression or token inputs', async () => {
  const errorKey =
    'Server-local time cannot be estimated in the browser. Use an IANA time zone.'
  const translatedError = zhTranslations.translation[errorKey]
  const previous = i18next.getResourceBundle('zh', 'translation')
  i18next.addResourceBundle('zh', 'translation', {
    [errorKey]: translatedError,
  })
  try {
    render(
      <TimePricingEditorFixture initialExpr='tier("base", p) * hour("Local")' />
    )
    expect(
      screen.getByText((text) => text.includes(errorKey))
    ).toBeInTheDocument()

    await act(async () => {
      await i18next.changeLanguage('zh')
    })

    expect(
      screen.getByText((text) => text.includes(translatedError))
    ).toBeInTheDocument()
    expect(
      screen.queryByText((text) => text.includes(errorKey))
    ).not.toBeInTheDocument()
  } finally {
    await act(async () => {
      await i18next.changeLanguage('en')
    })
    i18next.removeResourceBundle('zh', 'translation')
    if (previous) i18next.addResourceBundle('zh', 'translation', previous)
  }
})
