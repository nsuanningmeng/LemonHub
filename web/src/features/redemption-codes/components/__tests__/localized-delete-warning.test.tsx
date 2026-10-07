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
import { I18nextProvider } from 'react-i18next'
import { expect, test, vi } from 'vitest'

import { createLocaleI18n } from '@/i18n/__tests__/helpers'
import { api } from '@/lib/api'

import { RedemptionsPrimaryButtons } from '../redemptions-primary-buttons'
import { RedemptionsProvider } from '../redemptions-provider'

test('invalid redemption deletion shows a complete translated warning and waits for explicit confirmation', async () => {
  const i18n = await createLocaleI18n('zhCN')
  const remove = vi.spyOn(api, 'delete')
  const user = userEvent.setup()
  render(
    <I18nextProvider i18n={i18n}>
      <RedemptionsProvider>
        <RedemptionsPrimaryButtons />
      </RedemptionsProvider>
    </I18nextProvider>
  )
  await user.click(
    screen.getByRole('button', { name: i18n.t('Delete Invalid') })
  )
  const dialog = screen.getByRole('alertdialog')
  expect(dialog).toHaveTextContent(
    '这将删除所有已使用、已禁用和已过期的兑换码。'
  )
  for (const label of ['已使用', '已禁用', '已过期']) {
    expect(screen.getByText(label, { selector: 'strong' })).toBeInTheDocument()
  }
  expect(remove).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: i18n.t('Cancel') }))
  expect(remove).not.toHaveBeenCalled()
})
