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
import { expect, it, vi } from 'vitest'

import { GroupSpecialUsableRulesEditor } from '../group-special-usable-editor'

const group = 'enterprise-production-with-a-long-group-name'
const target = 'premium-production-with-a-long-group-name'

it('stacks expanded rule controls on phones and contains long group names', async () => {
  const user = userEvent.setup()
  render(
    <GroupSpecialUsableRulesEditor
      value={JSON.stringify({ [group]: { [`+:${target}`]: 'Premium access' } })}
      groupOptions={[group, target]}
      onChange={vi.fn()}
    />
  )

  expect(screen.getByText(group)).toHaveClass('min-w-0', 'break-all')
  await user.click(screen.getByRole('button', { name: 'Expand' }))
  expect(screen.getByRole('button', { name: 'Collapse' })).toHaveAttribute(
    'aria-expanded',
    'true'
  )
  expect(
    screen.getByRole('textbox', { name: 'Description' }).parentElement
  ).toHaveClass('flex-col', 'sm:flex-row')
  expect(screen.getByRole('combobox', { name: 'Status' })).toHaveClass(
    'w-full',
    'sm:w-32'
  )
  expect(screen.getByRole('combobox', { name: 'Group name' })).toHaveClass(
    'min-w-0',
    'w-full'
  )
  expect(
    screen.getByRole('combobox', { name: 'Add rules for a user group' })
  ).toHaveClass('w-full', 'max-w-60')
})

it('keeps the expanded rule editable and removable through named controls', async () => {
  const user = userEvent.setup()
  const onChange = vi.fn()
  render(
    <GroupSpecialUsableRulesEditor
      value={JSON.stringify({ default: { '+:premium': 'Premium access' } })}
      groupOptions={['default', 'premium']}
      onChange={onChange}
    />
  )

  await user.click(screen.getByRole('button', { name: 'Expand' }))
  await user.type(
    screen.getByRole('textbox', { name: 'Description' }),
    ' updated'
  )
  expect(JSON.parse(onChange.mock.lastCall?.[0] ?? '{}')).toEqual({
    default: { '+:premium': 'Premium access updated' },
  })
  await user.click(screen.getByRole('button', { name: 'Delete' }))
  expect(onChange).toHaveBeenLastCalledWith('{}')
  expect(
    screen.getByText('No rules yet. Add a group below to get started.')
  ).toBeVisible()
})
