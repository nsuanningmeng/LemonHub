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
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'

import { createTicket } from '../api'
import {
  DEFAULT_TICKET_PRIORITY,
  TICKET_PRIORITY_OPTIONS,
} from '../constants'
import type {
  TicketPriority,
  TicketTypeConfig,
  UploadedAttachment,
} from '../types'
import { MessageEditor } from './message-editor'

interface NewTicketDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  types: TicketTypeConfig[]
  onCreated: (id: number) => void
}

export function NewTicketDialog({
  open,
  onOpenChange,
  types,
  onCreated,
}: NewTicketDialogProps) {
  const { t } = useTranslation()
  const [typeKey, setTypeKey] = useState('')
  const [title, setTitle] = useState('')
  const [content, setContent] = useState('')
  const [priority, setPriority] = useState<TicketPriority>(
    DEFAULT_TICKET_PRIORITY
  )
  const [attachments, setAttachments] = useState<UploadedAttachment[]>([])
  const [submitting, setSubmitting] = useState(false)
  const lastTemplateRef = useRef('')

  // Reset the form whenever the dialog opens, seeding the first type's
  // prompt template into the content field as guidance.
  useEffect(() => {
    if (!open) return
    const first = types[0]
    const template = first?.prompt_template ?? ''
    setTypeKey(first?.key ?? '')
    setTitle('')
    setContent(template)
    setPriority(DEFAULT_TICKET_PRIORITY)
    setAttachments([])
    lastTemplateRef.current = template
  }, [open, types])

  const handleTypeChange = (value: string | null) => {
    if (value == null) return
    setTypeKey(value)
    const template = types.find((ty) => ty.key === value)?.prompt_template ?? ''
    // Only overwrite content the user has not customised.
    setContent((prev) =>
      prev.trim() === '' || prev === lastTemplateRef.current ? template : prev
    )
    lastTemplateRef.current = template
  }

  const selectedType = types.find((ty) => ty.key === typeKey)

  const handleSubmit = async () => {
    if (!typeKey) {
      toast.error(t('Please select a ticket type'))
      return
    }
    if (!title.trim()) {
      toast.error(t('Please enter a title'))
      return
    }
    if (!content.trim()) {
      toast.error(t('Please enter the ticket details'))
      return
    }

    setSubmitting(true)
    try {
      const res = await createTicket({
        type: typeKey,
        title: title.trim(),
        content,
        priority,
        attachment_ids: attachments.map((att) => att.id),
      })
      if (res.success && res.data) {
        toast.success(t('Ticket created'))
        onOpenChange(false)
        onCreated(res.data.id)
      } else {
        toast.error(res.message || t('Failed to create ticket'))
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-2xl'>
        <div className='flex max-h-[80vh] flex-col gap-4'>
          <DialogHeader>
            <DialogTitle>{t('New Ticket')}</DialogTitle>
            <DialogDescription>
              {t('Describe your issue and our team will get back to you.')}
            </DialogDescription>
          </DialogHeader>

          <div className='flex-1 space-y-4 overflow-y-auto'>
            <div className='space-y-1.5'>
              <Label>{t('Type')}</Label>
              <Select
                value={typeKey}
                onValueChange={handleTypeChange}
                items={types.map((ty) => ({ value: ty.key, label: ty.name }))}
              >
                <SelectTrigger className='w-full'>
                  <SelectValue placeholder={t('Select a type')} />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  {types.map((ty) => (
                    <SelectItem key={ty.key} value={ty.key}>
                      {ty.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className='space-y-1.5'>
              <Label>{t('Priority')}</Label>
              <Select
                value={priority}
                onValueChange={(value) => {
                  if (value == null) return
                  setPriority(value as TicketPriority)
                }}
              >
                <SelectTrigger className='w-full'>
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
            </div>

            <div className='space-y-1.5'>
              <Label>{t('Title')}</Label>
              <Input
                value={title}
                onChange={(event) => setTitle(event.target.value)}
                placeholder={t('Brief summary of your issue')}
              />
            </div>

            <div className='space-y-1.5'>
              <Label>{t('Details')}</Label>
              <MessageEditor
                content={content}
                onContentChange={setContent}
                attachments={attachments}
                onAttachmentsChange={setAttachments}
                placeholder={
                  selectedType?.prompt_template ||
                  t('Describe your issue in detail...')
                }
                disabled={submitting}
              />
            </div>
          </div>

          <DialogFooter>
            <Button
              variant='outline'
              onClick={() => onOpenChange(false)}
              disabled={submitting}
            >
              {t('Cancel')}
            </Button>
            <Button onClick={handleSubmit} disabled={submitting}>
              {submitting && <Spinner className='size-3.5' />}
              {t('Submit')}
            </Button>
          </DialogFooter>
        </div>
      </DialogContent>
    </Dialog>
  )
}
