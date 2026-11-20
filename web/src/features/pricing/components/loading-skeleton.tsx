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
import { Skeleton } from '@/components/ui/skeleton'

import { VIEW_MODES, type ViewMode } from '../constants'

export interface LoadingSkeletonProps {
  viewMode?: ViewMode
}

export function LoadingSkeleton(props: LoadingSkeletonProps) {
  const viewMode = props.viewMode ?? VIEW_MODES.CARD

  return (
    <div aria-busy='true'>
      {viewMode === VIEW_MODES.TABLE ? (
        <TableContentSkeleton />
      ) : (
        <div className='divide-border bg-card divide-y border-y'>
          {['first', 'second', 'third', 'fourth'].map((key) => (
            <div
              key={key}
              className='grid min-h-24 items-center gap-4 p-4 sm:grid-cols-[2fr_1fr_1fr_1fr]'
            >
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
