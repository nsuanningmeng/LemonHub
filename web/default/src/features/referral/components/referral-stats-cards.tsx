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
import {
  CheckCircle2,
  Clock,
  TrendingUp,
  Users,
  Wallet,
  type LucideIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { formatQuota } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { CopyButton } from '@/components/copy-button'
import type { AffStats } from '../types'

interface ReferralStatsCardsProps {
  stats?: AffStats
  loading: boolean
  affiliateLink: string
  onTransfer: () => void
}

interface StatTile {
  label: string
  value: string
  icon: LucideIcon
}

export function ReferralStatsCards(props: ReferralStatsCardsProps) {
  const { t } = useTranslation()

  if (props.loading) {
    return (
      <Card data-card-hover='false' className='bg-muted/20 py-0'>
        <CardContent className='space-y-4 p-4 sm:p-5'>
          <div className='grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5'>
            {Array.from({ length: 5 }).map((_, i) => (
              <div key={i} className='space-y-2'>
                <Skeleton className='h-3.5 w-20' />
                <Skeleton className='h-6 w-24' />
              </div>
            ))}
          </div>
          <div className='border-t pt-4'>
            <Skeleton className='h-3.5 w-28' />
            <Skeleton className='mt-2 h-9 w-full rounded-lg' />
          </div>
        </CardContent>
      </Card>
    )
  }

  const stats = props.stats
  const hasPending = (stats?.pending_quota ?? 0) > 0

  const tiles: StatTile[] = [
    {
      label: t('Pending'),
      value: formatQuota(stats?.pending_quota ?? 0),
      icon: Clock,
    },
    {
      label: t('Total Earned'),
      value: formatQuota(stats?.total_earned_quota ?? 0),
      icon: Wallet,
    },
    {
      label: t("This Month's Commission"),
      value: formatQuota(stats?.month_commission_quota ?? 0),
      icon: TrendingUp,
    },
    {
      label: t('Activated'),
      value: (stats?.activated_count ?? 0).toLocaleString(),
      icon: CheckCircle2,
    },
    {
      label: t('Total Invited'),
      value: (stats?.total_invited ?? 0).toLocaleString(),
      icon: Users,
    },
  ]

  return (
    <Card data-card-hover='false' className='bg-muted/20 py-0'>
      <CardContent className='space-y-4 p-4 sm:p-5'>
        <div className='grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5'>
          {tiles.map((tile) => (
            <div key={tile.label} className='min-w-0'>
              <div className='text-muted-foreground flex items-center gap-1.5'>
                <tile.icon
                  className='size-3.5 shrink-0 opacity-70'
                  aria-hidden='true'
                />
                <span className='truncate text-[10px] font-medium tracking-wider uppercase'>
                  {tile.label}
                </span>
              </div>
              <div className='text-foreground mt-1 truncate text-lg font-semibold tabular-nums'>
                {tile.value}
              </div>
            </div>
          ))}
        </div>

        <div className='border-t pt-4'>
          <Label
            htmlFor='referral-link'
            className='text-muted-foreground text-xs font-medium tracking-wider uppercase'
          >
            {t('Your Referral Link')}
          </Label>
          <div className='mt-2 flex items-center gap-2'>
            <Input
              id='referral-link'
              value={props.affiliateLink}
              readOnly
              className='border-muted bg-background/70 h-9 min-w-0 flex-1 font-mono text-xs'
            />
            <CopyButton
              value={props.affiliateLink}
              variant='outline'
              className='bg-background size-9 shrink-0'
              iconClassName='size-4'
              tooltip={t('Copy referral link')}
              aria-label={t('Copy referral link')}
            />
            {hasPending && (
              <Button
                onClick={props.onTransfer}
                className='h-9 shrink-0 px-3'
                size='sm'
              >
                {t('Transfer to Balance')}
              </Button>
            )}
          </div>
        </div>
      </CardContent>
    </Card>
  )
}
