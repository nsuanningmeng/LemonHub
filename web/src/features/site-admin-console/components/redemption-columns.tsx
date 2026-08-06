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
import { Ban, MoreHorizontal as DotsHorizontalIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { formatQuota, formatTimestampToDate } from '@/lib/format'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { MaskedValueDisplay } from '@/components/masked-value-display'
import { StatusBadge } from '@/components/status-badge'
import { TableId } from '@/components/table-id'
import { formatMilliYuan } from '@/components/wallet-logs-table'
import {
  REDEMPTION_STATUS,
  REDEMPTION_STATUSES,
  isRedemptionExpired,
} from '../constants'
import { type Redemption } from '../types'

export function useRedemptionColumns(
  onVoid: (redemption: Redemption) => void
): ColumnDef<Redemption>[] {
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
      accessorKey: 'status',
      header: t('Status'),
      meta: { mobileBadge: true },
      cell: ({ row }) => {
        const redemption = row.original
        const statusValue = row.getValue('status') as number
        if (isRedemptionExpired(redemption.expired_time, statusValue)) {
          return (
            <StatusBadge
              label={t('Expired')}
              variant='warning'
              copyable={false}
              className='-ml-1.5'
            />
          )
        }
        const config = REDEMPTION_STATUSES[statusValue]
        if (!config) return null
        return (
          <StatusBadge
            label={t(config.labelKey)}
            variant={config.variant}
            copyable={false}
            className='-ml-1.5'
          />
        )
      },
      size: 110,
    },
    {
      id: 'code',
      accessorKey: 'key',
      header: t('Code'),
      enableSorting: false,
      cell: function CodeCell({ row }) {
        const key = row.original.key
        const maskedKey =
          key.length > 16
            ? `${key.slice(0, 8)}${'*'.repeat(16)}${key.slice(-8)}`
            : key
        return (
          <MaskedValueDisplay
            label={t('Full Code')}
            fullValue={key}
            maskedValue={maskedKey}
            copyTooltip={t('Copy code')}
            copyAriaLabel={t('Copy redemption code')}
          />
        )
      },
      size: 300,
    },
    {
      accessorKey: 'quota',
      header: t('Quota'),
      cell: ({ row }) => (
        <StatusBadge
          label={formatQuota(row.getValue('quota') as number)}
          variant='neutral'
          copyable={false}
          className='-ml-1.5'
        />
      ),
      size: 120,
    },
    {
      accessorKey: 'cost_amount',
      header: t('Cost'),
      meta: { mobileHidden: true },
      cell: ({ row }) => (
        <span className='font-mono text-sm'>
          ¥{formatMilliYuan(row.getValue('cost_amount') as number)}
        </span>
      ),
      size: 110,
    },
    {
      accessorKey: 'created_time',
      header: t('Created'),
      meta: { mobileHidden: true },
      cell: ({ row }) => (
        <div className='min-w-[150px] font-mono text-sm'>
          {formatTimestampToDate(row.getValue('created_time'))}
        </div>
      ),
      size: 170,
    },
    {
      accessorKey: 'expired_time',
      header: t('Expires'),
      meta: { mobileHidden: true },
      cell: ({ row }) => {
        const expiredTime = row.getValue('expired_time') as number
        if (expiredTime === 0) {
          return (
            <StatusBadge
              label={t('Never')}
              variant='neutral'
              copyable={false}
              className='-ml-1.5'
            />
          )
        }
        return (
          <div className='min-w-[150px] font-mono text-sm'>
            {formatTimestampToDate(expiredTime)}
          </div>
        )
      },
      size: 170,
    },
    {
      id: 'actions',
      header: () => t('Actions'),
      cell: ({ row }) => {
        const redemption = row.original
        const canVoid = redemption.status === REDEMPTION_STATUS.ENABLED
        return (
          <div className='-ml-2'>
            <DropdownMenu modal={false}>
              <DropdownMenuTrigger
                render={
                  <Button
                    variant='ghost'
                    className='data-popup-open:bg-muted flex h-8 w-8 p-0'
                  />
                }
              >
                <DotsHorizontalIcon className='h-4 w-4' />
                <span className='sr-only'>{t('Open menu')}</span>
              </DropdownMenuTrigger>
              <DropdownMenuContent align='end' className='w-[160px]'>
                <DropdownMenuItem
                  onClick={() => onVoid(redemption)}
                  disabled={!canVoid}
                  className='text-destructive focus:text-destructive'
                >
                  {t('Void')}
                  <DropdownMenuShortcut>
                    <Ban size={16} />
                  </DropdownMenuShortcut>
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        )
      },
      meta: { pinned: 'right' as const },
      size: 88,
    },
  ]
}
