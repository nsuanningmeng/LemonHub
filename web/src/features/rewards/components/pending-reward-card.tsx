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
import { formatQuotaWithCurrency } from '@/lib/currency'
import dayjs from '@/lib/dayjs'

import { useRewardExpired } from '../hooks/use-reward-expiry'
import { useRewardDialogStore } from '../lib/reward-dialog-store'
import { isRewardExpired } from '../lib/reward-expiry'
import type { UserReward } from '../types'

export function PendingRewardCard(props: { reward: UserReward }) {
  const { t } = useTranslation()
  const openReward = useRewardDialogStore((state) => state.openReward)
  const isRandom = props.reward.type === 'random'
  const expired = useRewardExpired(props.reward)
  let claimLabel = isRandom ? t('Open red packet') : t('Claim reward')
  if (expired) claimLabel = t('Expired')

  return (
    <article className='bg-card flex min-w-0 flex-col rounded-2xl border p-5'>
      <div className='mb-4 flex items-center justify-between gap-3'>
        <span className='bg-muted rounded-lg p-2'>
          {isRandom ? (
            <Sparkles className='size-5' aria-hidden='true' />
          ) : (
            <Gift className='size-5' aria-hidden='true' />
          )}
        </span>
        <span className='text-muted-foreground text-xs'>
          {dayjs.unix(props.reward.created_at).format('YYYY-MM-DD')}
        </span>
      </div>
      <h3 className='font-semibold wrap-anywhere'>{props.reward.title}</h3>
      {props.reward.description && (
        <p className='text-muted-foreground mt-2 line-clamp-3 text-sm wrap-anywhere whitespace-pre-wrap'>
          {props.reward.description}
        </p>
      )}
      <p className='my-5 text-xl font-semibold wrap-anywhere'>
        {isRandom
          ? t('A surprise awaits')
          : formatQuotaWithCurrency(props.reward.min_quota)}
      </p>
      <Button
        type='button'
        className='mt-auto h-10 w-full'
        disabled={expired}
        onClick={() => {
          if (!isRewardExpired(props.reward)) openReward(props.reward)
        }}
      >
        {claimLabel}
      </Button>
      {props.reward.expires_at > 0 && (
        <p className='text-muted-foreground mt-3 text-xs'>
          {t('Expires at {{time}}', {
            time: dayjs
              .unix(props.reward.expires_at)
              .format('YYYY-MM-DD HH:mm'),
          })}
        </p>
      )}
    </article>
  )
}
