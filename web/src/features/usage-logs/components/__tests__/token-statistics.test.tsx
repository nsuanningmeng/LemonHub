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
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { render, screen } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import { usageLogSchema, type UsageLog } from '../../data/schema'
import { useCommonLogsColumns } from '../columns/common-logs-columns'
import { UsageLogsMobileList } from '../usage-logs-mobile-card'

function TokenLog(props: { log: UsageLog; mobile: boolean }) {
  const columns = useCommonLogsColumns(false).filter(
    (column) =>
      'accessorKey' in column &&
      ['created_at', 'prompt_tokens'].includes(String(column.accessorKey))
  )
  const table = useReactTable({
    data: [props.log],
    columns,
    getCoreRowModel: getCoreRowModel(),
  })
  if (props.mobile) {
    return <UsageLogsMobileList table={table} logCategory='common' />
  }
  const cell = table
    .getRowModel()
    .rows[0].getAllCells()
    .find((item) => item.column.id === 'prompt_tokens')
  if (!cell) throw new Error('The token column is missing')
  return <>{flexRender(cell.column.columnDef.cell, cell.getContext())}</>
}

describe.each([false, true])('cache-only token log (mobile=%s)', (mobile) => {
  test('shows cache read and write counts when fresh input and output are zero', () => {
    const log = usageLogSchema.parse({
      id: 1,
      user_id: 1,
      created_at: 1728000000,
      type: 2,
      content: '',
      prompt_tokens: 0,
      completion_tokens: 0,
      other: JSON.stringify({ cache_tokens: 120, cache_creation_tokens: 30 }),
    })
    render(<TokenLog log={log} mobile={mobile} />)
    expect(screen.getByText('Cache↓ 120')).toBeVisible()
    expect(screen.getByText('↑ 30')).toBeVisible()
    expect(screen.getByText('0 / 0')).toBeVisible()
  })

  test('keeps the empty marker for a legacy log with no token usage', () => {
    const log = usageLogSchema.parse({
      id: 1,
      user_id: 1,
      created_at: 1728000000,
      type: 2,
      content: '',
    })
    render(<TokenLog log={log} mobile={mobile} />)
    for (const marker of screen.getAllByText('-')) expect(marker).toBeVisible()
    expect(screen.queryByText('0 / 0')).not.toBeInTheDocument()
  })
})
