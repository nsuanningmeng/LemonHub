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
import { I18nextProvider } from 'react-i18next'
import { expect, test } from 'vitest'

import { createLocaleI18n } from '@/i18n/__tests__/helpers'

import { usageLogSchema } from '../../../data/schema'
import { DetailsDialog } from '../details-dialog'

test.each([
  ['password', '密码', 'Password'],
  ['oauth:github', 'OAuth · GitHub', 'OAuth · GitHub'],
  ['oauth: AcmeTenant ', 'oauth: AcmeTenant ', 'oauth: AcmeTenant '],
])(
  'localizes login details for %s without altering the recorded method',
  async (method, chinese, english) => {
    const language = await createLocaleI18n('zhCN')
    const raw = `Logged in successfully via ${method}`
    const log = usageLogSchema.parse({
      id: 1,
      user_id: 7,
      created_at: 1,
      type: 7,
      content: raw,
      other: JSON.stringify({
        login_method: method,
        op: { action: 'login', params: { method } },
      }),
    })
    const original = log.other
    render(
      <I18nextProvider i18n={language}>
        <DetailsDialog log={log} isAdmin open onOpenChange={() => undefined} />
      </I18nextProvider>
    )
    expect(
      screen.getByText('登录方式').parentElement?.lastElementChild?.textContent
    ).toBe(chinese)
    expect(screen.getByText(raw.trim())).toBeVisible()
    await act(async () => {
      await language.changeLanguage('en')
    })
    expect(
      screen.getByText('Login Method').parentElement?.lastElementChild
        ?.textContent
    ).toBe(english)
    expect(log.other).toBe(original)
    expect(log.content).toBe(raw)
  }
)
