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
import { VStack } from '@astryxdesign/core/VStack'
import { Link } from '@tanstack/react-router'
import { Share2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { formatQuota } from '@/lib/format'

import type { UserWalletData } from '../types'

interface AffiliateRewardsCardProps {
  user: UserWalletData | null
  affiliateLink: string
  onTransfer: () => void
  complianceConfirmed?: boolean
  loading?: boolean
}

export function AffiliateRewardsCard({
  user,
  affiliateLink,
  onTransfer,
  complianceConfirmed = true,
  loading,
}: AffiliateRewardsCardProps) {
  const { t } = useTranslation()
  if (loading) {
    return (
      <VStack
        as='section'
        gap={0}
        aria-label={t('Referral Program')}
        className='border-brand-line border-t pt-6'
      >
        <section className='grid gap-4 p-3 sm:p-4 lg:grid-cols-[minmax(220px,1fr)_minmax(220px,0.72fr)_minmax(320px,1.15fr)] lg:items-center'>
          <div>
            <Skeleton className='h-5 w-32' />
            <Skeleton className='mt-2 h-4 w-48' />
          </div>
          <Skeleton className='h-14 rounded-lg' />
          <Skeleton className='h-10 rounded-lg' />
        </section>
      </VStack>
    )
  }

  const hasRewards = (user?.aff_quota ?? 0) > 0
  const isCashSettled = user?.aff_cash_settled ?? false

  return (
    <VStack
      as='section'
      gap={0}
      aria-label={t('Referral Program')}
      className='border-brand-line border-t pt-6'
    >
      <section className='grid gap-5 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)] lg:items-center'>
        <div className='flex min-w-0 items-start gap-3'>
          <Share2
            className='text-tone-violet-strong mt-1 size-4 shrink-0'
            aria-hidden='true'
          />
          <div className='min-w-0'>
            <div className='flex flex-wrap items-center gap-2'>
              <h3 className='truncate text-sm font-semibold'>
                {t('Referral Program')}
              </h3>
              {isCashSettled && (
                <small className='text-warning shrink-0 text-xs'>
                  {t('Cash settled')}
                </small>
              )}
              <Button
                variant='link'
                size='sm'
                className='h-auto shrink-0 p-0 text-xs'
                render={<Link to='/referral' />}
              >
                {t('View Details')}
              </Button>
            </div>
            <p
              className={`text-muted-foreground text-xs ${isCashSettled ? 'line-clamp-2' : 'line-clamp-1'}`}
            >
              {isCashSettled
                ? t(
                    'Commission is settled off-platform in cash and is not credited to your balance.'
                  )
                : t(
                    'Earn rewards when your referrals add funds. Transfer accumulated rewards to your balance anytime.'
                  )}
            </p>
          </div>
        </div>

        <div
          className={`grid gap-1.5 text-center ${isCashSettled ? 'grid-cols-1' : 'grid-cols-3'}`}
        >
          {(isCashSettled
            ? // Cash-settled: the owed/paid breakdown lives on /referral (loaded on demand) to keep
              // money figures out of the long-lived /self → localStorage payload; show only invites here.
              [[t('Invites'), String(user?.aff_count ?? 0)]]
            : [
                [t('Pending'), formatQuota(user?.aff_quota ?? 0)],
                [t('Total Earned'), formatQuota(user?.aff_history_quota ?? 0)],
                [t('Invites'), String(user?.aff_count ?? 0)],
              ]
          ).map(([label, value]) => (
            <div key={label}>
              <div className='text-muted-foreground truncate text-[10px] font-medium tracking-wider uppercase'>
                {label}
              </div>
              <div className='mt-0.5 truncate text-sm font-semibold tabular-nums'>
                {value}
              </div>
            </div>
          ))}
        </div>

        <div className='flex flex-wrap items-center gap-2 lg:col-span-2'>
          <Input
            value={affiliateLink}
            readOnly
            aria-label={t('Referral link')}
            className='border-border bg-card h-9 min-w-0 flex-1 font-mono text-xs'
          />
          <CopyButton
            value={affiliateLink}
            variant='outline'
            className='bg-background size-9 shrink-0'
            iconClassName='size-4'
            tooltip={t('Copy referral link')}
            aria-label={t('Copy referral link')}
          />
          {hasRewards && (
            <Button
              onClick={onTransfer}
              disabled={!complianceConfirmed}
              className='h-9 shrink-0 px-3'
              size='sm'
            >
              {t('Transfer to Balance')}
            </Button>
          )}
        </div>
        {!complianceConfirmed ? (
          <p className='text-muted-foreground text-xs lg:col-span-2'>
            {t(
              'Referral reward transfer is disabled until the administrator confirms compliance terms.'
            )}
          </p>
        ) : null}
      </section>
    </VStack>
  )
}
