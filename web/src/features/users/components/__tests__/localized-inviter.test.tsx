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
import { expect, test } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { createLocaleI18n } from '@/i18n/__tests__/helpers'
import { TableColumnFixture } from '@/i18n/__tests__/table-fixture'

import { userSchema } from '../../types'
import { useUsersColumns } from '../users-columns'

function InviterFixture() {
  const columns = useUsersColumns()
  const user = userSchema.parse({
    id: 1,
    username: 'Local',
    display_name: 'Local',
    quota: 0,
    used_quota: 0,
    request_count: 0,
    group: 'default',
    status: 1,
    role: 1,
    inviter_id: 42,
  })
  return (
    <TableColumnFixture
      columns={columns}
      data={[user]}
      columnId='invite_info'
    />
  )
}

test('inviter badge and tooltip use complete localized sentences with the same ID', async () => {
  const i18n = await createLocaleI18n('zhCN')
  const user = userEvent.setup()
  render(
    <I18nextProvider i18n={i18n}>
      <TooltipProvider>
        <InviterFixture />
      </TooltipProvider>
    </I18nextProvider>
  )
  const badge = screen.getByText('邀请人：42', { exact: true })
  await user.hover(badge)
  expect(
    await screen.findByText('由用户 ID 42 邀请', { exact: true })
  ).toBeVisible()
})
