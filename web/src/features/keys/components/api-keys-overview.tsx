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
import { Text } from '@astryxdesign/core/Text'
import { VStack } from '@astryxdesign/core/VStack'
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
    <VStack
      as='section'
      gap={4}
      aria-label={t('Access overview')}
      aria-busy={props.isLoading}
      className='min-w-0 shrink-0'
    >
      <Text color='secondary' className='hidden max-w-3xl text-sm sm:block'>
        {t(
          'Create separate keys for your apps. Keep model access and spending limits under your control.'
        )}
      </Text>
      <dl className='grid min-w-0 grid-cols-3 gap-4 sm:gap-8'>
        <VStack gap={1} className='min-w-0'>
          <dt className='text-muted-foreground text-xs leading-relaxed'>
            {props.filtered ? t('Matching keys') : t('Total keys')}
          </dt>
          <dd className='text-tone-violet-strong text-2xl font-semibold tracking-tight wrap-anywhere tabular-nums'>
            {props.isLoading ? (
              <Skeleton className='h-7 w-10' />
            ) : (
              props.total.toLocaleString()
            )}
          </dd>
        </VStack>
        <VStack gap={1} className='min-w-0'>
          <dt className='text-muted-foreground text-xs leading-relaxed'>
            {t('Active on this page')}
          </dt>
          <dd className='text-tone-teal-strong text-2xl font-semibold tracking-tight wrap-anywhere tabular-nums'>
            {props.isLoading ? (
              <Skeleton className='h-7 w-10' />
            ) : (
              activeOnPage.toLocaleString()
            )}
          </dd>
        </VStack>
        <VStack gap={1} className='min-w-0'>
          <dt className='text-muted-foreground text-xs leading-relaxed'>
            {t('Inactive on this page')}
          </dt>
          <dd className='text-tone-peach-strong text-2xl font-semibold tracking-tight wrap-anywhere tabular-nums'>
            {props.isLoading ? (
              <Skeleton className='h-7 w-10' />
            ) : (
              inactiveOnPage.toLocaleString()
            )}
          </dd>
        </VStack>
      </dl>
    </VStack>
  )
}
