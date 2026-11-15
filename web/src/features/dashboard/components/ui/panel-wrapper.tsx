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
import { useId, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

interface PanelWrapperProps {
  title: ReactNode
  description?: ReactNode
  loading?: boolean
  empty?: boolean
  emptyMessage?: string
  height?: string
  className?: string
  contentClassName?: string
  headerActions?: ReactNode
  headerTone?: 'blue' | 'teal' | 'violet' | 'peach'
  children?: ReactNode
}

const HEADER_TONES = {
  blue: 'text-tone-blue-strong',
  teal: 'text-tone-teal-strong',
  violet: 'text-tone-violet-strong',
  peach: 'text-tone-peach-strong',
}

export function PanelWrapper(props: PanelWrapperProps) {
  const { t } = useTranslation()
  const titleId = useId()
  const height = props.height ?? 'h-64'

  let content: ReactNode
  if (props.loading) {
    content = (
      <section
        className={cn('p-5', props.contentClassName)}
        role='status'
        aria-label={t('Loading')}
      >
        <Skeleton className={cn('w-full', height)} />
      </section>
    )
  } else if (props.empty) {
    content = (
      <VStack
        gap={2}
        align='center'
        justify='center'
        className={cn(
          'text-muted-foreground px-5 text-center text-sm',
          height,
          props.contentClassName
        )}
        role='status'
      >
        <p>{props.emptyMessage ?? t('No data available')}</p>
      </VStack>
    )
  } else {
    content = (
      <section className={cn('p-5', props.contentClassName)}>
        {props.children}
      </section>
    )
  }

  return (
    <section
      aria-labelledby={titleId}
      aria-busy={Boolean(props.loading)}
      className={cn('h-full min-w-0', props.className)}
    >
      <HStack
        gap={3}
        align='start'
        justify='between'
        wrap='wrap'
        className={cn(
          'px-3 pb-3 sm:px-5',
          props.headerTone && HEADER_TONES[props.headerTone]
        )}
      >
        <VStack gap={1} className='min-w-0 flex-1 text-inherit'>
          <h4 id={titleId} className='text-sm font-semibold break-words'>
            {props.title}
          </h4>
          {props.description != null && (
            <p className='text-muted-foreground text-xs leading-relaxed'>
              {props.description}
            </p>
          )}
        </VStack>
        {props.headerActions}
      </HStack>
      {content}
    </section>
  )
}
