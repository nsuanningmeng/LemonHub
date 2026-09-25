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
import { Megaphone, Pause, Play } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { AnnouncementDetailModal } from '@/features/dashboard/components/overview/announcement-detail-dialog'
import { useAnnouncements } from '@/features/dashboard/hooks/use-status-data'
import { getPreviewText } from '@/features/dashboard/lib/text'
import type { AnnouncementItem } from '@/features/dashboard/types'

export function AnnouncementBanner() {
  const { t } = useTranslation()
  const { items } = useAnnouncements()
  const [isPaused, setIsPaused] = useState(false)
  const [selectedAnnouncement, setSelectedAnnouncement] =
    useState<AnnouncementItem | null>(null)
  const announcements = useMemo(
    () =>
      items
        .filter(
          (item) => typeof item.content === 'string' && item.content.trim()
        )
        .map((item) => ({
          item,
          preview: getPreviewText(item.content.replaceAll(/\s+/g, ' '), 160),
          publishedAt: Date.parse(item.publishDate ?? '') || 0,
        }))
        .filter(({ preview }) => preview.length > 0)
        .sort((a, b) => b.publishedAt - a.publishedAt)
        .slice(0, 5),
    [items]
  )

  if (announcements.length === 0) return null

  const duration = Math.max(
    30,
    announcements.reduce((total, { preview }) => total + preview.length, 0) / 4
  )
  const pauseLabel = isPaused
    ? t('Resume announcements')
    : t('Pause announcements')

  return (
    <>
      <section
        aria-label={t('Latest announcements')}
        className='announcement-banner border-warning/25 bg-warning/10 text-foreground mx-3 mt-2 flex min-w-0 shrink-0 items-center gap-2 overflow-hidden rounded-lg border px-2 sm:mx-4 sm:gap-3 sm:px-3'
        data-paused={isPaused || selectedAnnouncement !== null}
      >
        <div className='text-foreground flex shrink-0 items-center gap-2 text-sm font-semibold'>
          <Megaphone aria-hidden='true' className='text-warning size-4' />
          <span className='hidden sm:inline'>{t('Latest announcements')}</span>
        </div>
        <div className='announcement-banner-viewport min-w-0 flex-1'>
          <div
            className='announcement-banner-track'
            style={{ animationDuration: `${duration}s` }}
          >
            {[false, true].map((isCopy) => (
              <div
                key={String(isCopy)}
                className='announcement-banner-items'
                aria-hidden={isCopy ? true : undefined}
                data-copy={isCopy}
              >
                {announcements.map(({ item, preview }) => (
                  <button
                    key={item.id ?? `${item.publishDate ?? ''}:${item.content}`}
                    type='button'
                    tabIndex={isCopy ? -1 : 0}
                    className='focus-visible:ring-ring flex min-h-10 shrink-0 items-center gap-2 rounded-sm px-1 text-left text-sm font-medium whitespace-nowrap underline-offset-4 hover:underline focus-visible:ring-2 focus-visible:outline-none focus-visible:ring-inset'
                    onClick={() => setSelectedAnnouncement(item)}
                    onPointerDown={
                      isCopy ? (event) => event.preventDefault() : undefined
                    }
                    title={t('Click for details')}
                  >
                    <span
                      aria-hidden='true'
                      className='bg-warning size-1.5 shrink-0 rounded-full'
                    />
                    {preview}
                  </button>
                ))}
              </div>
            ))}
          </div>
        </div>
        <Button
          type='button'
          variant='ghost'
          size='icon'
          className='size-9 shrink-0 motion-reduce:hidden'
          aria-label={pauseLabel}
          title={pauseLabel}
          aria-pressed={isPaused}
          onClick={() => setIsPaused((paused) => !paused)}
        >
          {isPaused ? (
            <Play aria-hidden='true' />
          ) : (
            <Pause aria-hidden='true' />
          )}
        </Button>
      </section>
      {selectedAnnouncement !== null && (
        <AnnouncementDetailModal
          open
          onOpenChange={(open) => {
            if (!open) setSelectedAnnouncement(null)
          }}
          announcement={selectedAnnouncement}
        />
      )}
    </>
  )
}
