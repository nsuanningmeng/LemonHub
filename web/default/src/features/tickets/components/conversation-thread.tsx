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
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Markdown } from '@/components/ui/markdown'
import { formatTimestamp } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { TicketMessage } from '../types'

export function ConversationThread({
  messages,
}: {
  messages: TicketMessage[]
}) {
  const { t } = useTranslation()

  if (messages.length === 0) {
    return (
      <p className='text-muted-foreground py-6 text-center text-sm'>
        {t('No messages yet')}
      </p>
    )
  }

  return (
    <div className='space-y-3'>
      {messages.map((message) => (
        <div
          key={message.id}
          className={cn(
            'rounded-lg border p-3',
            message.is_admin ? 'border-primary/20 bg-primary/5' : 'bg-muted/40'
          )}
        >
          <div className='mb-1.5 flex items-center gap-2'>
            <span className='truncate text-sm font-medium'>
              {message.username}
            </span>
            <Badge variant={message.is_admin ? 'default' : 'secondary'}>
              {message.is_admin ? t('Staff') : t('User')}
            </Badge>
            <span className='text-muted-foreground ml-auto shrink-0 text-xs'>
              {formatTimestamp(message.created_at)}
            </span>
          </div>
          <Markdown>{message.content}</Markdown>
          {message.attachments.length > 0 && (
            <div className='mt-2 flex flex-wrap gap-2'>
              {message.attachments.map((att) => (
                <a
                  key={att.id}
                  href={`/api/ticket/attachment/${att.id}`}
                  target='_blank'
                  rel='noopener noreferrer'
                >
                  <img
                    src={`/api/ticket/attachment/${att.id}`}
                    alt={att.file_name}
                    className='max-h-40 rounded-md border object-cover'
                  />
                </a>
              ))}
            </div>
          )}
        </div>
      ))}
    </div>
  )
}
