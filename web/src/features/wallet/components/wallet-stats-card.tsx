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
import { ArrowUpRight } from 'lucide-react'
import { useId } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { formatQuota } from '@/lib/format'

import type { UserWalletData } from '../types'

interface WalletStatsCardProps {
  user: UserWalletData | null
  loading?: boolean
  error?: boolean
  onRetry?: () => void
  onAddFunds?: () => void
}

export function WalletStatsCard(props: WalletStatsCardProps) {
  const { t } = useTranslation()
  const titleId = useId()
  const unavailable = props.error && !props.user
  const balance = unavailable ? t('N/A') : formatQuota(props.user?.quota ?? 0)
  const stats = [
    {
      label: t('Total Usage'),
      value: unavailable ? t('N/A') : formatQuota(props.user?.used_quota ?? 0),
    },
    {
      label: t('API Requests'),
      value: unavailable
        ? t('N/A')
        : (props.user?.request_count ?? 0).toLocaleString(),
    },
  ]

  return (
    <VStack
      as='section'
      gap={6}
      aria-labelledby={titleId}
      aria-busy={Boolean(props.loading)}
      className='min-w-0'
    >
      <VStack gap={5} padding={6} className='surface-brand rounded-xl'>
        <h3 id={titleId} className='text-sm font-medium'>
          {t('Current Balance')}
        </h3>
        <VStack gap={2} className='text-inherit'>
          {props.loading ? (
            <Skeleton className='h-12 w-full max-w-60' />
          ) : (
            <p
              className='overflow-x-auto font-mono text-3xl font-medium tracking-tight whitespace-nowrap tabular-nums sm:text-4xl'
              title={balance}
            >
              {balance}
            </p>
          )}
          {props.error ? (
            <VStack gap={2} className='text-inherit' role='alert'>
              <p className='text-sm'>{t('Failed to load')}</p>
              {props.onRetry && (
                <Button
                  variant='outline'
                  onClick={props.onRetry}
                  className='bg-brand-paper text-brand-ink hover:bg-brand-paper/90 w-fit border-transparent'
                >
                  {t('Retry')}
                </Button>
              )}
            </VStack>
          ) : null}
        </VStack>
        {props.onAddFunds && (
          <Button
            onClick={props.onAddFunds}
            className='bg-brand-paper text-brand-ink hover:bg-brand-paper/90 w-full justify-between border border-transparent'
          >
            {t('Add Funds')}
            <ArrowUpRight className='size-4' aria-hidden='true' />
          </Button>
        )}
      </VStack>

      <dl className='border-tone-teal-border grid grid-cols-2 gap-5 border-t pt-5'>
        {stats.map((item) => (
          <VStack key={item.label} gap={2} className='min-w-0'>
            <HStack
              as='dt'
              gap={2}
              align='center'
              className='text-tone-teal-strong text-sm'
            >
              {item.label}
            </HStack>
            <dd>
              {props.loading ? (
                <Skeleton className='h-7 w-28' />
              ) : (
                <p
                  className='text-tone-teal-strong overflow-x-auto font-mono text-xl font-medium tracking-tight whitespace-nowrap tabular-nums'
                  title={item.value}
                >
                  {item.value}
                </p>
              )}
            </dd>
          </VStack>
        ))}
      </dl>
    </VStack>
  )
}
