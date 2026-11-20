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
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { formatCompactNumber, formatQuota } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { UserWalletData } from '../types'

interface WalletStatsCardProps {
  user: UserWalletData | null
  loading?: boolean
  compactRequests?: boolean
  className?: string
}

export function WalletStatsCard(props: WalletStatsCardProps) {
  const { t } = useTranslation()
  const user = props.user
  const requestCount = user?.request_count ?? 0
  const stats = [
    {
      label: t('Current Balance'),
      value: user && formatQuota(user.quota),
      description: t('Remaining quota'),
    },
    {
      label: t('Total Usage'),
      value: user && formatQuota(user.used_quota),
      description: t('Total consumed quota'),
    },
    {
      label: t('API Requests'),
      value:
        user &&
        (props.compactRequests
          ? formatCompactNumber(requestCount)
          : requestCount.toLocaleString()),
      description: t('Total requests made'),
    },
  ]

  return (
    <div
      className={cn(
        'bg-card @container/account min-w-0 overflow-hidden rounded-lg border',
        props.className
      )}
    >
      <dl
        aria-busy={Boolean(props.loading)}
        className='grid grid-cols-2 @xl/account:grid-cols-[1.15fr_1fr_1fr]'
      >
        {stats.map((item, index) => (
          <div
            key={item.label}
            className={cn(
              'min-w-0 px-4 py-3 sm:px-5 sm:py-4',
              index === 0 &&
                'bg-muted/50 col-span-2 border-b @xl/account:col-span-1 @xl/account:border-r @xl/account:border-b-0',
              index === 2 && 'border-l'
            )}
          >
            <dt className='text-muted-foreground text-xs leading-4 font-medium sm:truncate'>
              {item.label}
            </dt>
            <dd className='mt-2'>
              {props.loading ? (
                <Skeleton className='h-8 w-24 max-w-full' />
              ) : (
                <span
                  className={cn(
                    'block max-w-full overflow-x-auto font-semibold tracking-tight whitespace-nowrap tabular-nums',
                    index === 0 ? 'text-3xl leading-9' : 'text-2xl leading-9'
                  )}
                  title={item.value ?? undefined}
                >
                  {item.value ?? '—'}
                </span>
              )}
              <span className='text-muted-foreground mt-1 hidden text-xs @xl/account:block'>
                {item.description}
              </span>
            </dd>
          </div>
        ))}
      </dl>
    </div>
  )
}
