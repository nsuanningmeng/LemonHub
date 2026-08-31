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
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, test, vi } from 'vitest'

import { ApiKeyGroupPriorityList } from '../api-key-group-priority-list'

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'Maximum {{max}} groups selected': 'Maximum {{max}} groups selected',
        Remove: 'Remove',
      },
    },
  },
})

function renderList(value: string[]) {
  const options = Array.from({ length: 9 }, (_, index) => ({
    value: `group-${index}`,
    label: `Group ${index}`,
  }))
  const onChange = vi.fn()

  render(
    <I18nextProvider i18n={i18n}>
      <ApiKeyGroupPriorityList
        options={options}
        value={value}
        onChange={onChange}
      />
    </I18nextProvider>
  )

  return onChange
}

describe('API key group priority boundaries', () => {
  test('does not allow removing the only selected group', () => {
    const onChange = renderList(['group-0'])

    const removeButton = screen.getByRole('button', { name: 'Remove' })
    expect(removeButton).toBeDisabled()
    removeButton.click()
    expect(onChange).not.toHaveBeenCalled()
  })

  test('does not allow adding a ninth group', () => {
    renderList(Array.from({ length: 8 }, (_, index) => `group-${index}`))

    const trigger = screen.getByRole('combobox')
    expect(trigger).toBeDisabled()
    expect(trigger).toHaveTextContent('Maximum 8 groups selected')
  })
})
