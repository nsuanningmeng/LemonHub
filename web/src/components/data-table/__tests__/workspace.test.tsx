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
  getFilteredRowModel,
  getPaginationRowModel,
  useReactTable,
  type ColumnDef,
} from '@tanstack/react-table'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, expect, it, vi } from 'vitest'

import { Checkbox } from '@/components/ui/checkbox'

import { DataTablePage } from '../layout/data-table-page'
import { DataTableBulkActions } from '../toolbar/bulk-actions'

type RecordRow = { name: string }
const records = [{ name: 'Production app' }, { name: 'Development app' }]
const columns: ColumnDef<RecordRow, unknown>[] = [
  {
    id: 'select',
    header: '',
    cell: ({ row }) => (
      <Checkbox
        checked={row.getIsSelected()}
        onCheckedChange={(checked) => row.toggleSelected(!!checked)}
        aria-label={`Select ${row.original.name}`}
      />
    ),
  },
  { accessorKey: 'name', header: 'Name', meta: { mobileTitle: true } },
]

function Workspace(props: {
  empty?: boolean
  loading?: boolean
  paginated?: boolean
  total?: number
  failed?: boolean
}) {
  const [environment, setEnvironment] = useState('')
  const table = useReactTable({
    data: props.empty ? [] : records,
    columns,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    manualPagination: props.total != null,
    rowCount: props.total,
    initialState: {
      pagination: { pageIndex: 0, pageSize: props.paginated ? 1 : 20 },
    },
  })
  return (
    <DataTablePage
      table={table}
      columns={columns}
      enableCardView
      viewModeStorageKey='test-workspace-view'
      paginationInFooter={false}
      isLoading={props.loading}
      error={props.failed ? 'Records could not be loaded' : undefined}
      emptyTitle='No connections'
      emptyAction={<button type='button'>Create connection</button>}
      bulkActions={
        <DataTableBulkActions table={table} entityName='connection'>
          <button type='button'>Archive selected</button>
        </DataTableBulkActions>
      }
      toolbarProps={{
        searchPlaceholder: 'Search connections',
        hideViewOptions: true,
        expandable: (
          <input
            aria-label='Environment'
            value={environment}
            onChange={(event) => setEnvironment(event.target.value)}
          />
        ),
      }}
    />
  )
}

afterEach(() => {
  localStorage.clear()
  vi.restoreAllMocks()
})

it('starts a new workspace in table view and preserves a saved card preference', async () => {
  const user = userEvent.setup()
  const result = render(<Workspace />)
  expect(screen.getByRole('table')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Table view' })).toHaveAttribute(
    'aria-pressed',
    'true'
  )
  await user.click(screen.getByRole('button', { name: 'Card view' }))
  expect(screen.queryByRole('table')).not.toBeInTheDocument()
  expect(screen.getByText('Production app')).toBeInTheDocument()
  result.unmount()
  render(<Workspace />)
  expect(screen.getByRole('button', { name: 'Card view' })).toHaveAttribute(
    'aria-pressed',
    'true'
  )
})

it('filters from the named search rail and restores rows when reset is pressed', async () => {
  const user = userEvent.setup()
  render(<Workspace />)
  const rail = screen.getByRole('search', { name: 'Filter records' })
  await user.type(
    within(rail).getByRole('textbox', { name: 'Search connections' }),
    'Production'
  )
  expect(screen.getByText('Production app')).toBeInTheDocument()
  expect(screen.queryByText('Development app')).not.toBeInTheDocument()
  await user.click(within(rail).getByRole('button', { name: 'Reset' }))
  expect(screen.getByText('Development app')).toBeInTheDocument()
  expect(
    screen.getByRole('textbox', { name: 'Search connections' })
  ).toHaveValue('')
})

it('keeps advanced filter input when collapsed and exposes its expanded state', async () => {
  const user = userEvent.setup()
  render(<Workspace />)
  const expand = screen.getByRole('button', { name: 'Expand' })
  expect(expand).toHaveAttribute('aria-expanded', 'false')
  expect(
    screen.queryByRole('textbox', { name: 'Environment' })
  ).not.toBeInTheDocument()
  await user.click(expand)
  await user.type(
    screen.getByRole('textbox', { name: 'Environment' }),
    'Production'
  )
  const collapse = screen.getByRole('button', { name: 'Collapse' })
  const panel = document.querySelector(
    `[id="${collapse.getAttribute('aria-controls')}"]`
  )
  expect(panel).toBeVisible()
  await user.click(collapse)
  expect(panel).not.toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Expand' }))
  expect(screen.getByRole('textbox', { name: 'Environment' })).toHaveValue(
    'Production'
  )
})

