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
import { Link } from '@tanstack/react-router'
import { ArrowUpLeft, RouteOff } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { useSystemConfig } from '@/hooks/use-system-config'
import { cn } from '@/lib/utils'

type ErrorPageProps = {
  code?: string | number
  title: string
  description: ReactNode
  children?: ReactNode
  className?: string
  minimal?: boolean
}

export function ErrorPage(props: ErrorPageProps) {
  const { t } = useTranslation()
  const { systemName } = useSystemConfig()

  return (
    <section
      className={cn(
        'bg-background flex min-h-svh min-w-0 flex-col px-5 py-6 sm:px-10',
        props.className
      )}
    >
      {!props.minimal && (
        <header className='pb-6'>
          <Link
            to='/'
            className='text-foreground inline-flex min-w-0 items-center gap-3 text-lg font-semibold tracking-tight'
          >
            <ArrowUpLeft
              className='text-tone-blue-strong size-5 shrink-0'
              aria-hidden
            />
            <strong className='[overflow-wrap:anywhere]'>{systemName}</strong>
          </Link>
        </header>
      )}
      <main
        className={cn(
          'mx-auto grid w-full max-w-5xl flex-1 content-center gap-8 py-12',
          !props.minimal && 'md:grid-cols-[1fr_2fr] md:gap-6'
        )}
      >
        {!props.minimal && (
          <VStack
            gap={4}
            className='surface-peach border-tone-peach-border justify-between rounded-3xl border p-6 sm:p-8'
          >
            <p className='text-tone-peach-strong font-mono text-xs tracking-widest uppercase'>
              {t('Connection notice')}
            </p>
            <p className='text-tone-peach-strong text-7xl leading-none font-medium tracking-tighter sm:text-8xl'>
              {props.code ?? '—'}
            </p>
            <RouteOff className='text-tone-peach-strong size-10' aria-hidden />
          </VStack>
        )}
        <VStack
          gap={6}
          vAlign='center'
          className={cn(
            'min-w-0 rounded-3xl p-6 sm:p-10',
            !props.minimal && 'surface-blue border-tone-blue-border border'
          )}
        >
          <h1 className='text-tone-blue-strong text-3xl leading-tight font-semibold tracking-tight sm:text-4xl'>
            {props.title}
          </h1>
          <section className='text-muted-foreground max-w-lg text-sm leading-relaxed sm:text-base'>
            {props.description}
          </section>
          {props.children && (
            <HStack gap={3} wrap='wrap'>
              {props.children}
            </HStack>
          )}
        </VStack>
      </main>
    </section>
  )
}
