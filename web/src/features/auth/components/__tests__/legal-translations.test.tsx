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
import i18next from 'i18next'
import { afterEach, expect, test, vi } from 'vitest'

import zh from '@/i18n/locales/zh.json'

import type { SystemStatus } from '../../types'
import { LegalConsent } from '../legal-consent'
import { TermsFooter } from '../terms-footer'

afterEach(async () => {
  await i18next.changeLanguage('en')
})

const status = {
  user_agreement_enabled: true,
  privacy_policy_enabled: true,
} as SystemStatus

test.each(['sign-in', 'sign-up'] as const)(
  'translates the %s footer and links when the language changes',
  async (variant) => {
    render(<TermsFooter status={status} variant={variant} />)
    expect(
      screen.getByRole('link', { name: 'User Agreement' })
    ).toHaveAttribute('href', '/user-agreement')
    i18next.addResourceBundle('zh', 'translation', zh.translation, true, true)
    await act(() => i18next.changeLanguage('zh'))
    expect(
      screen.getByText(
        variant === 'sign-in'
          ? '点击登录即表示您同意我们的'
          : '创建账户即表示您同意我们的',
        { exact: false }
      )
    ).toBeVisible()
    expect(screen.getByRole('link', { name: '用户协议' })).toHaveAttribute(
      'href',
      '/user-agreement'
    )
    expect(screen.getByRole('link', { name: '隐私政策' })).toHaveAttribute(
      'href',
      '/privacy-policy'
    )
  }
)

test('translates the legal consent conjunction and omits it with only one document', async () => {
  i18next.addResourceBundle('zh', 'translation', zh.translation, true, true)
  await act(() => i18next.changeLanguage('zh'))
  const view = render(
    <LegalConsent status={status} checked={false} onCheckedChange={vi.fn()} />
  )
  expect(screen.getByRole('checkbox')).toHaveAccessibleName(
    /用户协议 和 隐私政策/
  )
  view.rerender(
    <LegalConsent
      status={{ ...status, privacy_policy_enabled: false }}
      checked={false}
      onCheckedChange={vi.fn()}
    />
  )
  expect(
    screen.queryByRole('link', { name: '隐私政策' })
  ).not.toBeInTheDocument()
  expect(screen.getByRole('checkbox')).not.toHaveAccessibleName(/ 和 /)
})
