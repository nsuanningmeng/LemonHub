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
import { expect, test } from 'vitest'

import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogTitle,
} from '../alert-dialog'
import { Dialog, DialogContent, DialogTitle } from '../dialog'

test('a tall dialog stays within the visible viewport and can scroll to its controls', () => {
  render(
    <Dialog open>
      <DialogContent>
        <DialogTitle>Attachment cleanup</DialogTitle>
        <div style={{ height: 800 }}>Cleanup options</div>
        <button type='button'>Run cleanup</button>
      </DialogContent>
    </Dialog>
  )

  const dialog = screen.getByRole('dialog', { name: 'Attachment cleanup' })
  expect(dialog).toHaveClass(
    'max-h-[calc(100dvh-2rem)]',
    'overflow-y-auto',
    'overscroll-contain'
  )
  expect(dialog).toContainElement(
    screen.getByRole('button', { name: 'Run cleanup' })
  )
})

test('a long confirmation leaves phone edge space and lets users scroll and cancel', async () => {
  render(
    <AlertDialog defaultOpen>
      <AlertDialogContent>
        <AlertDialogTitle>Confirm deletion</AlertDialogTitle>
        <AlertDialogDescription>
          {'A long resource name and deletion warning. '.repeat(20)}
        </AlertDialogDescription>
        <AlertDialogCancel>Cancel</AlertDialogCancel>
      </AlertDialogContent>
    </AlertDialog>
  )

  const dialog = screen.getByRole('alertdialog', { name: 'Confirm deletion' })
  expect(dialog).toHaveClass(
    'w-[calc(100%-2rem)]',
    'max-h-[calc(100dvh-2rem)]',
    'overflow-y-auto',
    'overscroll-contain'
  )
  await userEvent.setup().click(screen.getByRole('button', { name: 'Cancel' }))
  expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
})
