import { ImagePlus, X } from 'lucide-react'
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
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Markdown } from '@/components/ui/markdown'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'

import { uploadTicketAttachment } from '../api'
import { MAX_ATTACHMENT_BYTES } from '../constants'
import type { UploadedAttachment } from '../types'

interface MessageEditorProps {
  content: string
  onContentChange: (value: string) => void
  attachments: UploadedAttachment[]
  onAttachmentsChange: (next: UploadedAttachment[]) => void
  placeholder?: string
  disabled?: boolean
  textareaClassName?: string
}

/**
 * Shared composer used by both the new-ticket and reply flows: a markdown
 * textarea with a Write/Preview toggle and an images-only attachment uploader.
 */
export function MessageEditor(props: MessageEditorProps) {
  const { t } = useTranslation()
  const [preview, setPreview] = useState(false)
  const [uploading, setUploading] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  const handleFiles = async (fileList: FileList | null) => {
    if (!fileList || fileList.length === 0) return

    const valid: File[] = []
    for (const file of fileList) {
      if (!file.type.startsWith('image/')) {
        toast.error(t('Only image files are allowed'))
        continue
      }
      if (file.size > MAX_ATTACHMENT_BYTES) {
        toast.error(t('Image must be smaller than 5MB'))
        continue
      }
      valid.push(file)
    }

    if (valid.length === 0) {
      if (inputRef.current) inputRef.current.value = ''
      return
    }

    setUploading(true)
    try {
      const uploaded: UploadedAttachment[] = []
      for (const file of valid) {
        const res = await uploadTicketAttachment(file)
        if (res.success && res.data) {
          uploaded.push(res.data)
        } else {
          toast.error(res.message || t('Failed to upload image'))
        }
      }
      if (uploaded.length > 0) {
        props.onAttachmentsChange([...props.attachments, ...uploaded])
      }
    } finally {
      setUploading(false)
      if (inputRef.current) inputRef.current.value = ''
    }
  }

  const removeAttachment = (id: number) => {
    props.onAttachmentsChange(props.attachments.filter((att) => att.id !== id))
  }

  return (
    <div className='space-y-2'>
      <div className='flex items-center justify-between gap-2'>
        <div className='flex gap-1'>
          <Button
            type='button'
            variant={preview ? 'ghost' : 'secondary'}
            size='xs'
            onClick={() => setPreview(false)}
          >
            {t('Write')}
          </Button>
          <Button
            type='button'
            variant={preview ? 'secondary' : 'ghost'}
            size='xs'
            onClick={() => setPreview(true)}
          >
            {t('Preview')}
          </Button>
        </div>
        <input
          ref={inputRef}
          type='file'
          accept='image/*'
          multiple
          className='hidden'
          onChange={(event) => handleFiles(event.target.files)}
          disabled={props.disabled || uploading}
        />
        <Button
          type='button'
          variant='outline'
          size='xs'
          onClick={() => inputRef.current?.click()}
          disabled={props.disabled || uploading}
        >
          {uploading ? (
            <Spinner className='size-3.5' />
          ) : (
            <ImagePlus className='size-3.5' />
          )}
          {t('Add image')}
        </Button>
      </div>

      {preview ? (
        <div className='min-h-24 rounded-lg border px-3 py-2'>
          {props.content.trim() ? (
            <Markdown>{props.content}</Markdown>
          ) : (
            <span className='text-muted-foreground text-sm'>
              {t('Nothing to preview')}
            </span>
          )}
        </div>
      ) : (
        <Textarea
          value={props.content}
          onChange={(event) => props.onContentChange(event.target.value)}
          placeholder={props.placeholder}
          disabled={props.disabled}
          className={cn(
            'min-h-24 focus-visible:ring-1 focus-visible:ring-ring/40',
            props.textareaClassName
          )}
        />
      )}

      {props.attachments.length > 0 && (
        <div className='flex flex-wrap gap-2'>
          {props.attachments.map((att) => (
            <div key={att.id} className='group/att relative'>
              <img
                src={att.url || `/api/ticket/attachment/${att.id}`}
                alt={att.file_name}
                className='size-16 rounded-md border object-cover'
              />
              <button
                type='button'
                onClick={() => removeAttachment(att.id)}
                aria-label={t('Remove image')}
                className='bg-background/80 hover:bg-background absolute -top-1.5 -right-1.5 rounded-full border p-0.5 opacity-0 transition-opacity group-hover/att:opacity-100'
              >
                <X className='size-3' />
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
