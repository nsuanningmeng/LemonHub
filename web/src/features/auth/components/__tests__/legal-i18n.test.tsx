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
import { act, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import zhTW from '@/i18n/locales/zh-TW.json'
import zh from '@/i18n/locales/zh.json'

import { LegalConsent } from '../legal-consent'
import { TermsFooter } from '../terms-footer'

const cases = [
  {
    locale: 'zh',
    variant: 'sign-in',
    prefix: '点击登录，即表示你同意我们的',
    agreement: '用户协议',
    privacy: '隐私政策',
  },
  {
    locale: 'zh',
    variant: 'sign-up',
    prefix: '创建账户，即表示你同意我们的',
    agreement: '用户协议',
    privacy: '隐私政策',
  },
  {
    locale: 'zh-TW',
    variant: 'sign-in',
    prefix: '點擊登入，即表示你同意我們的',
    agreement: '用戶協議',
    privacy: '私隱政策',
  },
  {
    locale: 'zh-TW',
    variant: 'sign-up',
    prefix: '建立帳戶，即表示你同意我們的',
    agreement: '用戶協議',
    privacy: '私隱政策',
  },
] as const

test.each(cases)(
  '$locale $variant legal text is fully localized and retains working consent and policy links',
  async (item) => {
    const language = createInstance()
    await language.use(initReactI18next).init({
      lng: item.locale,
      fallbackLng: 'en',
      resources: { en, zh, 'zh-TW': zhTW },
    })
    const onCheckedChange = vi.fn()
    const status = {
      user_agreement_enabled: true,
      privacy_policy_enabled: true,
    }
    render(
      <I18nextProvider i18n={language}>
        <section aria-label='Consent'>
          <LegalConsent
            status={status}
            checked={false}
            onCheckedChange={onCheckedChange}
          />
        </section>
        <section aria-label='Terms'>
          <TermsFooter variant={item.variant} status={status} />
        </section>
      </I18nextProvider>
    )
    const consent = screen.getByRole('region', { name: 'Consent' })
    const footer = screen.getByRole('region', { name: 'Terms' })
    expect(consent).not.toHaveTextContent('and the')
    expect(footer).toHaveTextContent(item.prefix)
    for (const surface of [consent, footer]) {
      expect(
        within(surface).getByRole('link', { name: item.agreement })
      ).toHaveAttribute('href', '/user-agreement')
      expect(
        within(surface).getByRole('link', { name: item.privacy })
      ).toHaveAttribute('href', '/privacy-policy')
      expect(surface).not.toHaveTextContent(/User Agreement|Privacy Policy/)
    }
    const user = userEvent.setup()
    await user.click(within(consent).getByRole('checkbox'))
    expect(onCheckedChange).toHaveBeenCalledWith(true)
    await act(async () => {
      await language.changeLanguage('en')
    })
    expect(
      within(footer).getByRole('link', { name: 'User Agreement' })
    ).toHaveAttribute('href', '/user-agreement')
    expect(footer).toHaveTextContent(
      item.variant === 'sign-in'
        ? 'By clicking sign in, you agree to our'
        : 'By creating an account, you agree to our'
    )
  }
)
