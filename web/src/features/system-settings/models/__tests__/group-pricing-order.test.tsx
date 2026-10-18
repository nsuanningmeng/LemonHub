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
import { expect, test, vi } from 'vitest'

import { GroupRatioVisualEditor } from '../group-ratio-visual-editor'

test('adding consecutive groups prepends them without changing existing prices or descriptions', async () => {
  const user = userEvent.setup()
  const onChange = vi.fn()
  render(
    <GroupRatioVisualEditor
      groupRatio='{"default":1,"vip":0.8}'
      topupGroupRatio='{"vip":1.2}'
      userUsableGroups='{"default":"Standard","vip":"Premium"}'
      groupGroupRatio='{}'
      autoGroups='["default","vip"]'
      maxTokenAutoGroupsField={null}
      groupSpecialUsableGroup='{}'
      onChange={onChange}
    />
  )
  await user.click(screen.getByRole('button', { name: 'Add group' }))
  await user.click(screen.getByRole('button', { name: 'Add group' }))
  expect(
    screen
      .getAllByDisplayValue<HTMLInputElement>(/^(group_[12]|default|vip)$/)
      .map((input) => input.value)
  ).toEqual(['group_2', 'group_1', 'default', 'vip'])
  expect(onChange).toHaveBeenCalledWith(
    'GroupRatio',
    JSON.stringify({ group_2: 1, group_1: 1, default: 1, vip: 0.8 }, null, 2)
  )
  expect(onChange).toHaveBeenCalledWith(
    'TopupGroupRatio',
    JSON.stringify({ vip: 1.2 }, null, 2)
  )
  expect(onChange).toHaveBeenCalledWith(
    'UserUsableGroups',
    JSON.stringify(
      { group_2: '', group_1: '', default: 'Standard', vip: 'Premium' },
      null,
      2
    )
  )
  expect(onChange).not.toHaveBeenCalledWith('AutoGroups', expect.anything())
})
