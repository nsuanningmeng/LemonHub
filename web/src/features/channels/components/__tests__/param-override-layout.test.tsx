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
import { fireEvent, render, screen } from '@testing-library/react'
import { expect, test, vi } from 'vitest'

import { ParamOverrideEditorDialog } from '../dialogs/param-override-editor-dialog'

test('the rule list stacks above the full-width editor on phones while desktop retains two columns and edited values can be saved', () => {
  const onSave = vi.fn()
  render(
    <ParamOverrideEditorDialog
      open
      value={JSON.stringify({
        operations: [{ mode: 'set', path: 'temperature', value: 0.7 }],
      })}
      onSave={onSave}
      onOpenChange={vi.fn()}
    />
  )
  const rules = screen.getByText('Rules').closest('div.flex-col')
  expect(rules).toHaveClass('w-full', 'md:w-[280px]', 'h-56', 'md:h-auto')
  expect(rules?.parentElement).toHaveClass('flex-col', 'md:flex-row')
  const path = screen.getByDisplayValue('temperature')
  expect(path.closest('[data-slot="scroll-area"]')).toHaveClass('min-w-0')
  fireEvent.change(path, { target: { value: 'top_p' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  expect(onSave).toHaveBeenCalledOnce()
  expect(JSON.parse(onSave.mock.calls[0][0])).toEqual({
    operations: [{ mode: 'set', path: 'top_p', value: 0.7 }],
  })
})

test('JSON editing keeps its content scrollable when a short desktop dialog cannot fit the editor', () => {
  render(
    <ParamOverrideEditorDialog
      open
      value='{"operations":[{"mode":"set","path":"temperature","value":0.7}]}'
      onSave={vi.fn()}
      onOpenChange={vi.fn()}
    />
  )
  fireEvent.click(screen.getByRole('button', { name: 'JSON Text' }))
  const editor = screen.getByRole('textbox', { name: 'Advanced text editing' })
  expect(editor.closest('.min-h-0.flex-1')).toHaveClass('overflow-y-auto')
})
