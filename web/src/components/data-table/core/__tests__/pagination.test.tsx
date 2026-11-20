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
import {
  getCoreRowModel,
  getPaginationRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'

import { DataTablePagination } from '../pagination'

const records = Array.from({ length: 51 }, (_, id) => ({ id }))
const columns = [{ accessorKey: 'id' }]

function PaginatedRecords() {
  const table = useReactTable({
    data: records,
    columns,
    getCoreRowModel: getCoreRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
  })
  return <DataTablePagination table={table} />
}

test('navigating to the second page moves the current-page marker and enables the previous page', async () => {
  const user = userEvent.setup()
  render(<PaginatedRecords />)
  expect(screen.getByRole('button', { current: 'page' })).toHaveTextContent('1')
  const previous = screen.getByRole('button', { name: 'Go to previous page' })
  expect(previous).toBeDisabled()
  const next = screen.getByRole('button', { name: 'Go to next page' })
  next.focus()
  await user.keyboard('{Enter}')
  expect(screen.getByRole('button', { current: 'page' })).toHaveTextContent('2')
  expect(previous).toBeEnabled()
})

test('page size is identifiable independently of the visible option', () => {
  render(<PaginatedRecords />)
  expect(
    screen.getByRole('combobox', { name: 'Rows per page' })
  ).toHaveTextContent('10')
})
