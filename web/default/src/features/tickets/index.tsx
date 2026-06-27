import { useQuery } from '@tanstack/react-query'
import { ChevronRight, Plus } from 'lucide-react'
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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
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
        return { items: [], total: 0, page, page_size: PAGE_SIZE }
      }
      return res.data
    },
    placeholderData: (prev) => prev,
  })

  const tickets = listQuery.data?.items ?? []
  const total = listQuery.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const typeNameOf = (key: string) =>
    types.find((ty) => ty.key === key)?.name ?? key

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          {t('Support Tickets')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Select
            value={status}
            onValueChange={(value) => {
              if (value == null) return
              setStatus(value)
              setPage(1)
            }}
          >
            <SelectTrigger size='sm' className='w-[150px]'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectItem value={ALL_STATUS}>{t('All statuses')}</SelectItem>
              {TICKET_STATUS_OPTIONS.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {t(option.labelKey)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button onClick={() => setNewOpen(true)} disabled={!enabled}>
            <Plus className='size-3.5' />
            {t('New Ticket')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='mx-auto flex w-full max-w-3xl flex-col gap-3'>
            {!enabled && (
              <div className='text-muted-foreground rounded-lg border border-dashed py-10 text-center text-sm'>
                {t('Support tickets are currently disabled.')}
              </div>
            )}

            {enabled && listQuery.isLoading && (
              <div className='flex justify-center py-10'>
                <Spinner />
              </div>
            )}

            {enabled && !listQuery.isLoading && tickets.length === 0 && (
              <div className='text-muted-foreground rounded-lg border border-dashed py-10 text-center text-sm'>
                {t('You have no tickets yet.')}
              </div>
            )}

            {enabled &&
              tickets.map((ticket) => (
                <button
                  key={ticket.id}
                  type='button'
                  onClick={() => setDetailId(ticket.id)}
                  className='hover:bg-muted/40 flex w-full items-center gap-3 rounded-lg border p-3 text-left transition-colors'
                >
                  <div className='min-w-0 flex-1'>
                    <div className='flex items-center gap-2'>
                      <span className='truncate font-medium'>
                        {ticket.title}
                      </span>
                      <TicketPriorityBadge priority={ticket.priority} />
                      <TicketStatusBadge status={ticket.status} />
                    </div>
                    <div className='text-muted-foreground mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs'>
                      <span>{typeNameOf(ticket.type)}</span>
                      <span>
                        {t('Messages')}: {ticket.message_num ?? 0}
                      </span>
                      <span>
                        {t('Last activity')}:{' '}
                        {formatTimestamp(
                          ticket.last_reply_at || ticket.updated_at
                        )}
                      </span>
                    </div>
                  </div>
                  <ChevronRight className='text-muted-foreground size-4 shrink-0' />
                </button>
              ))}

            {enabled && totalPages > 1 && (
              <div className='flex items-center justify-between pt-1'>
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
