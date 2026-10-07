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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createTable,
  getCoreRowModel,
  getFilteredRowModel,
} from '@tanstack/react-table'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { DataTableBulkActions } from '../data-table-bulk-actions'

type SelectionRow = {
  key: string
  id: number
  visible: string
  allow: boolean
  children?: SelectionRow[]
}
const clients: QueryClient[] = []
const initialAuth = useAuthStore.getState().auth

afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  useAuthStore.setState({ auth: initialAuth })
})

function realTable(allInvalid = false) {
  const node = (
    key: string,
    id: number,
    visible = 'yes',
    allow = true
  ): SelectionRow => ({ key, id, visible, allow })
  const children = [
    node('a', 5),
    node('duplicate', 5),
    node('b', 7),
    node('notselected', 9),
    node('negative', -1),
    node('fraction', 1.5),
    node('unsafe', Number.MAX_SAFE_INTEGER + 1),
    node('infinite', Infinity),
    node('cannotselect', 88, 'yes', false),
  ]
  const data: SelectionRow[] = [
    { ...node('synthetic', 9191), children },
    node('filtered', 66, 'no'),
  ]
  const rowSelection = Object.fromEntries(
    [
      'synthetic',
      'a',
      'duplicate',
      'b',
      'negative',
      'fraction',
      'unsafe',
      'infinite',
      'cannotselect',
      'filtered',
    ]
      .filter((key) => !allInvalid || !['a', 'duplicate', 'b'].includes(key))
      .map((key) => [key, true])
  )
  return createTable<SelectionRow>({
    data,
    columns: [{ accessorKey: 'visible', filterFn: 'equalsString' }],
    getRowId: (row) => row.key,
    getSubRows: (row) => row.children,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    filterFromLeafRows: true,
    state: { rowSelection, columnFilters: [{ id: 'visible', value: 'yes' }] },
    enableRowSelection: (row) => !row.original.children && row.original.allow,
    onStateChange: () => undefined,
    renderFallbackValue: null,
  })
}

test('filtered TanStack selection excludes numeric parents, unselected children and invalid IDs while deduplicating channels', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'admin', role: 100 })
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true, data: 2 } })
  const client = new QueryClient()
  clients.push(client)
  render(
    <QueryClientProvider client={client}>
      <DataTableBulkActions table={realTable()} />
    </QueryClientProvider>
  )
  expect(screen.getByRole('toolbar')).toHaveAttribute(
    'aria-label',
    'Bulk actions for 2 selected channels'
  )
  await userEvent.click(
    screen.getByRole('button', { name: 'Enable selected channels' })
  )
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
  expect(post.mock.calls[0].slice(0, 2)).toEqual([
    '/api/channel/status/batch',
    { ids: [5, 7], status: 1 },
  ])
})

test('stale selection containing only synthetic, filtered or invalid records cannot expose bulk mutations', () => {
  const client = new QueryClient()
  clients.push(client)
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true, data: 0 } })
  render(
    <QueryClientProvider client={client}>
      <DataTableBulkActions table={realTable(true)} />
    </QueryClientProvider>
  )
  expect(screen.queryByRole('toolbar')).not.toBeInTheDocument()
  expect(post).not.toHaveBeenCalled()
})
