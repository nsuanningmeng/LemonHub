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
import {
  ArrowRight,
  ArrowUpRight,
  Braces,
  KeyRound,
  Layers3,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { useStatus } from '@/hooks/use-status'
import { useSystemConfig } from '@/hooks/use-system-config'
import { getModuleAccessFromStatus } from '@/lib/nav-modules'

type AuthLayoutProps = {
  children: React.ReactNode
}

export function AuthLayout(props: AuthLayoutProps) {
  const { t } = useTranslation()
  const { systemName, logo, loading } = useSystemConfig()
  const { status } = useStatus()
  const pricingAccess = getModuleAccessFromStatus(status, 'pricing')

  return (
    <section className='bg-background relative min-h-svh min-w-0'>
      <header className='flex min-w-0 items-center justify-between gap-6 px-5 py-5 sm:px-8 lg:px-12'>
        <Link
          to='/'
          className='flex min-w-0 items-center gap-3 transition-opacity hover:opacity-80'
        >
          {loading ? (
            <Skeleton className='size-9 shrink-0 rounded-xl' />
          ) : (
            <img
              src={logo}
              alt={t('Logo')}
              className='size-9 shrink-0 rounded-xl object-cover'
            />
          )}
          {loading ? (
            <Skeleton className='h-6 w-24' />
          ) : (
            <h1
              className='min-w-0 truncate text-xl font-semibold tracking-tight'
              title={systemName}
            >
              {systemName}
            </h1>
          )}
        </Link>
        {pricingAccess.enabled && !pricingAccess.requireAuth && (
          <Link
            to='/pricing'
            className='surface-violet hidden shrink-0 items-center gap-2 rounded-full px-4 py-2 text-sm font-medium transition-opacity hover:opacity-80 sm:inline-flex'
          >
            {t('Explore models')}
            <ArrowUpRight className='size-4' aria-hidden />
          </Link>
        )}
      </header>

      <main className='grid min-w-0 gap-6 px-5 pb-6 sm:px-8 lg:min-h-[calc(100svh-5rem)] lg:grid-cols-[1.05fr_1fr] lg:gap-8 lg:pb-8'>
        <aside
          aria-label={t('Your AI workspace')}
          className='surface-brand relative hidden min-w-0 flex-col justify-between gap-10 overflow-hidden rounded-3xl p-8 lg:flex lg:p-10 xl:p-12'
        >
          <VStack gap={8}>
            <HStack gap={3} vAlign='center'>
              <HStack
                hAlign='center'
                vAlign='center'
                className='border-brand-paper/30 size-10 rounded-2xl border'
              >
                <Braces className='text-brand-paper size-5' aria-hidden />
              </HStack>
              <p className='text-brand-paper/80 font-mono text-xs tracking-widest uppercase'>
                {t('Built for what comes next')}
              </p>
            </HStack>
            <VStack gap={6}>
              <h2 className='text-brand-paper max-w-lg text-5xl leading-tight font-semibold tracking-tight xl:text-6xl'>
                {t('Your ideas.')}
                <br />
                <em className='text-brand-paper font-normal not-italic'>
                  {t('Connected.')}
                </em>
              </h2>
              <p className='text-brand-paper/80 max-w-md text-base leading-relaxed'>
                {t(
                  'One workspace for the models, keys, and insights behind your next project.'
                )}
              </p>
            </VStack>
          </VStack>

          <VStack gap={4}>
            <HStack
              gap={4}
              vAlign='center'
              className='surface-teal rounded-2xl p-5'
            >
              <HStack
                hAlign='center'
                vAlign='center'
                className='border-tone-teal-border size-11 shrink-0 rounded-xl border'
              >
                <KeyRound
                  className='text-tone-teal-strong size-5'
                  aria-hidden
                />
              </HStack>
              <VStack gap={1} className='min-w-0'>
                <p className='text-tone-teal-strong text-sm font-semibold'>
                  {t('One key. More possibilities.')}
                </p>
                <p className='text-tone-teal-strong text-xs leading-relaxed'>
                  {t('Connect your tools through a unified API.')}
                </p>
              </VStack>
              <ArrowUpRight
                className='text-tone-teal-strong size-5 shrink-0'
                aria-hidden
              />
            </HStack>
            <HStack
              gap={4}
              vAlign='center'
              className='surface-violet rounded-2xl p-5 lg:ml-8'
            >
              <HStack
                hAlign='center'
                vAlign='center'
                className='border-tone-violet-border size-11 shrink-0 rounded-xl border'
              >
                <Layers3
                  className='text-tone-violet-strong size-5'
                  aria-hidden
                />
              </HStack>
              <VStack gap={1} className='min-w-0'>
                <p className='text-tone-violet-strong text-sm font-semibold'>
                  {t('A clearer view of every request.')}
                </p>
                <p className='text-tone-violet-strong text-xs leading-relaxed'>
                  {t('Keep models, usage, and spending in focus.')}
                </p>
              </VStack>
            </HStack>
          </VStack>
          <HStack
            gap={4}
            vAlign='center'
            hAlign='between'
            className='border-brand-paper/20 border-t pt-6'
          >
            <p className='text-brand-paper/80 max-w-xs text-sm leading-relaxed'>
              {t('From first call to your next big idea.')}
            </p>
            <HStack
              hAlign='center'
              vAlign='center'
              className='surface-peach size-10 shrink-0 rounded-full'
            >
              <ArrowRight className='size-5' aria-hidden />
            </HStack>
          </HStack>
        </aside>

        <VStack
          gap={8}
          vAlign='center'
          className='min-w-0 py-4 sm:py-8 lg:px-6'
        >
          <section
            aria-label={t('Authentication')}
            className='border-tone-blue-border bg-card mx-auto w-full max-w-md min-w-0 rounded-3xl border p-6 shadow-sm sm:p-8'
          >
            <HStack
              gap={3}
              vAlign='center'
              className='surface-blue mb-8 rounded-xl px-4 py-3'
            >
              <KeyRound
                className='text-tone-blue-strong size-4 shrink-0'
                aria-hidden
              />
              <p className='text-tone-blue-strong text-xs font-medium'>
                {t('Your workspace starts here')}
              </p>
            </HStack>
            {props.children}
          </section>
        </VStack>
      </main>
    </section>
  )
}
