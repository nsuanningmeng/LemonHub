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
    <div role='status' aria-label={t('Loading models')} aria-busy='true'>
      {viewMode === VIEW_MODES.TABLE ? (
        <TableContentSkeleton />
      ) : (
        <div className='grid grid-cols-1 gap-3 md:grid-cols-2'>
          {['first', 'second', 'third', 'fourth'].map((key) => (
            <div key={key} className='bg-card space-y-4 rounded-lg border p-5'>
              <div className='flex items-center gap-3'>
                <Skeleton className='size-9 rounded-md' />
                <Skeleton className='h-5 w-2/5' />
              </div>
              <Skeleton className='h-4 w-3/5' />
              <Skeleton className='h-6 w-4/5' />
              <Skeleton className='h-4 w-1/3' />
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function TableContentSkeleton() {
  return (
    <div className='bg-card overflow-hidden rounded-lg border'>
      <div className='bg-muted h-9 border-b' />
      {['first', 'second', 'third', 'fourth', 'fifth', 'sixth'].map((row) => (
        <div
          key={row}
          className='flex h-12 items-center justify-between gap-4 border-b px-3 last:border-0'
        >
          <Skeleton className='h-4 w-1/3' />
          <Skeleton className='h-4 w-1/5' />
          <Skeleton className='h-4 w-1/6' />
        </div>
      ))}
    </div>
  )
}
