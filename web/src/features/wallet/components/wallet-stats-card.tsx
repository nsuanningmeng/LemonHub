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
        '@container/account min-w-0 overflow-hidden rounded-lg',
        props.compactRequests ? 'bg-card border' : 'bg-brand-ink text-white',
        props.className
      )}
    >
      <dl
        aria-busy={Boolean(props.loading)}
        className={cn(
          'grid grid-cols-2',
          props.compactRequests && '@xl/account:grid-cols-[1.15fr_1fr_1fr]'
        )}
      >
        {stats.map((item, index) => (
          <div
            key={item.label}
            className={cn(
              'min-w-0 px-4 py-3 sm:px-5 sm:py-4',
              index === 0 && 'col-span-2 border-b',
              index === 0 &&
                props.compactRequests &&
                'bg-muted/50 @xl/account:col-span-1 @xl/account:border-r @xl/account:border-b-0',
              !props.compactRequests && 'border-white/20',
              index === 2 && 'border-l'
            )}
          >
            <dt
              className={cn(
                'text-xs leading-4 font-medium sm:truncate',
                props.compactRequests
                  ? 'text-muted-foreground'
                  : 'text-white/80'
              )}
            >
              {item.label}
            </dt>
            <dd className='mt-2'>
              {props.loading ? (
                <Skeleton className='h-8 w-24 max-w-full' />
              ) : (
                <span
                  className={cn(
                    'block max-w-full overflow-x-auto font-semibold tracking-tight whitespace-nowrap tabular-nums',
                    index === 0 && !props.compactRequests
                      ? 'text-brand-citrus text-4xl leading-tight sm:text-5xl'
                      : 'text-2xl leading-9'
                  )}
                  title={item.value ?? undefined}
                >
                  {item.value ?? '—'}
                </span>
              )}
              <span
                className={cn(
                  'mt-1 hidden text-xs',
                  props.compactRequests &&
                    'text-muted-foreground @xl/account:block'
                )}
              >
                {item.description}
              </span>
            </dd>
          </div>
        ))}
      </dl>
    </div>
  )
}
