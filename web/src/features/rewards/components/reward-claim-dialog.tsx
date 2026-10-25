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
import { Check, Gift, Loader2, Sparkles, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { formatQuotaWithCurrency } from '@/lib/currency'
import dayjs from '@/lib/dayjs'

import { useRewardExpired } from '../hooks/use-reward-expiry'
import { useClaimReward } from '../hooks/use-rewards'
import { isRewardExpired } from '../lib/reward-expiry'
import type { UserReward } from '../types'

interface RewardClaimDialogProps {
  reward: UserReward
  onDismiss: () => void
  onComplete: () => void
}

export function RewardClaimDialog(props: RewardClaimDialogProps) {
  const { t } = useTranslation()
  const claim = useClaimReward()
  const result = claim.data?.reward
  const isRandom = props.reward.type === 'random'
  const deadlinePassed = useRewardExpired(props.reward)
  const serverExpired =
    claim.error instanceof Error && claim.error.message === 'reward expired'
  const expired = !result && (deadlinePassed || serverExpired)
  const close = result || expired ? props.onComplete : props.onDismiss
  let rewardMessage = t('Your reward is ready to claim')
  if (expired) {
    rewardMessage = t('This reward has expired and can no longer be claimed.')
  } else if (isRandom) {
    rewardMessage = t('Your lucky amount will be revealed when opened')
  }
  let claimLabel = isRandom ? t('Open red packet') : t('Claim reward')
  if (expired) claimLabel = t('Expired')
  let title = t('You received a reward')
  if (result) title = t('Reward claimed')
  else if (expired) title = t('Reward expired')
  else if (isRandom) title = t('You received a red packet')

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !claim.isPending) close()
      }}
    >
      <DialogContent
        showCloseButton={false}
        className='max-h-[90svh] gap-5 overflow-y-auto p-5 sm:max-w-lg sm:p-6'
      >
        <DialogHeader className='pr-8'>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>
            {t('A little appreciation, just for you.')}
          </DialogDescription>
        </DialogHeader>
        <Button
          type='button'
          variant='ghost'
          size='icon-sm'
          className='absolute top-3 right-3'
          aria-label={t('Close')}
          disabled={claim.isPending}
          onClick={close}
        >
          <X aria-hidden='true' />
        </Button>
        <div className='relative isolate overflow-hidden rounded-2xl border border-[#b36c52] bg-[#8f352d] p-3 text-center text-[#ffedcc]'>
          <div
            aria-hidden='true'
            className='pointer-events-none absolute inset-x-0 top-0 -z-10 h-40 bg-[#a24a3e] [clip-path:polygon(0_0,100%_0,50%_100%)]'
          />
          <div className='flex min-h-80 flex-col items-center justify-center rounded-xl border border-[#c98060]/50 px-5 py-8 sm:min-h-96'>
            <div className='mb-6 flex size-14 items-center justify-center rounded-2xl bg-[#fff8ed] text-[#8f352d]'>
              {result ? (
                <Check className='size-8' aria-hidden='true' />
              ) : (
                <Gift className='size-8' aria-hidden='true' />
              )}
            </div>
            <h2 className='max-w-full text-2xl font-semibold wrap-anywhere'>
              {props.reward.title}
            </h2>
            {props.reward.description && (
              <p className='mt-3 max-w-full text-sm wrap-anywhere whitespace-pre-wrap'>
                {props.reward.description}
              </p>
            )}
            <div className='mt-8 w-full' aria-live='polite'>
              {result ? (
                <>
                  <p className='text-4xl font-semibold wrap-anywhere tabular-nums'>
                    +{formatQuotaWithCurrency(result.quota_awarded)}
                  </p>
                  <p className='mt-3 text-sm'>
                    {t('Added to your account balance')}
                  </p>
                </>
              ) : (
                <>
                  {isRandom ? (
                    <Sparkles
                      className='mx-auto mb-3 size-6'
                      aria-hidden='true'
                    />
                  ) : (
                    <p className='mb-3 text-4xl font-semibold wrap-anywhere tabular-nums'>
                      {formatQuotaWithCurrency(props.reward.min_quota)}
                    </p>
                  )}
                  <p className='text-sm'>{rewardMessage}</p>
                  {!expired && (
                    <p className='mt-3 text-xs'>
                      {t('Credited directly to your balance after claiming')}
                    </p>
                  )}
                </>
              )}
            </div>
            <span className='mt-6 rounded-full border border-[#c98060] px-5 py-2 text-sm'>
              {isRandom ? t('Lucky red packet') : t('Reward credit')}
            </span>
            {props.reward.expires_at > 0 && (
              <p className='mt-4 text-xs text-[#efd2b2]'>
                {t('Expires at {{time}}', {
                  time: dayjs
                    .unix(props.reward.expires_at)
                    .format('YYYY-MM-DD HH:mm'),
                })}
              </p>
            )}
          </div>
        </div>
        {claim.isError && !expired && (
          <p role='alert' className='text-destructive text-sm'>
            {t(
              claim.error instanceof Error
                ? claim.error.message
                : 'Unable to claim reward. Please try again.'
            )}
          </p>
        )}
        {result ? (
          <Button
            type='button'
            className='h-11 w-full'
            onClick={props.onComplete}
          >
            {t('Done')}
          </Button>
        ) : (
          <div className='grid grid-cols-2 gap-3'>
            <Button
              type='button'
              variant='outline'
              className='h-11'
              disabled={claim.isPending}
              onClick={close}
            >
              {expired ? t('Close') : t('Claim later')}
            </Button>
            <Button
              type='button'
              className='h-11'
              disabled={claim.isPending || expired}
              onClick={() => {
                if (!isRewardExpired(props.reward)) {
                  claim.mutate({
                    id: props.reward.id,
                    user_id: props.reward.user_id,
                  })
                }
              }}
            >
              {claim.isPending && (
                <Loader2 className='size-4 animate-spin' aria-hidden='true' />
              )}
              {claimLabel}
            </Button>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
