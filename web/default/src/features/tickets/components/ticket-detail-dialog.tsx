import { useQuery, useQueryClient } from '@tanstack/react-query'
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
import { Spinner } from '@/components/ui/spinner'

import { closeTicket, getTicket, replyTicket } from '../api'
import type { UploadedAttachment } from '../types'
import { ConversationThread } from './conversation-thread'
import { MessageEditor } from './message-editor'
import { TicketPriorityBadge } from './ticket-priority-badge'
import { TicketStatusBadge } from './ticket-status-badge'

interface TicketDetailDialogProps {
  ticketId: number | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function TicketDetailDialog({
  ticketId,
  open,
  onOpenChange,
}: TicketDetailDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [reply, setReply] = useState('')
  const [attachments, setAttachments] = useState<UploadedAttachment[]>([])
  const [sending, setSending] = useState(false)
  const [closing, setClosing] = useState(false)

  const detailQuery = useQuery({
    queryKey: ['ticket', ticketId],
    queryFn: async () => {
      const res = await getTicket(ticketId as number)
      if (!res.success || !res.data) {
        throw new Error(res.message || t('Failed to load ticket'))
      }
      return res.data
    },
    enabled: open && ticketId != null,
  })

  useEffect(() => {
    if (open) {
      setReply('')
      setAttachments([])
    }
  }, [open, ticketId])

  const ticket = detailQuery.data?.ticket
  const messages = detailQuery.data?.messages ?? []
  const isClosed = ticket?.status === 'closed'

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ['ticket', ticketId] })
    queryClient.invalidateQueries({ queryKey: ['tickets'] })
  }

  const handleSend = async () => {
    if (ticketId == null) return
    if (!reply.trim() && attachments.length === 0) {
      toast.error(t('Please enter a reply'))
      return
    }
    setSending(true)
    try {
      const res = await replyTicket(ticketId, {
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

  const handleClose = async () => {
    if (ticketId == null) return
    setClosing(true)
    try {
      const res = await closeTicket(ticketId)
      if (res.success) {
        toast.success(t('Ticket closed'))
        refresh()
      } else {
        toast.error(res.message || t('Failed to close ticket'))
      }
    } finally {
      setClosing(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-2xl'>
        <div className='flex max-h-[85vh] flex-col gap-4'>
          <DialogHeader>
            <div className='flex items-center gap-2 pr-6'>
              <DialogTitle className='truncate'>
                {ticket?.title ?? t('Ticket')}
              </DialogTitle>
              {ticket && <TicketPriorityBadge priority={ticket.priority} />}
              {ticket && <TicketStatusBadge status={ticket.status} />}
            </div>
            <DialogDescription>
              {ticket ? `#${ticket.id}` : ''}
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

          {isClosed ? (
            <div className='text-muted-foreground rounded-lg border border-dashed py-3 text-center text-sm'>
              {t('This ticket is closed.')}
            </div>
          ) : (
            <div className='space-y-3 border-t pt-3'>
              <MessageEditor
                content={reply}
                onContentChange={setReply}
                attachments={attachments}
                onAttachmentsChange={setAttachments}
                placeholder={t('Write a reply...')}
                disabled={sending}
              />
              <div className='flex items-center justify-between gap-2'>
                <Button
                  variant='destructive'
                  onClick={handleClose}
                  disabled={closing || sending || !ticket}
                >
                  {closing && <Spinner className='size-3.5' />}
                  {t('Close ticket')}
                </Button>
                <Button
                  onClick={handleSend}
                  disabled={sending || closing || !ticket}
                >
                  {sending && <Spinner className='size-3.5' />}
                  {t('Send')}
                </Button>
              </div>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
