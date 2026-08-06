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
  TriangleAlert,
  UserPlus,
  Wallet,
  type LucideIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { formatQuota } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useReferralAdminSummary } from '../hooks/use-referral-admin-summary'

interface SummaryTile {
  label: string
  value: string
  icon: LucideIcon
}

export function ReferralAdminSummaryCards() {
  const { t } = useTranslation()
  const { data, isLoading, isError, refetch } = useReferralAdminSummary()

  if (isLoading) {
    return (
      <Card data-card-hover='false' className='bg-muted/20 py-0'>
        <CardContent className='p-4 sm:p-5'>
          <div className='grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5'>
            {['a', 'b', 'c', 'd', 'e'].map((k) => (
              <div key={k} className='space-y-2'>
                <Skeleton className='h-3.5 w-20' />
                <Skeleton className='h-6 w-24' />
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    )
  }

  if (isError) {
    return (
      <Card data-card-hover='false' className='bg-muted/20 py-0'>
        <CardContent className='flex flex-col items-center gap-2 p-6 text-center'>
          <TriangleAlert
            className='text-muted-foreground size-5'
            aria-hidden='true'
          />
          <span className='text-muted-foreground text-sm'>
            {t('Failed to load')}
          </span>
          <Button variant='outline' size='sm' onClick={() => refetch()}>
            {t('Retry')}
          </Button>
        </CardContent>
      </Card>
    )
  }

  const tiles: SummaryTile[] = [
    {
      label: t('Total Commission Paid'),
      value: formatQuota(data?.total_commission_paid ?? 0),
      icon: Wallet,
    },
    {
      label: t('Total Pending'),
      value: formatQuota(data?.total_pending_quota ?? 0),
      icon: Clock,
    },
    {
      label: t('Total Activated'),
      value: (data?.total_activated ?? 0).toLocaleString(),
      icon: CheckCircle2,
    },
    {
      label: t('Inviters'),
      value: (data?.inviter_count ?? 0).toLocaleString(),
      icon: UserPlus,
    },
    {
      label: t("This Month's Commission"),
      value: formatQuota(data?.month_commission_quota ?? 0),
      icon: TrendingUp,
    },
  ]

  return (
    <Card data-card-hover='false' className='bg-muted/20 py-0'>
      <CardContent className='p-4 sm:p-5'>
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
      </CardContent>
    </Card>
  )
}
