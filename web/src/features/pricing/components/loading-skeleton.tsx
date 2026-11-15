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

import { Skeleton } from '@/components/ui/skeleton'

import { VIEW_MODES, type ViewMode } from '../constants'

export interface LoadingSkeletonProps {
  viewMode?: ViewMode
}

export function LoadingSkeleton(props: LoadingSkeletonProps) {
  const { t } = useTranslation()
  const viewMode = props.viewMode ?? VIEW_MODES.CARD

  return (
    <section
      role='status'
      aria-label={t('Loading models')}
      aria-busy='true'
      className='space-y-5'
    >
      <div className='surface-violet space-y-4 rounded-3xl p-8'>
        <Skeleton className='h-8 w-40' />
        <Skeleton className='h-4 w-52' />
      </div>
      <Skeleton className='h-10 w-full rounded-lg' />
      <FilterBarSkeleton />
      {viewMode === VIEW_MODES.TABLE ? (
        <TableContentSkeleton />
      ) : (
        <CardContentSkeleton />
      )}
    </section>
  )
}

function CardContentSkeleton() {
  return (
    <div className='grid grid-cols-1 gap-4 md:grid-cols-2 2xl:grid-cols-3'>
      {[1, 2, 3, 4, 5, 6, 7, 8, 9].map((slot) => (
        <div
          key={slot}
          className='surface-violet border-tone-violet-border rounded-2xl border p-5'
        >
          <div className='flex items-start justify-between gap-3'>
            <div className='flex min-w-0 flex-1 items-start gap-3'>
              <Skeleton className='size-10 shrink-0 rounded-xl' />
              <div className='min-w-0 flex-1 space-y-2'>
                <Skeleton className='h-5 w-full max-w-36' />
                <Skeleton className='h-3.5 w-full max-w-48' />
              </div>
            </div>
            <Skeleton className='h-8 w-16 shrink-0 rounded-md' />
          </div>
          <div className='mt-4 space-y-2'>
            <Skeleton className='h-3.5 w-full' />
            <Skeleton className='h-3.5 w-4/5' />
          </div>
          <div className='mt-4 flex items-center gap-2'>
            <Skeleton className='h-4 w-24' />
            <Skeleton className='h-4 w-16' />
          </div>
          <div className='mt-2 flex items-center gap-3'>
            <Skeleton className='h-3.5 w-14' />
            <Skeleton className='h-3.5 w-14' />
            <Skeleton className='h-3.5 w-8' />
          </div>
        </div>
      ))}
    </div>
  )
}

function FilterBarSkeleton() {
  return (
    <div className='space-y-3'>
      <div className='flex flex-wrap items-center gap-3'>
        <div className='flex min-w-0 flex-1 basis-full flex-wrap items-center gap-2 sm:basis-auto'>
          {['w-20', 'w-24', 'w-16', 'w-28', 'w-14'].map((width) => (
            <Skeleton key={width} className={`h-8 rounded-lg ${width}`} />
          ))}
        </div>
        <div className='flex min-w-0 flex-wrap items-center gap-2'>
          <Skeleton className='h-8 w-24 rounded-lg' />
          <Skeleton className='h-8 w-20 rounded-lg' />
          <Skeleton className='h-8 w-24' />
          <Skeleton className='h-8 w-20 rounded-lg' />
        </div>
      </div>
      <Skeleton className='h-5 w-24' />
    </div>
  )
}

function TableContentSkeleton() {
  const columns = [
    { id: 'model', width: 'w-48' },
    { id: 'input', width: 'w-24' },
    { id: 'output', width: 'w-24' },
    { id: 'cached', width: 'w-24' },
    { id: 'group', width: 'w-20' },
    { id: 'actions', width: 'w-24' },
  ]

  return (
    <div className='space-y-4'>
      <div className='overflow-hidden rounded-lg border'>
        <div className='bg-muted/30 border-b px-4 py-3'>
          <div className='flex items-center gap-4'>
            {columns.map((col) => (
              <Skeleton key={col.id} className={`h-4 ${col.width}`} />
            ))}
          </div>
        </div>
        {[1, 2, 3, 4, 5, 6, 7, 8, 9, 10].map((slot) => (
          <div
            key={slot}
            className='flex items-center gap-4 border-b px-4 py-3 last:border-b-0'
          >
            {columns.map((col) => (
              <Skeleton key={col.id} className={`h-5 ${col.width}`} />
            ))}
          </div>
        ))}
      </div>
      <div className='flex items-center justify-between'>
        <Skeleton className='h-5 w-32' />
        <div className='flex min-w-0 flex-wrap items-center gap-2'>
          {[1, 2, 3, 4].map((page) => (
            <Skeleton key={page} className='size-8' />
          ))}
        </div>
      </div>
    </div>
  )
}
