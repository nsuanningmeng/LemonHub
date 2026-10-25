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
import { Gift, Sparkles } from 'lucide-react'
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
        className='flex items-center gap-3 rounded-2xl border p-5'
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
    <div className='grid gap-5 md:grid-cols-2'>
      <section
        aria-labelledby='reward-credit-heading'
        className='bg-card flex min-h-64 flex-col rounded-3xl border p-6 sm:p-8'
      >
        <div className='flex justify-between gap-4'>
          <div>
            <h2 id='reward-credit-heading' className='text-lg font-semibold'>
              {t('Rewards to claim')}
            </h2>
            <p className='text-muted-foreground mt-1 text-sm'>
              {t('A little appreciation, just for you.')}
            </p>
          </div>
          <div className='bg-muted flex size-10 shrink-0 items-center justify-center rounded-full'>
            <Gift className='size-5' aria-hidden='true' />
          </div>
        </div>
        <div className='my-8 flex items-baseline gap-3'>
          {summary.isPending ? (
            <Skeleton className='h-12 w-16' />
          ) : (
            <span className='text-5xl font-semibold tabular-nums'>
              {summary.data?.fixed_count ?? 0}
            </span>
          )}
          <span className='text-muted-foreground text-sm'>
            {t('ready to claim')}
          </span>
        </div>
        <div className='text-muted-foreground mt-auto border-t pt-5 text-sm'>
          {t('Available reward credit')}:{' '}
          {summary.isPending
            ? '—'
            : formatQuotaWithCurrency(summary.data?.fixed_quota ?? 0)}
        </div>
      </section>
      <section
        aria-labelledby='reward-packets-heading'
        className='bg-card flex min-h-64 flex-col rounded-3xl border p-6 sm:p-8'
      >
        <div className='flex justify-between gap-4'>
          <div>
            <h2 id='reward-packets-heading' className='text-lg font-semibold'>
              {t('My red packets')}
            </h2>
            <p className='text-muted-foreground mt-1 text-sm'>
              {t('Open a little surprise, straight to your balance')}
            </p>
          </div>
          <div className='bg-muted flex size-10 shrink-0 items-center justify-center rounded-full'>
            <Sparkles className='size-5' aria-hidden='true' />
          </div>
        </div>
        <div className='my-8 flex items-baseline gap-3'>
          {summary.isPending ? (
            <Skeleton className='h-12 w-16' />
          ) : (
            <span className='text-5xl font-semibold tabular-nums'>
              {summary.data?.random_count ?? 0}
            </span>
          )}
          <span className='text-muted-foreground text-sm'>
            {t('ready to claim')}
          </span>
        </div>
        <p className='text-muted-foreground mt-auto border-t pt-5 text-sm'>
          {t('Your lucky amount will be revealed when opened')}
        </p>
      </section>
    </div>
  )
}
