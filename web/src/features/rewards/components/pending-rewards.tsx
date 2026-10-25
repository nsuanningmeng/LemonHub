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
import { Gift } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'

import { useRewards } from '../hooks/use-rewards'
import type { RewardType } from '../types'
import { PendingRewardCard } from './pending-reward-card'
import { RewardPagination } from './reward-pagination'

export function PendingRewards(props: { type: RewardType }) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const rewards = useRewards({
    status: 'pending',
    type: props.type,
    p: page,
    page_size: 12,
  })
  const isRandom = props.type === 'random'
  const lastPage = Math.max(1, Math.ceil((rewards.data?.total ?? 0) / 12))
  const pageOutOfRange = rewards.isSuccess && page > lastPage

  useEffect(() => {
    if (pageOutOfRange && !rewards.isFetching) setPage(lastPage)
  }, [lastPage, pageOutOfRange, rewards.isFetching])

  if (rewards.isPending || pageOutOfRange) {
    return (
      <div
        role='status'
        aria-label={t('Loading rewards')}
        className='grid gap-4 sm:grid-cols-2 lg:grid-cols-3'
      >
        <Skeleton className='h-56 rounded-2xl' />
        <Skeleton className='h-56 rounded-2xl' />
      </div>
    )
  }
  if (rewards.isError) {
    return (
      <div role='alert' className='space-y-3 rounded-3xl border p-6'>
        <p className='text-sm'>{t('Unable to load rewards')}</p>
        <Button
          type='button'
          variant='outline'
          onClick={() => void rewards.refetch()}
        >
          {t('Retry')}
        </Button>
      </div>
    )
  }
  return (
    <div>
      {rewards.data.items.length === 0 ? (
        <div className='text-muted-foreground flex min-h-52 flex-col items-center justify-center gap-3 rounded-3xl border border-dashed p-6 text-center'>
          <Gift className='size-8' aria-hidden='true' />
          <p>
            {isRandom
              ? t('No red packets waiting to be opened')
              : t('No reward credits waiting to be claimed')}
          </p>
          <p className='text-sm'>
            {t('New rewards will appear here when they arrive')}
          </p>
        </div>
      ) : (
        <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-3'>
          {rewards.data.items.map((reward) => (
            <PendingRewardCard key={reward.id} reward={reward} />
          ))}
        </div>
      )}
      <RewardPagination
        page={page}
        pageSize={12}
        total={rewards.data.total}
        loading={rewards.isFetching}
        onChange={setPage}
      />
    </div>
  )
}
