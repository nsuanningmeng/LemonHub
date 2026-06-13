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
import { type ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { formatTimestampToDate } from '@/lib/format'
import { StatusBadge } from '@/components/status-badge'
import { TableId } from '@/components/table-id'
import { SITE_STATUSES } from '../constants'
import { type Site } from '../types'
import { DataTableRowActions } from './data-table-row-actions'

export function useSubSiteColumns(): ColumnDef<Site>[] {
  const { t } = useTranslation()
  return [
    {
      accessorKey: 'id',
      header: t('ID'),
      meta: { mobileHidden: true },
      cell: ({ row }) => (
        <TableId value={row.getValue('id') as number} className='w-[60px]' />
      ),
      size: 80,
    },
    {
      accessorKey: 'name',
      header: t('Name'),
      meta: { mobileTitle: true },
      cell: ({ row }) => (
        <div className='max-w-[150px] truncate font-medium'>
          {row.getValue('name')}
        </div>
      ),
      size: 160,
    },
    {
      accessorKey: 'domains',
      header: t('Domains'),
      enableSorting: false,
      cell: ({ row }) => {
        const site = row.original
        const domainList = site.domains
        return (
          <div className='text-muted-foreground max-w-[200px] truncate text-sm'>
            {domainList.length === 0 ? '-' : domainList.join(', ')}
          </div>
        )
      },
      size: 220,
    },
    {
      accessorKey: 'owner_username',
      header: t('Owner Username'),
      cell: ({ row }) => (
        <div className='max-w-[120px] truncate text-sm'>
          {(row.getValue('owner_username') as string) || '-'}
        </div>
      ),
      size: 140,
    },
    {
      accessorKey: 'status',
      header: t('Status'),
      meta: { mobileBadge: true },
      cell: ({ row }) => {
        const statusValue = row.getValue('status') as number
        const statusConfig = SITE_STATUSES[statusValue]
        if (!statusConfig) return null
        return (
          <StatusBadge
            label={t(statusConfig.labelKey)}
            variant={statusConfig.variant}
            copyable={false}
            className='-ml-1.5'
          />
        )
      },
      filterFn: (row, id, value) => {
        return (value as string[]).includes(String(row.getValue(id)))
      },
      size: 110,
    },
    {
      accessorKey: 'wallet_balance',
      header: t('Wallet Balance'),
      meta: { mobileHidden: true },
      cell: ({ row }) => {
        const balance = row.getValue('wallet_balance') as number
        return (
          <span className='font-mono text-sm'>
            ¥{(balance / 1000).toFixed(3)}
          </span>
        )
      },
      size: 130,
    },
    {
      accessorKey: 'discount_rate',
      header: t('Discount Rate'),
      meta: { mobileHidden: true },
      cell: ({ row }) => {
        const rate = row.getValue('discount_rate') as number
        return (
          <span className='font-mono text-sm'>
            {rate === 10000
              ? t('No Discount')
              : `${(rate / 100).toFixed(0)}%`}
          </span>
        )
      },
      size: 120,
    },
    {
      accessorKey: 'created_time',
      header: t('Created'),
      meta: { mobileHidden: true },
      cell: ({ row }) => (
        <div className='min-w-[160px] font-mono text-sm'>
          {formatTimestampToDate(row.getValue('created_time'))}
        </div>
      ),
      size: 180,
    },
    {
      id: 'actions',
      header: () => t('Actions'),
      cell: ({ row }) => <DataTableRowActions row={row} />,
      meta: { pinned: 'right' as const },
      size: 88,
    },
  ]
}