it('marks the current page and keeps pagination boundary buttons disabled', async () => {
  const user = userEvent.setup()
  render(<Workspace paginated />)
  const pagination = screen.getByRole('navigation', {
    name: 'Table pagination',
  })
  expect(
    within(pagination).getByRole('button', { name: 'Go to previous page' })
  ).toBeDisabled()
  expect(
    within(pagination).getByRole('button', { name: /Go to page 1/ })
  ).toHaveAttribute('aria-current', 'page')
  await user.click(
    within(pagination).getByRole('button', { name: 'Go to next page' })
  )
  expect(screen.getByText('Development app')).toBeInTheDocument()
  expect(
    within(pagination).getByRole('button', { name: /Go to page 2/ })
  ).toHaveAttribute('aria-current', 'page')
  expect(
    within(pagination).getByRole('button', { name: 'Go to next page' })
  ).toBeDisabled()
})

it('shows an empty-state action after loading completes without replacing the search rail', () => {
  const result = render(<Workspace empty loading />)
  expect(screen.queryByText('No connections')).not.toBeInTheDocument()
  expect(
    screen.getByRole('search', { name: 'Filter records' })
  ).toBeInTheDocument()
  result.rerender(<Workspace empty />)
  expect(screen.getByText('No connections')).toBeInTheDocument()
  expect(
    screen.getByRole('button', { name: 'Create connection' })
  ).toBeInTheDocument()
})

it('keeps the empty-state action available after switching to cards', async () => {
  const user = userEvent.setup()
  render(<Workspace empty />)
  await user.click(screen.getByRole('button', { name: 'Card view' }))
  expect(
    screen.getByRole('button', { name: 'Create connection' })
  ).toBeInTheDocument()
})

it('starts a narrow workspace in cards while retaining search and pagination', () => {
  const matchMedia = window.matchMedia
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
    ...matchMedia(query),
    matches: query === '(max-width: 640px)',
  }))
  render(<Workspace />)
  expect(screen.queryByRole('table')).not.toBeInTheDocument()
  expect(screen.getByText('Production app')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Card view' })).toHaveAttribute(
    'aria-pressed',
    'true'
  )
  expect(
    screen.getByRole('search', { name: 'Filter records' })
  ).toBeInTheDocument()
  expect(
    screen.getByRole('navigation', { name: 'Table pagination' })
  ).toBeInTheDocument()
})

it('keeps keyboard selection and its accessible row state in sync with the bulk-action rail', async () => {
  const user = userEvent.setup()
  render(<Workspace />)
  const checkbox = screen.getByRole('checkbox', {
    name: 'Select Production app',
  })
  checkbox.focus()
  await user.keyboard(' ')

  expect(checkbox).toBeChecked()
  expect(
    screen.getByRole('row', { name: /Production app/, selected: true })
  ).toBeInTheDocument()
  const selectionRail = screen.getByRole('toolbar', {
    name: 'Bulk actions for 1 selected connection',
  })
  const clear = within(selectionRail).getByRole('button', {
    name: 'Clear selection',
  })
  clear.focus()
  await user.keyboard('{Enter}')

  expect(checkbox).not.toBeChecked()
  expect(
    screen.getByRole('row', { name: /Production app/, selected: false })
  ).toBeInTheDocument()
  expect(screen.queryByRole('toolbar')).not.toBeInTheDocument()
})

it('lets pagination controls wrap instead of clipping them when large totals need more room', () => {
  render(<Workspace total={1234567} />)
  const pagination = screen.getByRole('navigation', {
    name: 'Table pagination',
  })

  expect(pagination).not.toHaveClass('overflow-clip')
  expect(pagination).toHaveClass('flex-wrap')
  expect(
    within(pagination).getByRole('button', { name: 'Go to next page' })
  ).toBeEnabled()
  expect(
    within(pagination).getByRole('combobox', { name: 'Rows per page' })
  ).toBeEnabled()
})

it('reserves the full narrow-screen row for search so filter controls cannot squeeze out the query', () => {
  render(<Workspace />)
  const search = screen.getByRole('textbox', { name: 'Search connections' })
  expect(search.closest('label')).toHaveClass('basis-full')
})

it('keeps filters and view preferences through a failed request while hiding stale rows and bulk actions', async () => {
  const user = userEvent.setup()
  const result = render(<Workspace />)
  await user.type(
    screen.getByRole('textbox', { name: 'Search connections' }),
    'Production'
  )
  await user.click(
    screen.getByRole('checkbox', { name: 'Select Production app' })
  )
  expect(screen.getByRole('toolbar')).toBeVisible()

  result.rerender(<Workspace failed />)
  expect(screen.getByRole('alert')).toHaveTextContent(
    'Records could not be loaded'
  )
  expect(screen.queryByRole('table')).not.toBeInTheDocument()
  expect(screen.queryByRole('toolbar')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('navigation', { name: 'Table pagination' })
  ).not.toBeInTheDocument()
  expect(
    screen.getByRole('textbox', { name: 'Search connections' })
  ).toHaveValue('Production')
  await user.click(screen.getByRole('button', { name: 'Card view' }))

  result.rerender(<Workspace />)
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  expect(screen.getByText('Production app')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Card view' })).toHaveAttribute(
    'aria-pressed',
    'true'
  )
})
