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
import { getRouteApi } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { Input } from '@/components/ui/input'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'

import { getAdminTickets, getTicketConfig } from '../../api'
import {
  TICKET_PRIORITY_OPTIONS,
  TICKET_STATUS_OPTIONS,
} from '../../constants'
import { useAdminTicketColumns } from './admin-ticket-columns'
import { AdminTicketDetailDialog } from './admin-ticket-detail-dialog'

const route = getRouteApi('/_authenticated/console/tickets-admin')

export function AdminTicketsTable() {
  const { t } = useTranslation()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const [detailId, setDetailId] = useState<number | null>(null)

  const configQuery = useQuery({
    queryKey: ['ticket', 'config'],
    queryFn: async () => {
      const res = await getTicketConfig()
      return res.data
    },
  })
  const types = configQuery.data?.types ?? []

  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
    globalFilter: { enabled: true, key: 'filter' },
    columnFilters: [
      { columnId: 'status', searchKey: 'status', type: 'array' },
      { columnId: 'type', searchKey: 'type', type: 'array' },
      { columnId: 'priority', searchKey: 'priority', type: 'array' },
      { columnId: 'user', searchKey: 'userId', type: 'string' },
    ],
  })

  const statusFilter =
    (columnFilters.find((filter) => filter.id === 'status')?.value as
      | string[]
      | undefined) ?? []
  const typeFilter =
    (columnFilters.find((filter) => filter.id === 'type')?.value as
      | string[]
      | undefined) ?? []
  const priorityFilter =
    (columnFilters.find((filter) => filter.id === 'priority')?.value as
      | string[]
      | undefined) ?? []
  const userFilter =
    (columnFilters.find((filter) => filter.id === 'user')?.value as string) ??
    ''

  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'admin-tickets',
      pagination.pageIndex + 1,
      pagination.pageSize,
      globalFilter,
      statusFilter,
      typeFilter,
      priorityFilter,
      userFilter,
    ],
    queryFn: async () => {
      const res = await getAdminTickets({
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
        status: statusFilter[0] ?? '',
        type: typeFilter[0] ?? '',
        priority: priorityFilter[0] ?? '',
        user_id: userFilter,
        keyword: globalFilter ?? '',
      })
      if (!res.success || !res.data) {
        toast.error(res.message || t('Failed to load tickets'))
        return { items: [], total: 0 }
      }
      return { items: res.data.items, total: res.data.total }
    },
    placeholderData: (previousData) => previousData,
  })

  const columns = useAdminTicketColumns({ types, onView: setDetailId })

  const { table } = useDataTable({
    data: data?.items ?? [],
    columns,
    columnFilters,
    globalFilter,
    pagination,
    onPaginationChange,
    onGlobalFilterChange,
    onColumnFiltersChange,
    manualPagination: true,
    manualFiltering: true,
    totalCount: data?.total ?? 0,
    ensurePageInRange,
  })

  const userIdInput = (
    <Input
      value={userFilter}
      onChange={(event) =>
        table.getColumn('user')?.setFilterValue(event.target.value)
      }
      placeholder={t('User ID')}
      className='w-full sm:w-[120px]'
    />
  )

  return (
    <>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={isLoading}
        isFetching={isFetching}
        emptyTitle={t('No Tickets Found')}
        emptyDescription={t('No tickets match the current filters.')}
        skeletonKeyPrefix='admin-tickets-skeleton'
        applyHeaderSize
        toolbarProps={{
          searchPlaceholder: t('Search by title or content...'),
          searchDebounceMs: 300,
          additionalSearch: userIdInput,
          filters: [
            {
              columnId: 'status',
              title: t('Status'),
              options: TICKET_STATUS_OPTIONS.map((option) => ({
                value: option.value,
                label: t(option.labelKey),
              })),
              singleSelect: true,
            },
            {
              columnId: 'type',
              title: t('Type'),
              options: types.map((ty) => ({ value: ty.key, label: ty.name })),
              singleSelect: true,
            },
            {
              columnId: 'priority',
              title: t('Priority'),
              options: TICKET_PRIORITY_OPTIONS.map((option) => ({
                value: option.value,
                label: t(option.labelKey),
              })),
              singleSelect: true,
            },
          ],
        }}
      />

      <AdminTicketDetailDialog
        ticketId={detailId}
        open={detailId != null}
        onOpenChange={(isOpen) => !isOpen && setDetailId(null)}
      />
    </>
  )
}
