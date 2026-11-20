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

import { RiskAcknowledgementDialog } from '../risk-acknowledgement-dialog'

test('inserting a static confirmation segment keeps the active input and its focus', () => {
  const props = {
    open: true,
    onOpenChange: () => {},
    onConfirm: () => {},
    title: 'Confirm',
  }
  const input = { type: 'input' as const, text: 'I accept' }
  const text = { type: 'static' as const, text: ' • ' }
  const { rerender } = render(
    <RiskAcknowledgementDialog {...props} requiredTextParts={[text, input]} />
  )
  const field = screen.getByRole('textbox')
  field.focus()
  rerender(
    <RiskAcknowledgementDialog
      {...props}
      requiredTextParts={[text, text, input]}
    />
  )
  expect(screen.getByRole('textbox')).toBe(field)
  expect(field).toHaveFocus()
})
