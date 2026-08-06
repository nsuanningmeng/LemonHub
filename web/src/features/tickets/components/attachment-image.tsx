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

import { api } from '@/lib/api'
import { cn } from '@/lib/utils'

/**
 * Ticket attachments are served by an authenticated API endpoint, so a native
 * <img src> request fails: the browser sends cookies but not the auth headers
 * the dashboard API requires, and the endpoint answers auth/permission
 * failures as HTTP 200 JSON, which renders as a broken image. Fetch the bytes
 * through the shared axios instance instead and render them via an object URL.
 */
export function AttachmentImage({
  attachmentId,
  fileName,
  className,
  openOnClick = false,
}: {
  attachmentId: number
  fileName: string
  className?: string
  openOnClick?: boolean
}) {
  const { t } = useTranslation()
  const [objectUrl, setObjectUrl] = useState<string>()
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let cancelled = false
    let url: string | undefined
    setObjectUrl(undefined)
    setFailed(false)
    api
      .get(`/api/ticket/attachment/${attachmentId}`, {
        responseType: 'blob',
        skipErrorHandler: true,
        skipBusinessError: true,
        disableDuplicate: true,
      })
      .then((res) => {
        const blob = res.data as Blob
        // Auth/permission failures arrive as 200 JSON bodies; only an image
        // content type is a successful fetch.
        if (!blob.type.startsWith('image/')) throw new Error('not an image')
        url = URL.createObjectURL(blob)
        if (cancelled) {
          URL.revokeObjectURL(url)
          return
        }
        setObjectUrl(url)
      })
      .catch(() => {
        if (!cancelled) setFailed(true)
      })
    return () => {
      cancelled = true
      if (url) URL.revokeObjectURL(url)
    }
  }, [attachmentId])

  if (failed) {
    return (
      <div
        className={cn(
          'bg-muted/40 text-muted-foreground flex h-24 w-32 items-center justify-center rounded-md border p-2 text-center text-xs break-all',
          className
        )}
        title={fileName}
      >
        {t('Failed to load')}
      </div>
    )
  }

  if (!objectUrl) {
    return (
      <div
        className={cn(
          'bg-muted/40 h-24 w-32 animate-pulse rounded-md border',
          className
        )}
        aria-label={fileName}
      />
    )
  }

  const img = (
    <img
      src={objectUrl}
      alt={fileName}
      className={cn('rounded-md border object-cover', className)}
    />
  )

  if (!openOnClick) return img

  return (
    <a href={objectUrl} target='_blank' rel='noopener noreferrer'>
      {img}
    </a>
  )
}
