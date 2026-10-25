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
import { useQuery } from '@tanstack/react-query'
import { Gift } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatQuotaWithCurrency } from '@/lib/currency'
import { formatTimestamp } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'

import { listRewardCampaigns } from './api'
import { formatRewardClaimedAmount } from './lib/claimed-amount'
import type { RewardDistributionStatus } from './types'

const distributionStatusLabels: Record<RewardDistributionStatus, string> = {
  backfilling: 'Processing eligible users',
  scheduled: 'Waiting for activity period',
  active: 'Automatically distributing',
  completed: 'Activity period ended',
  legacy: 'Legacy one-time distribution',
}

export function CampaignHistory() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const userId = useAuthStore((state) => state.auth.user?.id)
  const query = useQuery({
    queryKey: ['reward-campaigns', userId, page],
    queryFn: () => listRewardCampaigns(page),
    enabled: Boolean(userId),
    refetchInterval: 30_000,
    refetchOnWindowFocus: true,
  })
  const campaigns = query.data?.items ?? []
  const total = query.data?.total ?? 0

  return (
    <Card className='rounded-2xl'>
      <CardHeader>
        <CardTitle>{t('Distribution history')}</CardTitle>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Validity starts separately when each user receives their reward. Existing one-time campaigns stay unchanged.'
          )}
        </p>
      </CardHeader>
      <CardContent>
        {query.isLoading && (
          <p role='status' className='text-muted-foreground py-8 text-center'>
            {t('Loading...')}
          </p>
        )}
        {query.isError && (
          <div role='alert' className='flex flex-col items-center gap-3 py-8'>
            <p>{t('Failed to load reward campaigns')}</p>
            <Button
              variant='outline'
              onClick={() => {
                void query.refetch()
              }}
            >
              {t('Retry')}
            </Button>
          </div>
        )}
        {!query.isLoading && !query.isError && campaigns.length === 0 && (
          <div className='text-muted-foreground flex flex-col items-center gap-3 py-10'>
            <Gift className='size-8' aria-hidden='true' />
            <p>{t('No reward campaigns yet')}</p>
          </div>
        )}
        {campaigns.length > 0 && !query.isError && (
          <div className='overflow-x-auto'>
            <Table className='min-w-160'>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Campaign')}</TableHead>
                  <TableHead>{t('Reward type')}</TableHead>
                  <TableHead>{t('Amount per user')}</TableHead>
                  <TableHead>{t('Rewards issued')}</TableHead>
                  <TableHead>{t('Claimed recipients')}</TableHead>
                  <TableHead>{t('Total claimed amount')}</TableHead>
                  <TableHead>{t('Activity period')}</TableHead>
                  <TableHead>{t('Created at')}</TableHead>
                  <TableHead>{t('Reward validity')}</TableHead>
                  <TableHead>{t('Distribution status')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {campaigns.map((campaign) => {
                  let validity = t('No expiration')
                  let statusLabel = 'Legacy one-time distribution'
                  if (campaign.continuous) {
                    validity = t('{{count}} days per reward', {
                      count: campaign.expires_in_days,
                    })
                    statusLabel = campaign.distribution_status
                      ? distributionStatusLabels[campaign.distribution_status]
                      : 'Unknown'
                  } else if (campaign.expires_at > 0) {
                    validity = formatTimestamp(campaign.expires_at)
                  }
                  return (
                    <TableRow key={campaign.id}>
                      <TableCell>
                        <div className='max-w-64 font-medium break-words whitespace-normal'>
                          {campaign.title}
                        </div>
                      </TableCell>
                      <TableCell>
                        {campaign.type === 'fixed'
                          ? t('Reward credit')
                          : t('Lucky red packet')}
                      </TableCell>
                      <TableCell className='tabular-nums'>
                        {formatQuotaWithCurrency(campaign.min_quota, {
                          abbreviate: false,
                        })}
                        {campaign.type === 'random' &&
                          ` – ${formatQuotaWithCurrency(campaign.max_quota, { abbreviate: false })}`}
                      </TableCell>
                      <TableCell className='tabular-nums'>
                        {campaign.recipient_count}
                      </TableCell>
                      <TableCell className='tabular-nums'>
                        {campaign.claimed_count}
                      </TableCell>
                      <TableCell className='tabular-nums'>
                        {formatRewardClaimedAmount(campaign)}
                      </TableCell>
                      <TableCell>
                        <div className='text-xs'>
                          {formatTimestamp(campaign.start_time)}
                          <br />
                          {formatTimestamp(campaign.end_time)}
                        </div>
                      </TableCell>
                      <TableCell className='text-xs'>
                        {formatTimestamp(campaign.created_at)}
                      </TableCell>
                      <TableCell className='text-xs'>{validity}</TableCell>
                      <TableCell className='max-w-48 text-xs whitespace-normal'>
                        {t(statusLabel)}
                        {campaign.continuous &&
                          campaign.distribution_status === 'completed' && (
                            <p className='text-muted-foreground mt-1'>
                              {t(
                                'Late records from the activity period can still qualify for rewards.'
                              )}
                            </p>
                          )}
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </div>
        )}
        {total > 10 && (
          <div className='mt-4 flex flex-wrap items-center justify-between gap-3'>
            <p className='text-muted-foreground text-sm'>
              {t('Page {{page}} of {{pages}}', {
                page,
                pages: Math.ceil(total / 10),
              })}
            </p>
            <div className='flex gap-2'>
              <Button
                variant='outline'
                disabled={page <= 1 || query.isFetching}
                onClick={() => setPage((current) => current - 1)}
              >
                {t('Previous')}
              </Button>
              <Button
                variant='outline'
                disabled={page * 10 >= total || query.isFetching}
                onClick={() => setPage((current) => current + 1)}
              >
                {t('Next')}
              </Button>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
