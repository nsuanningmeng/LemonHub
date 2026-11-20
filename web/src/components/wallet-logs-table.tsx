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
import { useQuery } from '@tanstack/react-query'
import type { ColumnDef } from '@tanstack/react-table'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { ErrorState } from '@/components/error-state'
import { StatusBadge } from '@/components/status-badge'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { formatTimestampToDate } from '@/lib/format'
import {
  formatMilliYuan,
  getWalletLogTypeOptions,
  WALLET_LOG_TYPES,
  type WalletLog,
} from '@/lib/wallet-log'

function useWalletLogColumns(): ColumnDef<WalletLog>[] {
  const { t } = useTranslation()
  return [
    {
      accessorKey: 'created_time',
      header: t('Time'),
      meta: { mobileTitle: true },
      cell: ({ row }) => (
        <div className='min-w-[150px] font-mono text-sm'>
          {formatTimestampToDate(row.getValue('created_time'))}
        </div>
      ),
      size: 170,
    },
    {
      accessorKey: 'type',
      header: t('Type'),
      meta: { mobileBadge: true },
      cell: ({ row }) => {
        const type = row.getValue('type') as number
        const cfg = WALLET_LOG_TYPES[type]
        if (!cfg) return <span className='text-muted-foreground'>-</span>
        return (
          <StatusBadge
            label={t(cfg.labelKey)}
            variant={cfg.variant}
            copyable={false}
            className='-ml-1.5'
          />
        )
      },
      size: 140,
    },
    {
      accessorKey: 'amount',
      header: t('Amount'),
      cell: ({ row }) => {
        const amount = row.getValue('amount') as number
        const positive = amount >= 0
        return (
          <span
            className={`font-mono text-sm ${positive ? 'text-success' : 'text-destructive'}`}
          >
            {positive ? '+' : '-'}¥{formatMilliYuan(Math.abs(amount))}
          </span>
        )
      },
      size: 130,
    },
    {
      accessorKey: 'balance_after',
      header: t('Balance After'),
      meta: { mobileHidden: true },
      cell: ({ row }) => (
        <span className='font-mono text-sm'>
          ¥{formatMilliYuan(row.getValue('balance_after') as number)}
        </span>
      ),
      size: 130,
    },
    {
      accessorKey: 'remark',
      header: t('Remark'),
      enableSorting: false,
      meta: { mobileHidden: true },
      cell: ({ row }) => (
        <div className='text-muted-foreground max-w-[240px] truncate text-sm'>
          {(row.getValue('remark') as string) || '-'}
        </div>
      ),
      size: 240,
    },
  ]
}

// ============================================================================
// Wallet Logs Table
// ============================================================================

type WalletLogsTableProps = {
  /** Stable react-query key prefix (e.g. `site-admin-wallet-logs`). */
  queryKey: string
  /** Fetch one page of wallet logs. Must already unwrap the API envelope. */
  fetchLogs: (params: {
    p: number
    page_size: number
    type?: number
  }) => Promise<{ items: WalletLog[]; total: number }>
  /** Bump to force a refetch after recharge/adjust. */
  refreshKey?: number
}

/**
 * Self-contained, server-paginated wallet ledger table. Manages its own
 * pagination + type filter via local state (no URL state) so it can be embedded
 * in dialogs/tabs. Amounts are rendered in 元 (厘 / 1000).
 */
export function WalletLogsTable({
  queryKey,
  fetchLogs,
  refreshKey = 0,
}: WalletLogsTableProps) {
  const { t } = useTranslation()
  const columns = useWalletLogColumns()
  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize: 10 })
  const [type, setType] = useState<number | undefined>(undefined)

  const { data, isLoading, isFetching, error, refetch } = useQuery({
    queryKey: [
      queryKey,
      pagination.pageIndex + 1,
      pagination.pageSize,
      type,
      refreshKey,
    ],
    queryFn: () =>
      fetchLogs({
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
        type,
      }),
    placeholderData: (previousData) => previousData,
  })

  const { table } = useDataTable({
    data: data?.items ?? [],
    columns,
    enableRowSelection: false,
    pagination,
    onPaginationChange: setPagination,
    manualPagination: true,
    totalCount: data?.total ?? 0,
  })

  const typeOptions = useMemo(() => getWalletLogTypeOptions(t), [t])
  const selectItems = useMemo(
    () => [{ value: 'all', label: t('All Types') }, ...typeOptions],
    [t, typeOptions]
  )

  const toolbar = (
    <div className='flex items-center gap-2'>
      <Select
        items={selectItems}
        value={type === undefined ? 'all' : String(type)}
        onValueChange={(v) => {
          if (v === null) return
          setType(v === 'all' ? undefined : Number.parseInt(String(v), 10))
          setPagination((p) => ({ ...p, pageIndex: 0 }))
        }}
      >
        <SelectTrigger aria-label={t('Type')} className='w-[200px]'>
          <SelectValue />
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          <SelectGroup>
            {selectItems.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </div>
  )

  if (error && !data) {
    return (
      <div className='space-y-3'>
        {toolbar}
        <ErrorState
          description={error.message}
          onRetry={() => void refetch()}
        />
      </div>
    )
  }

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      emptyTitle={t('No Wallet Logs Found')}
      skeletonKeyPrefix={`${queryKey}-skeleton`}
      applyHeaderSize
      fixedHeight={false}
      paginationInFooter={false}
      toolbar={toolbar}
    />
  )
}
