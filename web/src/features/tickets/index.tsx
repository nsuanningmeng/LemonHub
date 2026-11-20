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
import { ChevronRight, Inbox, MessageSquare, Plus } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { formatTimestamp } from '@/lib/format'

import { getTicketConfig, getTickets } from './api'
import { NewTicketDialog } from './components/new-ticket-dialog'
import { TicketDetailDialog } from './components/ticket-detail-dialog'
import { TicketPriorityBadge } from './components/ticket-priority-badge'
import { TicketStatusBadge } from './components/ticket-status-badge'
import { TICKET_STATUS_OPTIONS } from './constants'

const ALL_STATUS = 'all'
const PAGE_SIZE = 20

export function Tickets() {
  const { t } = useTranslation()
  const [newOpen, setNewOpen] = useState(false)
  const [detailId, setDetailId] = useState<number | null>(null)
  const [status, setStatus] = useState(ALL_STATUS)
  const [page, setPage] = useState(1)
  const statusItems = [
    { value: ALL_STATUS, label: t('All statuses') },
    ...TICKET_STATUS_OPTIONS.map((option) => ({
      value: option.value,
      label: t(option.labelKey),
    })),
  ]

  const configQuery = useQuery({
    queryKey: ['ticket', 'config'],
    queryFn: async () => {
      const res = await getTicketConfig()
      return res.data
    },
  })
  const types = configQuery.data?.types ?? []
  const enabled = configQuery.data?.enabled !== false

  const listQuery = useQuery({
    queryKey: ['tickets', page, status],
    queryFn: async () => {
      const res = await getTickets({
        p: page,
        page_size: PAGE_SIZE,
        status: status === ALL_STATUS ? '' : status,
      })
      if (!res.success || !res.data) {
        throw new Error(res.message || 'Unable to load tickets')
      }
      return res.data
    },
    placeholderData: (prev) => prev,
    // Poll so staff replies bump the ticket's status/last-activity without a
    // manual refresh. Paused automatically when the tab is hidden.
    refetchInterval: 30_000,
  })

  const tickets = listQuery.data?.items ?? []
  const total = listQuery.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const typeNameOf = (key: string) =>
    types.find((ty) => ty.key === key)?.name ?? key

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Support inbox')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button onClick={() => setNewOpen(true)} disabled={!enabled}>
            <Plus className='size-3.5' />
            {t('New Ticket')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='bg-card flex w-full min-w-0 flex-col overflow-hidden rounded-xl border'>
            <div className='bg-muted/50 flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3 sm:px-6'>
              <h2 className='inline-flex items-center gap-2 font-semibold'>
                <Inbox aria-hidden='true' className='text-primary size-4' />
                {t('Ticket history')}
                {listQuery.isSuccess && enabled && (
                  <span className='text-muted-foreground text-sm tabular-nums'>
                    {total}
                  </span>
                )}
              </h2>
              <Select
                value={status}
                items={statusItems}
                onValueChange={(value) => {
                  if (value == null) return
                  setStatus(value)
                  setPage(1)
                }}
              >
                <SelectTrigger
                  size='sm'
                  aria-label={t('Status')}
                  className='max-w-full sm:w-36'
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  {statusItems.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            {!enabled && (
              <EmptyState
                title={t('Support tickets are currently disabled.')}
              />
            )}
            {enabled && listQuery.isLoading && <LoadingState />}
            {enabled && listQuery.isError && (
              <ErrorState onRetry={() => void listQuery.refetch()} />
            )}
            {enabled && listQuery.isSuccess && tickets.length === 0 && (
              <EmptyState
                icon={Inbox}
                title={
                  status === ALL_STATUS
                    ? t('No support requests yet')
                    : t('No tickets match the current filters.')
                }
                action={
                  status === ALL_STATUS ? undefined : (
                    <Button
                      size='sm'
                      variant='outline'
                      onClick={() => {
                        setStatus(ALL_STATUS)
                        setPage(1)
                      }}
                    >
                      {t('Clear filters')}
                    </Button>
                  )
                }
              />
            )}

            {enabled &&
              tickets.map((ticket) => (
                <button
                  key={ticket.id}
                  type='button'
                  onClick={() => setDetailId(ticket.id)}
                  className='hover:bg-muted/50 focus-visible:ring-ring grid w-full min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-2 border-b px-4 py-4 text-start last:border-b-0 focus-visible:ring-2 focus-visible:outline-none focus-visible:ring-inset sm:grid-cols-[minmax(0,1fr)_auto_minmax(9rem,auto)_auto] sm:px-6'
                >
                  <span className='min-w-0'>
                    <span className='block font-semibold wrap-anywhere'>
                      {ticket.title}
                    </span>
                    <span className='text-muted-foreground mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs'>
                      <span>{typeNameOf(ticket.type)}</span>
                      <span className='inline-flex items-center gap-1'>
                        <MessageSquare className='size-3' aria-hidden='true' />
                        <span className='sr-only'>{t('Messages')}</span>
                        {ticket.message_num ?? 0}
                      </span>
                    </span>
                  </span>
                  <span className='flex flex-wrap items-center justify-end gap-1.5'>
                    <TicketPriorityBadge priority={ticket.priority} />
                    <TicketStatusBadge status={ticket.status} />
                  </span>
                  <span className='text-muted-foreground min-w-0 text-xs sm:text-end'>
                    <span className='me-1.5 sm:me-0 sm:block'>
                      {t('Last activity')}
                    </span>
                    <span>
                      {formatTimestamp(
                        ticket.last_reply_at || ticket.updated_at
                      )}
                    </span>
                  </span>
                  <ChevronRight
                    aria-hidden='true'
                    className='text-muted-foreground size-4 justify-self-end'
                  />
                </button>
              ))}

            {enabled && totalPages > 1 && (
              <div className='flex flex-wrap items-center justify-between gap-2 px-4 py-2'>
                <span className='text-muted-foreground text-xs'>
                  {t('Page')} {page} / {totalPages}
                </span>
                <div className='flex gap-2'>
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={() => setPage((p) => Math.max(1, p - 1))}
                    disabled={page <= 1}
                  >
                    {t('Previous')}
                  </Button>
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                    disabled={page >= totalPages}
                  >
                    {t('Next')}
                  </Button>
                </div>
              </div>
            )}
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <NewTicketDialog
        open={newOpen}
        onOpenChange={setNewOpen}
        types={types}
        onCreated={(id) => {
          listQuery.refetch()
          setDetailId(id)
        }}
      />

      <TicketDetailDialog
        ticketId={detailId}
        open={detailId != null}
        onOpenChange={(isOpen) => !isOpen && setDetailId(null)}
      />
    </>
  )
}
