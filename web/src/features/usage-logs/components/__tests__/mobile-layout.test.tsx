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
import { render, screen, within } from '@testing-library/react'
import { expect, it } from 'vitest'

import { usageLogSchema } from '../../data/schema'
import { UsageLogsMobileList } from '../usage-logs-mobile-card'

const log = usageLogSchema.parse({
  id: 1,
  user_id: 1,
  created_at: 1736000000,
  type: 2,
  content: '',
  model_name: 'workspace-model',
  use_time: 2,
  prompt_tokens: 860,
  completion_tokens: 420,
  is_stream: true,
  other: JSON.stringify({ frt: 150 }),
})

function MobileLog() {
  const table = useReactTable({
    data: [log],
    columns: [{ accessorKey: 'created_at' }, { accessorKey: 'model_name' }],
    getCoreRowModel: getCoreRowModel(),
  })
  return <UsageLogsMobileList table={table} logCategory='common' />
}

it('gives timing the full mobile card width and lets stream details wrap without overlapping', () => {
  render(<MobileLog />)
  const timing = screen.getByRole('group', { name: 'Timing' })
  expect(timing).toHaveClass('col-span-2', 'flex-wrap')
  expect(within(timing).getByText('First token')).toBeVisible()
  expect(within(timing).getByText('Duration')).toBeVisible()
  expect(within(timing).getByText('Stream')).toBeVisible()
  expect(within(timing).getByText('210 t/s')).toBeVisible()
  expect(screen.getByText('860 / 420')).toBeVisible()
})
