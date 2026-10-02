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
  getCoreRowModel,
  useReactTable,
  type ColumnDef,
} from '@tanstack/react-table'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it } from 'vitest'

import { StatusBadge } from '@/components/status-badge'

import { channelSchema, type Channel } from '../../types'
import { ChannelCard } from '../channel-card'
import { ChannelsProvider, useChannels } from '../channels-provider'

const clients: QueryClient[] = []
afterEach(() => clients.splice(0).forEach((client) => client.clear()))

const channel = channelSchema.parse({
  id: 42,
  key: '',
  type: 1,
  name: 'Production',
  status: 3,
  created_time: 0,
  test_time: 0,
  response_time: 350,
  balance_updated_time: 0,
  models: 'gpt-4o',
  group: 'vip',
})
const columns: ColumnDef<Channel>[] = [
  { id: 'name', cell: () => 'Production' },
  {
    id: 'status',
    cell: () => <StatusBadge label='Auto-disabled' copyable={false} />,
  },
  { id: 'priority', cell: () => <button type='button'>Priority 3</button> },
  { id: 'weight', cell: () => <button type='button'>Weight 5</button> },
  {
    id: 'balance',
    cell: () => <StatusBadge label='12 / 34' copyable={false} />,
  },
  {
    id: 'response_time',
    cell: () => <StatusBadge label='350 ms' copyable={false} />,
  },
  {
    id: 'test_time',
    cell: () => <StatusBadge label='Never' copyable={false} />,
  },
]

function CardHarness() {
  const context = useChannels()
  const table = useReactTable({
    data: [channel],
    columns,
    getCoreRowModel: getCoreRowModel(),
  })
  return (
    <>
      <button type='button' onClick={() => context.setSensitiveVisible(false)}>
        Hide sensitive data
      </button>
      <ChannelCard row={table.getRowModel().rows[0]} isSelected={false} />
    </>
  )
}

it('aligns card metric labels with their values and removes only metric badge padding', async () => {
  const client = new QueryClient()
  clients.push(client)
  render(
    <QueryClientProvider client={client}>
      <ChannelsProvider>
        <CardHarness />
      </ChannelsProvider>
    </QueryClientProvider>
  )

  expect(screen.getAllByRole('term').map((term) => term.textContent)).toEqual([
    'Used / Remaining',
    'Response',
    'Last Tested',
  ])
  expect(
    screen.getAllByRole('definition').map((value) => value.textContent)
  ).toEqual(['12 / 34', '350 ms', 'Never'])
  expect(
    screen.getByText('350 ms').closest('[data-slot="status-badge"]')
  ).not.toHaveClass('px-1.5')
  expect(
    screen.getByText('Auto-disabled').closest('[data-slot="status-badge"]')
  ).toHaveClass('px-1.5')
  expect(screen.getByRole('button', { name: 'Priority 3' })).toBeVisible()
  expect(screen.getByRole('button', { name: 'Weight 5' })).toBeVisible()

  await userEvent.click(
    screen.getByRole('button', { name: 'Hide sensitive data' })
  )
  expect(screen.queryByText('#42')).not.toBeInTheDocument()
  expect(screen.queryByText('vip')).not.toBeInTheDocument()
})
