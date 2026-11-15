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
import { HStack } from '@astryxdesign/core/HStack'
import { VStack } from '@astryxdesign/core/VStack'
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
    <VStack
      as='section'
      role='status'
      aria-label={t('Loading models')}
      aria-busy='true'
      gap={8}
    >
      <VStack gap={3}>
        <Skeleton className='h-9 w-full max-w-40' />
        <Skeleton className='h-4 w-full max-w-64' />
      </VStack>
      <Skeleton className='h-12 w-full max-w-2xl rounded-lg' />
      <HStack gap={6} wrap='wrap' aria-hidden>
        {[1, 2, 3].map((slot) => (
          <Skeleton key={slot} className='h-5 w-20' />
        ))}
      </HStack>
      {viewMode === VIEW_MODES.TABLE ? (
        <TableContentSkeleton />
      ) : (
        <CardContentSkeleton />
      )}
    </VStack>
  )
}

function CardContentSkeleton() {
  return (
    <ul className='grid min-w-0 grid-cols-1 gap-x-8 xl:grid-cols-2' aria-hidden>
      {[1, 2, 3, 4, 5, 6].map((slot) => (
        <VStack
          as='li'
          key={slot}
          gap={4}
          className='border-border/60 min-w-0 border-b py-6'
        >
          <HStack gap={3} vAlign='start'>
            <Skeleton className='size-10 shrink-0 rounded-lg' />
            <VStack gap={2} className='min-w-0 flex-1'>
              <Skeleton className='h-5 w-full max-w-48' />
              <Skeleton className='h-3.5 w-full max-w-24' />
            </VStack>
          </HStack>
          <Skeleton className='h-4 w-full max-w-80' />
          <HStack gap={4} wrap='wrap'>
            <Skeleton className='h-4 w-24' />
            <Skeleton className='h-4 w-24' />
          </HStack>
        </VStack>
      ))}
    </ul>
  )
}

function TableContentSkeleton() {
  return (
    <VStack gap={0} className='overflow-hidden' aria-hidden>
      {[1, 2, 3, 4, 5, 6].map((slot) => (
        <HStack key={slot} gap={4} className='border-border/60 border-b py-4'>
          <Skeleton className='h-5 min-w-0 flex-1' />
          <Skeleton className='h-5 w-16 shrink-0' />
          <Skeleton className='h-5 w-16 shrink-0' />
        </HStack>
      ))}
    </VStack>
  )
}
