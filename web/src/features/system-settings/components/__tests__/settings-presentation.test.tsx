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
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { expect, test } from 'vitest'

import { Input } from '@/components/ui/input'

import { SettingsCard } from '../settings-card'
import {
  SettingsControlChildren,
  SettingsControlGroup,
  SettingsSwitchField,
} from '../settings-form-layout'

function SettingsFixture() {
  const [enabled, setEnabled] = useState(false)
  return (
    <SettingsCard title='Verification' description='Manage access checks'>
      <SettingsControlGroup>
        <SettingsSwitchField
          controlId='verification-enabled'
          checked={enabled}
          onCheckedChange={setEnabled}
          label='Enable verification'
        />
        {enabled && (
          <SettingsControlChildren>
            <label htmlFor='verification-key'>Public key</label>
            <Input id='verification-key' />
          </SettingsControlChildren>
        )}
      </SettingsControlGroup>
    </SettingsCard>
  )
}

test('an open settings section has a named heading and keeps dependent controls keyboard accessible', async () => {
  const user = userEvent.setup()
  render(<SettingsFixture />)
  const section = screen.getByRole('region', { name: 'Verification' })
  expect(within(section).getByRole('heading', { level: 2 })).toHaveTextContent(
    'Verification'
  )
  expect(within(section).getByText('Manage access checks')).toBeVisible()
  expect(within(section).queryByRole('textbox')).not.toBeInTheDocument()

  await user.tab()
  expect(
    within(section).getByRole('switch', { name: 'Enable verification' })
  ).toHaveFocus()
  await user.keyboard('[Space]')
  await user.tab()
  const key = within(section).getByRole('textbox', { name: 'Public key' })
  expect(key).toHaveFocus()
  await user.type(key, 'public-key')
  expect(key).toHaveValue('public-key')
})
