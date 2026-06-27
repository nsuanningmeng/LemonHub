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
import type { ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'

import { LongText } from '@/components/long-text'
import { TableId } from '@/components/table-id'
import { Button } from '@/components/ui/button'
import { formatTimestamp } from '@/lib/format'

import type { Ticket, TicketTypeConfig } from '../../types'
import { TicketPriorityBadge } from '../ticket-priority-badge'
import { TicketStatusBadge } from '../ticket-status-badge'

interface UseAdminTicketColumnsOptions {
  types: TicketTypeConfig[]
  onView: (id: number) => void
}

export function useAdminTicketColumns({
  types,
  onView,
}: UseAdminTicketColumnsOptions): ColumnDef<Ticket>[] {
  const { t } = useTranslation()
  const typeNameOf = (key: string) =>
    types.find((ty) => ty.key === key)?.name ?? key

  return [
    {
      accessorKey: 'id',
      header: t('ID'),
      cell: ({ row }) => (
        <TableId value={row.original.id} className='w-[60px]' />
      ),
      size: 80,
      meta: { mobileHidden: true },
    },
    {
      id: 'user',
      header: t('User'),
      cell: ({ row }) => (
        <div className='flex min-w-0 flex-col'>
          <span className='truncate text-sm font-medium'>
            {row.original.username || '-'}
          </span>
          {row.original.user_email && (
            <span className='text-muted-foreground truncate text-xs'>
              {row.original.user_email}
            </span>
          )}
        </div>
      ),
      enableSorting: false,
      size: 180,
    },
    {
      id: 'type',
      accessorKey: 'type',
      header: t('Type'),
      cell: ({ row }) => (
        <span className='text-sm'>{typeNameOf(row.original.type)}</span>
      ),
      enableSorting: false,
      size: 130,
    },
    {
      accessorKey: 'title',
      header: t('Title'),
      cell: ({ row }) => (
        <LongText className='max-w-[240px] font-medium'>
          {row.original.title}
        </LongText>
      ),
      enableSorting: false,
      size: 260,
      meta: { mobileTitle: true },
    },
    {
      id: 'priority',
      accessorKey: 'priority',
      header: t('Priority'),
      cell: ({ row }) => (
        <TicketPriorityBadge priority={row.original.priority} />
      ),
      enableSorting: false,
      size: 110,
    },
    {
      id: 'status',
      accessorKey: 'status',
      header: t('Status'),
      cell: ({ row }) => <TicketStatusBadge status={row.original.status} />,
      enableSorting: false,
      size: 120,
      meta: { mobileBadge: true },
    },
    {
      id: 'messages',
      header: t('Messages'),
      cell: ({ row }) => (
        <span className='text-sm tabular-nums'>
          {row.original.message_num ?? 0}
        </span>
      ),
      enableSorting: false,
      size: 90,
      meta: { mobileHidden: true },
    },
    {
      accessorKey: 'last_reply_at',
      header: t('Last activity'),
      cell: ({ row }) => (
        <span className='text-muted-foreground text-sm'>
          {formatTimestamp(
            row.original.last_reply_at || row.original.updated_at
          )}
        </span>
      ),
      size: 170,
      meta: { mobileHidden: true },
    },
    {
      id: 'actions',
      header: () => t('Actions'),
      cell: ({ row }) => (
        <Button
          variant='outline'
          size='xs'
          onClick={() => onView(row.original.id)}
        >
          {t('View')}
        </Button>
      ),
      size: 90,
      meta: { pinned: 'right' as const },
    },
  ]
}
