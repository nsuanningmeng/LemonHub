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
import { Heading } from '@astryxdesign/core/Heading'
import { HStack } from '@astryxdesign/core/HStack'
import { Text } from '@astryxdesign/core/Text'
import { VStack } from '@astryxdesign/core/VStack'
import { Link } from '@tanstack/react-router'
import { ArrowUpRight, KeyRound, Layers3 } from 'lucide-react'
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
            className='text-tone-blue-strong hidden shrink-0 items-center gap-2 py-2 text-sm font-medium underline-offset-4 hover:underline sm:inline-flex'
          >
            {t('Explore models')}
            <ArrowUpRight className='size-4' aria-hidden />
          </Link>
        )}
      </header>

      <main className='grid min-w-0 gap-6 px-5 pb-6 sm:px-8 lg:min-h-[calc(100svh-5rem)] lg:grid-cols-[1.05fr_1fr] lg:gap-8 lg:pb-8'>
        <aside
          aria-label={t('Your AI workspace')}
          className='surface-brand relative hidden min-w-0 flex-col justify-between gap-10 overflow-hidden rounded-2xl p-8 lg:flex lg:p-10 xl:p-12'
        >
          <VStack gap={6}>
            <Heading
              level={2}
              color='inherit'
              className='text-brand-paper max-w-lg text-5xl leading-tight tracking-tight xl:text-6xl'
            >
              {t('Your ideas.')}
              <Text
                type='inherit'
                color='inherit'
                display='block'
                className='font-normal'
              >
                {t('Connected.')}
              </Text>
            </Heading>
            <Text
              as='p'
              color='inherit'
              className='text-brand-paper/80 max-w-md leading-relaxed'
            >
              {t(
                'One workspace for the models, keys, and insights behind your next project.'
              )}
            </Text>
          </VStack>
          <VStack gap={8}>
            <HStack gap={4} align='start'>
              <KeyRound
                className='text-brand-paper size-5 shrink-0'
                aria-hidden
              />
              <VStack gap={2} className='min-w-0'>
                <Text type='label' color='inherit' className='text-brand-paper'>
                  {t('One key. More possibilities.')}
                </Text>
                <Text
                  as='p'
                  type='supporting'
                  color='inherit'
                  className='text-brand-paper/80 leading-relaxed'
                >
                  {t('Connect your tools through a unified API.')}
                </Text>
              </VStack>
            </HStack>
            <HStack gap={4} align='start'>
              <Layers3
                className='text-brand-paper size-5 shrink-0'
                aria-hidden
              />
              <VStack gap={2} className='min-w-0'>
                <Text type='label' color='inherit' className='text-brand-paper'>
                  {t('A clearer view of every request.')}
                </Text>
                <Text
                  as='p'
                  type='supporting'
                  color='inherit'
                  className='text-brand-paper/80 leading-relaxed'
                >
                  {t('Keep models, usage, and spending in focus.')}
                </Text>
              </VStack>
            </HStack>
          </VStack>
        </aside>

        <VStack
          gap={8}
          vAlign='center'
          className='min-w-0 py-4 sm:py-8 lg:px-6'
        >
          <section
            aria-label={t('Authentication')}
            className='mx-auto w-full max-w-md min-w-0 py-6 sm:px-4 sm:py-8'
          >
            {props.children}
          </section>
        </VStack>
      </main>
    </section>
  )
}
