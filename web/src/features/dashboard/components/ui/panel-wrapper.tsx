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
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

interface PanelWrapperProps {
  title: ReactNode
  description?: ReactNode
  loading?: boolean
  empty?: boolean
  emptyMessage?: string
  className?: string
  headerActions?: ReactNode
  children?: ReactNode
}

function PanelHeader(props: {
  title: ReactNode
  description?: ReactNode
  actions?: ReactNode
}) {
  const heading = (
    <div className='flex min-w-0 flex-col gap-1'>
      <h3 className='text-base font-semibold tracking-tight'>{props.title}</h3>
      {props.description != null && (
        <div className='text-muted-foreground text-xs'>{props.description}</div>
      )}
    </div>
  )

  return (
    <div className='border-b px-4 py-3 sm:px-5'>
      {props.actions != null ? (
        <div className='flex items-start justify-between gap-2'>
          {heading}
          {props.actions}
        </div>
      ) : (
        heading
      )}
    </div>
  )
}

export function PanelWrapper(props: PanelWrapperProps) {
  const { t } = useTranslation()
  const resolvedEmptyMessage = props.emptyMessage ?? t('No data available')
  const frameClassName = cn(
    'min-w-0 overflow-hidden rounded-lg border bg-card',
    props.className
  )

  let content: ReactNode
  if (props.loading) {
    content = (
      <div
        aria-busy='true'
        aria-label={t('Loading')}
        className='space-y-2 px-4 py-5 sm:px-5'
      >
        <Skeleton className='h-3 w-3/5' />
        <Skeleton className='h-3 w-2/5' />
      </div>
    )
  } else if (props.empty) {
    content = (
      <p className='text-muted-foreground px-4 py-5 text-sm sm:px-5'>
        {resolvedEmptyMessage}
      </p>
    )
  } else {
    content = (
      <ScrollArea className='[&>[data-slot=scroll-area-viewport]]:h-auto [&>[data-slot=scroll-area-viewport]]:max-h-80'>
        {props.children}
      </ScrollArea>
    )
  }

  return (
    <section className={frameClassName}>
      <PanelHeader
        title={props.title}
        description={props.description}
        actions={props.loading ? undefined : props.headerActions}
      />
      {content}
    </section>
  )
}
