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
import { Copy, Check } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'

import { getRequestBodyLog } from '../../api'

interface RequestBodyDialogProps {
  requestId: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

// Best-effort pretty-print for JSON request bodies; falls back to the raw text.
function formatBody(body: string): string {
  const trimmed = body.trim()
  if (!trimmed) return ''
  if (trimmed.startsWith('{') || trimmed.startsWith('[')) {
    try {
      return JSON.stringify(JSON.parse(trimmed), null, 2)
    } catch {
      return body
    }
  }
  return body
}

export function RequestBodyDialog({
  requestId,
  open,
  onOpenChange,
}: RequestBodyDialogProps) {
  const { t } = useTranslation()
  const { copiedText, copyToClipboard } = useCopyToClipboard({ notify: false })
  const [loading, setLoading] = useState(false)
  const [body, setBody] = useState('')
  const [error, setError] = useState('')

  useEffect(() => {
    if (!open || !requestId) return
    let cancelled = false
    setLoading(true)
    setError('')
    setBody('')
    const run = async () => {
      try {
        const res = await getRequestBodyLog(requestId)
        if (cancelled) return
        if (res.success && res.data) {
          setBody(res.data.body ?? '')
        } else {
          setError(res.message || t('Request body not recorded'))
        }
      } catch {
        if (!cancelled) setError(t('Failed to load request body'))
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void run()
    return () => {
      cancelled = true
    }
  }, [open, requestId, t])

  const formatted = formatBody(body)

  let content: React.ReactNode
  if (loading) {
    content = <p className='text-muted-foreground text-sm'>{t('Loading...')}</p>
  } else if (error) {
    content = <p className='text-muted-foreground text-sm'>{error}</p>
  } else {
    content = (
      <div className='bg-muted/50 relative rounded-md border p-3'>
        <Button
          variant='ghost'
          size='sm'
          className='absolute top-2 right-2 h-8 w-8 p-0'
          onClick={() => copyToClipboard(formatted)}
          title={t('Copy to clipboard')}
        >
          {copiedText === formatted ? (
            <Check className='size-4 text-green-600' />
          ) : (
            <Copy className='size-4' />
          )}
        </Button>
        <pre className='pr-10 font-mono text-xs leading-relaxed break-words whitespace-pre-wrap'>
          {formatted || '-'}
        </pre>
      </div>
    )
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Request Body')}
      description={t('The full request body recorded for this request')}
      contentClassName='sm:max-w-2xl'
      contentHeight='auto'
      bodyClassName='space-y-4'
    >
      <ScrollArea className='max-h-[560px] pr-4'>
        <div className='py-2'>{content}</div>
      </ScrollArea>
    </Dialog>
  )
}
