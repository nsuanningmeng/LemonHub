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
import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, test, vi } from 'vitest'

import { createLocaleI18n, type TestLocale } from '@/i18n/__tests__/helpers'
import { api } from '@/lib/api'

import { DeleteAccountDialog } from '../delete-account-dialog'
import { TwoFASetupDialog } from '../two-fa-setup-dialog'

// Navigation is external to the confirmation dialog; no account is deleted.
vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))

const confirmCases: [TestLocale, string][] = [
  ['en', 'Type 用户甲 to confirm'],
  ['zhCN', '输入 用户甲 以确认'],
  ['zhTW', '輸入 用户甲 以確認'],
  ['ja', '確認のために 用户甲 と入力してください'],
  ['ru', 'Введите 用户甲 для подтверждения'],
  ['vi', 'Nhập 用户甲 để xác nhận'],
  ['fr', 'Saisissez 用户甲 pour confirmer'],
]

describe('localized account confirmation', () => {
  test.each(confirmCases)(
    '%s uses one instruction and preserves exact username confirmation',
    async (language, label) => {
      const i18n = await createLocaleI18n(language)
      render(
        <I18nextProvider i18n={i18n}>
          <DeleteAccountDialog open onOpenChange={vi.fn()} username='用户甲' />
        </I18nextProvider>
      )
      const input = screen.getByLabelText(label)
      const confirm = screen.getByRole('button', {
        name: i18n.t('Delete Account'),
      })
      expect(
        screen.getByText('用户甲', { selector: 'strong' })
      ).toBeInTheDocument()
      expect(confirm).toBeDisabled()
      fireEvent.change(input, { target: { value: '用户甲 ' } })
      expect(confirm).toBeDisabled()
      fireEvent.change(input, { target: { value: '用户甲' } })
      expect(confirm).toBeEnabled()
    }
  )

  test('a username containing markup and interpolation syntax stays literal bold text', async () => {
    const username = '<strong>用户</strong>{{name}}"&'
    const i18n = await createLocaleI18n('zhCN')
    render(
      <I18nextProvider i18n={i18n}>
        <DeleteAccountDialog open onOpenChange={vi.fn()} username={username} />
      </I18nextProvider>
    )
    const input = screen.getByLabelText(`输入 ${username} 以确认`)
    const strong = screen.getByText(username, { selector: 'strong' })
    expect(strong.children).toHaveLength(0)
    fireEvent.change(input, { target: { value: username } })
    expect(
      screen.getByRole('button', {
        name: i18n.t('Delete Account'),
      })
    ).toBeEnabled()
  })
})

describe('localized two-factor setup steps', () => {
  test.each([
    [
      'en',
      [
        'Step 1 of 3: Scan QR Code',
        'Step 2 of 3: Save Backup Codes',
        'Step 3 of 3: Verify Setup',
      ],
    ],
    [
      'zhCN',
      [
        '第 1 步，共 3 步：扫描二维码',
        '第 2 步，共 3 步：保存备份代码',
        '第 3 步，共 3 步：验证设置',
      ],
    ],
  ] as const)(
    '%s retains all three state transitions with a complete step sentence',
    async (language, descriptions) => {
      const i18n = await createLocaleI18n(language)
      const post = vi.spyOn(api, 'post').mockResolvedValue({
        data: {
          success: true,
          data: {
            qr_code_data: 'otpauth://totp/local-fixture',
            secret: 'LOCALTEST',
            backup_codes: ['local-backup'],
          },
        },
      })
      const user = userEvent.setup()
      render(
        <I18nextProvider i18n={i18n}>
          <TwoFASetupDialog open onOpenChange={vi.fn()} onSuccess={vi.fn()} />
        </I18nextProvider>
      )
      const dialog = screen.getByRole('dialog')
      expect(within(dialog).getByText(descriptions[0])).toBeInTheDocument()
      await within(dialog).findByText('LOCALTEST')
      const next = within(dialog).getByRole('button', { name: i18n.t('Next') })
      await user.click(next)
      expect(within(dialog).getByText(descriptions[1])).toBeInTheDocument()
      await user.click(
        within(dialog).getByRole('button', { name: i18n.t('Next') })
      )
      expect(within(dialog).getByText(descriptions[2])).toBeInTheDocument()
      expect(
        within(dialog).getByRole('button', { name: i18n.t('Enable 2FA') })
      ).toBeDisabled()
      expect(post).toHaveBeenCalledTimes(1)
      expect(post).toHaveBeenCalledWith('/api/user/2fa/setup')
    }
  )
})
