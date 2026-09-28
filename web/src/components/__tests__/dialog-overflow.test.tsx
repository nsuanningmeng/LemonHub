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
import { expect, test } from 'vitest'

import { Dialog } from '../dialog'

test('keeps ordinary dialog bodies scrollable within the popup by default', () => {
  render(
    <Dialog open title='Account settings'>
      <button type='button'>Save settings</button>
    </Dialog>
  )

  const popup = screen.getByRole('dialog', { name: 'Account settings' })
  const scrollContainer = screen
    .getByRole('button', { name: 'Save settings' })
    .closest<HTMLElement>('.overflow-y-auto')

  expect(popup).toHaveClass('overflow-hidden')
  expect(scrollContainer).not.toBeNull()
  expect(popup).toContainElement(scrollContainer)
  expect(scrollContainer).toHaveClass('overflow-x-hidden')
})
