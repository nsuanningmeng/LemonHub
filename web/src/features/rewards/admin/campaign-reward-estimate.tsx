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

import { formatQuotaWithCurrency } from '@/lib/currency'

import type { RewardCampaignInput } from './types'

type CampaignRewardEstimateProps = {
  input: RewardCampaignInput
  recipientCount?: number
}

export function CampaignRewardEstimate(props: CampaignRewardEstimateProps) {
  const { t } = useTranslation()
  let estimate = t('Not previewed')
  if (props.recipientCount !== undefined) {
    const minimum = props.input.min_quota * props.recipientCount
    const maximum = props.input.max_quota * props.recipientCount
    if (Number.isSafeInteger(minimum) && Number.isSafeInteger(maximum)) {
      estimate = formatQuotaWithCurrency(minimum, { abbreviate: false })
      if (props.input.type === 'random') {
        estimate += ` – ${formatQuotaWithCurrency(maximum, { abbreviate: false })}`
      }
    } else {
      // This only affects the optional estimate, never campaign creation.
      estimate = t('Estimate unavailable')
    }
  }

  return (
    <div className='space-y-3'>
      <dl className='grid gap-4 sm:grid-cols-3'>
        <div>
          <dt className='text-muted-foreground text-sm'>
            {t('Current eligible users')}
          </dt>
          <dd className='mt-1 font-semibold tabular-nums'>
            {props.recipientCount ?? t('Not previewed')}
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-sm'>
            {t('Estimated amount for current users')}
          </dt>
          <dd className='mt-1 font-semibold break-words tabular-nums'>
            {estimate}
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-sm'>
            {t('Reward validity')}
          </dt>
          <dd className='mt-1 font-semibold tabular-nums'>
            {t('{{count}} days per reward', {
              count: props.input.expires_in_days,
            })}
          </dd>
        </div>
      </dl>
      <p className='text-muted-foreground text-xs'>
        {t(
          'This preview is a current snapshot, not a final recipient count or budget.'
        )}
      </p>
    </div>
  )
}
