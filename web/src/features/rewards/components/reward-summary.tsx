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

import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { formatQuotaWithCurrency } from '@/lib/currency'

import { useRewardSummary } from '../hooks/use-rewards'

export function RewardSummaryCards() {
  const { t } = useTranslation()
  const summary = useRewardSummary()
  if (summary.isError) {
    return (
      <div
        role='alert'
        className='bg-card flex flex-wrap items-center gap-3 rounded-lg border p-4'
      >
        <p className='text-sm'>{t('Unable to load rewards')}</p>
        <Button
          type='button'
          variant='outline'
          onClick={() => void summary.refetch()}
        >
          {t('Retry')}
        </Button>
      </div>
    )
  }
  return (
    <aside className='bg-brand-ink rounded-xl px-5 py-6 text-white sm:px-6'>
      <h2 className='text-sm font-medium text-white/80'>
        {t('Available reward credit')}
      </h2>
      {summary.isPending ? (
        <Skeleton className='my-3 h-11 w-28' />
      ) : (
        <p className='text-brand-citrus mt-2 mb-5 text-4xl font-semibold tracking-tight tabular-nums'>
          {formatQuotaWithCurrency(summary.data?.fixed_quota ?? 0)}
        </p>
      )}
      <div className='grid gap-3 border-t border-white/25 pt-4'>
        <section
          aria-labelledby='reward-credit-heading'
          className='flex min-w-0 items-baseline justify-between gap-4'
        >
          <h3 id='reward-credit-heading' className='text-sm'>
            {t('Rewards to claim')}
          </h3>
          {summary.isPending ? (
            <Skeleton className='h-5 w-8' />
          ) : (
            <span className='text-lg font-semibold tabular-nums'>
              {summary.data?.fixed_count ?? 0}
            </span>
          )}
        </section>
        <section
          aria-labelledby='reward-packets-heading'
          className='flex min-w-0 items-baseline justify-between gap-4'
        >
          <h3 id='reward-packets-heading' className='text-sm'>
            {t('My red packets')}
          </h3>
          {summary.isPending ? (
            <Skeleton className='h-5 w-8' />
          ) : (
            <span className='text-lg font-semibold tabular-nums'>
              {summary.data?.random_count ?? 0}
            </span>
          )}
        </section>
      </div>
    </aside>
  )
}
