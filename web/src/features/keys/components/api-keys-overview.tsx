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
import { CircleCheck, KeyRound, Layers3, PauseCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'

import { API_KEY_STATUS } from '../constants'
import { getApiKeyEffectiveStatus, type ApiKey } from '../types'

type ApiKeysOverviewProps = {
  keys: ApiKey[]
  total: number
  isLoading: boolean
  filtered: boolean
}

export function ApiKeysOverview(props: ApiKeysOverviewProps) {
  const { t } = useTranslation()
  const activeOnPage = props.keys.filter(
    (key) => getApiKeyEffectiveStatus(key) === API_KEY_STATUS.ENABLED
  ).length
  const inactiveOnPage = props.keys.length - activeOnPage

  return (
    <section
      aria-label={t('Access overview')}
      aria-busy={props.isLoading}
      className='grid shrink-0 gap-3 xl:grid-cols-12'
    >
      <VStack
        gap={3}
        padding={5}
        className='surface-brand hidden min-w-0 rounded-xl sm:flex xl:col-span-5'
      >
        <HStack gap={2} vAlign='center'>
          <KeyRound aria-hidden='true' className='size-4 shrink-0' />
          <p className='font-mono text-xs font-medium tracking-wide'>
            {t('Workspace credentials')}
          </p>
        </HStack>
        <VStack gap={1.5} className='min-w-0'>
          <h3 className='text-lg font-semibold tracking-tight'>
            {t('A connection for every app')}
          </h3>
          <p className='max-w-xl text-xs leading-relaxed opacity-85'>
            {t(
              'Create separate keys for your apps. Keep model access and spending limits under your control.'
            )}
          </p>
        </VStack>
      </VStack>
      <dl className='grid min-w-0 grid-cols-3 gap-3 xl:col-span-7'>
        <VStack
          gap={2}
          padding={3}
          className='surface-violet border-tone-violet-border min-w-0 rounded-xl border sm:p-5'
        >
          <Layers3 aria-hidden='true' className='mb-1 size-4' />
          <dt className='text-xs leading-relaxed'>
            {props.filtered ? t('Matching keys') : t('Total keys')}
          </dt>
          <dd className='mt-auto text-2xl font-semibold tracking-tight wrap-anywhere tabular-nums sm:text-3xl'>
            {props.isLoading ? (
              <Skeleton className='h-7 w-10' />
            ) : (
              props.total.toLocaleString()
            )}
          </dd>
        </VStack>
        <VStack
          gap={2}
          padding={3}
          className='surface-teal border-tone-teal-border min-w-0 rounded-xl border sm:p-5'
        >
          <CircleCheck aria-hidden='true' className='mb-1 size-4' />
          <dt className='text-xs leading-relaxed'>
            {t('Active on this page')}
          </dt>
          <dd className='mt-auto text-2xl font-semibold tracking-tight wrap-anywhere tabular-nums sm:text-3xl'>
            {props.isLoading ? (
              <Skeleton className='h-7 w-10' />
            ) : (
              activeOnPage.toLocaleString()
            )}
          </dd>
        </VStack>
        <VStack
          gap={2}
          padding={3}
          className='surface-peach border-tone-peach-border min-w-0 rounded-xl border sm:p-5'
        >
          <PauseCircle aria-hidden='true' className='mb-1 size-4' />
          <dt className='text-xs leading-relaxed'>
            {t('Inactive on this page')}
          </dt>
          <dd className='mt-auto text-2xl font-semibold tracking-tight wrap-anywhere tabular-nums sm:text-3xl'>
            {props.isLoading ? (
              <Skeleton className='h-7 w-10' />
            ) : (
              inactiveOnPage.toLocaleString()
            )}
          </dd>
        </VStack>
      </dl>
    </section>
  )
}
