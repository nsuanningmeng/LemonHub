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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'

import {
  getAdminTicket,
  replyAdminTicket,
  setAdminTicketStatus,
} from '../../api'
import {
  TICKET_PRIORITY_OPTIONS,
  TICKET_STATUS_OPTIONS,
} from '../../constants'
import type { UploadedAttachment } from '../../types'
import { ConversationThread } from '../conversation-thread'
import { MessageEditor } from '../message-editor'
import { TicketPriorityBadge } from '../ticket-priority-badge'
import { TicketStatusBadge } from '../ticket-status-badge'

interface AdminTicketDetailDialogProps {
  ticketId: number | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function AdminTicketDetailDialog({
  ticketId,
  open,
  onOpenChange,
}: AdminTicketDetailDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [reply, setReply] = useState('')
  const [attachments, setAttachments] = useState<UploadedAttachment[]>([])
  const [sending, setSending] = useState(false)
  const [statusUpdating, setStatusUpdating] = useState(false)
  const [priorityUpdating, setPriorityUpdating] = useState(false)

  const detailQuery = useQuery({
    queryKey: ['admin-ticket', ticketId],
    queryFn: async () => {
      const res = await getAdminTicket(ticketId as number)
      if (!res.success || !res.data) {
        throw new Error(res.message || t('Failed to load ticket'))
      }
      return res.data
    },
    enabled: open && ticketId != null,
    // Poll while the dialog is open so a customer's new reply appears without a
    // manual page refresh. React Query pauses this interval when the tab is
    // hidden (refetchIntervalInBackground defaults to false).
    refetchInterval: 10_000,
  })

  useEffect(() => {
    if (open) {
      setReply('')
      setAttachments([])
    }
  }, [open, ticketId])

  const ticket = detailQuery.data?.ticket
  const messages = detailQuery.data?.messages ?? []

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ['admin-ticket', ticketId] })
    queryClient.invalidateQueries({ queryKey: ['admin-tickets'] })
  }

  const handleSend = async () => {
    if (ticketId == null) return
    if (!reply.trim() && attachments.length === 0) {
      toast.error(t('Please enter a reply'))
      return
    }
    setSending(true)
    try {
      const res = await replyAdminTicket(ticketId, {
        content: reply,
        attachment_ids: attachments.map((att) => att.id),
      })
      if (res.success) {
        setReply('')
        setAttachments([])
        refresh()
      } else {
        toast.error(res.message || t('Failed to send reply'))
      }
    } finally {
      setSending(false)
    }
  }

  const handleStatusChange = async (value: string | null) => {
    if (value == null || ticketId == null) return
    setStatusUpdating(true)
    try {
      const res = await setAdminTicketStatus(ticketId, { status: value })
      if (res.success) {
        toast.success(t('Status updated'))
        refresh()
      } else {
        toast.error(res.message || t('Failed to update status'))
      }
    } finally {
      setStatusUpdating(false)
    }
  }

  const handlePriorityChange = async (value: string | null) => {
    if (value == null || ticketId == null) return
    setPriorityUpdating(true)
    try {
      const res = await setAdminTicketStatus(ticketId, { priority: value })
      if (res.success) {
        toast.success(t('Priority updated'))
        refresh()
      } else {
        toast.error(res.message || t('Failed to update priority'))
      }
    } finally {
      setPriorityUpdating(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-2xl'>
        <div className='flex max-h-[85vh] min-w-0 flex-col gap-4'>
          <DialogHeader>
            <div className='flex min-w-0 items-start gap-2 pr-6'>
              <DialogTitle className='min-w-0 leading-snug break-words'>
                {ticket?.title ?? t('Ticket')}
              </DialogTitle>
              {ticket && (
                <div className='flex shrink-0 items-center gap-2'>
                  <TicketPriorityBadge priority={ticket.priority} />
                  <TicketStatusBadge status={ticket.status} />
                </div>
              )}
            </div>
            <DialogDescription className='break-words [overflow-wrap:anywhere]'>
              {ticket
                ? `#${ticket.id} · ${ticket.username ?? ''} ${
                    ticket.user_email ? `(${ticket.user_email})` : ''
                  }`.trim()
                : ''}
            </DialogDescription>
          </DialogHeader>

          <div className='min-h-24 flex-1 overflow-y-auto'>
            {detailQuery.isLoading ? (
              <div className='flex justify-center py-8'>
                <Spinner />
              </div>
            ) : (
              <ConversationThread messages={messages} />
            )}
          </div>

          <div className='space-y-3 border-t pt-3'>
            <div className='flex flex-wrap items-center gap-x-4 gap-y-2'>
              <div className='flex items-center gap-2'>
                <Label className='shrink-0'>{t('Status')}</Label>
                <Select
                  value={ticket?.status ?? ''}
                  onValueChange={handleStatusChange}
                  disabled={!ticket || statusUpdating}
                >
                  <SelectTrigger size='sm' className='w-[180px]'>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    {TICKET_STATUS_OPTIONS.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {t(option.labelKey)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {statusUpdating && <Spinner className='size-3.5' />}
              </div>

              <div className='flex items-center gap-2'>
                <Label className='shrink-0'>{t('Priority')}</Label>
                <Select
                  value={ticket?.priority ?? ''}
                  onValueChange={handlePriorityChange}
                  disabled={!ticket || priorityUpdating}
                >
                  <SelectTrigger size='sm' className='w-[150px]'>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    {TICKET_PRIORITY_OPTIONS.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {t(option.labelKey)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {priorityUpdating && <Spinner className='size-3.5' />}
              </div>
            </div>

            <MessageEditor
              content={reply}
              onContentChange={setReply}
              attachments={attachments}
              onAttachmentsChange={setAttachments}
              placeholder={t('Write a reply...')}
              disabled={sending}
            />
            <div className='flex justify-end'>
              <Button onClick={handleSend} disabled={sending || !ticket}>
                {sending && <Spinner className='size-3.5' />}
                {t('Send')}
              </Button>
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
