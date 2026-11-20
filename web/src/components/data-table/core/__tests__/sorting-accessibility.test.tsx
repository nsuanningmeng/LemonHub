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
import { getCoreRowModel, useReactTable } from '@tanstack/react-table'
import { fireEvent, render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import { DataTableHeader } from '../data-table-header'

function SortableTable() {
  const table = useReactTable({
    data: [{ name: 'Lemon', status: 'Active' }],
    columns: [
      { accessorKey: 'name', header: 'Name' },
      { accessorKey: 'status', header: 'Status', enableSorting: false },
    ],
    getCoreRowModel: getCoreRowModel(),
  })

  return (
    <>
      <button
        type='button'
        onClick={() => table.setSorting([{ id: 'name', desc: false }])}
      >
        Sort ascending
      </button>
      <button
        type='button'
        onClick={() => table.setSorting([{ id: 'name', desc: true }])}
      >
        Sort descending
      </button>
      <button type='button' onClick={() => table.resetSorting()}>
        Clear sorting
      </button>
      <table>
        <DataTableHeader table={table} />
      </table>
    </>
  )
}

test('changing and clearing table sorting exposes only the current direction on the column header', () => {
  render(<SortableTable />)
  const nameHeader = screen.getByRole('columnheader', { name: 'Name' })
  const statusHeader = screen.getByRole('columnheader', { name: 'Status' })
  expect(nameHeader).not.toHaveAttribute('aria-sort')
  fireEvent.click(screen.getByRole('button', { name: 'Sort ascending' }))
  expect(nameHeader).toHaveAttribute('aria-sort', 'ascending')
  expect(statusHeader).not.toHaveAttribute('aria-sort')
  fireEvent.click(screen.getByRole('button', { name: 'Sort descending' }))
  expect(nameHeader).toHaveAttribute('aria-sort', 'descending')
  fireEvent.click(screen.getByRole('button', { name: 'Clear sorting' }))
  expect(nameHeader).not.toHaveAttribute('aria-sort')
})
