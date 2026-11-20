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
import { Link } from '@tanstack/react-router'
import { ArrowRight } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { getUserQuotaDates } from '@/features/dashboard/api'
import type { QuotaDataItem } from '@/features/dashboard/types'
import { formatNumber, formatQuota } from '@/lib/format'
import { computeTimeRange } from '@/lib/time'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { WorkspaceActivityChart } from './workspace-activity-chart'

const ACTIVITY_BUCKET_COUNT = 12

type ActivityMetric = 'usage' | 'requests'

function getBucketIndex(
  timestamp: number,
  start: number,
  end: number,
  bucketCount: number
): number {
  if (end <= start) return 0
  const ratio = (timestamp - start) / (end - start)
  return Math.min(bucketCount - 1, Math.max(0, Math.floor(ratio * bucketCount)))
}

function buildActivitySeries(
  data: QuotaDataItem[],
  start: number,
  end: number
): Record<ActivityMetric, number[]> {
  const usage = Array.from({ length: ACTIVITY_BUCKET_COUNT }, () => 0)
  const requests = Array.from({ length: ACTIVITY_BUCKET_COUNT }, () => 0)

  for (const item of data) {
    const timestamp = Number(item.created_at) || start
    const index = getBucketIndex(timestamp, start, end, ACTIVITY_BUCKET_COUNT)
    usage[index] += Number(item.quota) || 0
    requests[index] += Number(item.count) || 0
  }

  return { usage, requests }
}

function getRunwayDays(
  remainQuota: number,
  recentUsage: number
): number | null {
  if (remainQuota <= 0 || recentUsage <= 0) return null
  const days = remainQuota / recentUsage
  if (!Number.isFinite(days)) return null
  return days
}

type HealthLevel = 'healthy' | 'caution' | 'critical'

function getHealthLevel(remainQuota: number, recentUsage: number): HealthLevel {
  if (remainQuota <= 0) return 'critical'
  const days = getRunwayDays(remainQuota, recentUsage)
  if (days !== null && days < 3) return 'caution'
  return 'healthy'
}

const HEALTH_CONFIG: Record<
  HealthLevel,
  { dotClass: string; labelKey: string }
> = {
  healthy: {
    dotClass: 'bg-success',
    labelKey: 'Healthy',
  },
  caution: {
    dotClass: 'bg-warning',
    labelKey: 'Low balance',
  },
  critical: {
    dotClass: 'bg-destructive',
    labelKey: 'Balance depleted',
  },
}

export function SummaryCards() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)

  const summaryTimeRange = useMemo(() => computeTimeRange(1), [])
  const remainQuota = Number(user?.quota ?? 0)
  const usedQuota = Number(user?.used_quota ?? 0)
  const requestCount = Number(user?.request_count ?? 0)

  const usageTrendQuery = useQuery({
    queryKey: [
      'dashboard',
      'overview',
      'summary-sparklines',
      summaryTimeRange.start_timestamp,
      summaryTimeRange.end_timestamp,
    ],
    queryFn: async () =>
      getUserQuotaDates({
        start_timestamp: summaryTimeRange.start_timestamp,
        end_timestamp: summaryTimeRange.end_timestamp,
        default_time: 'hour',
      }),
    staleTime: 60 * 1000,
  })

  const activitySeries = useMemo(
    () =>
      buildActivitySeries(
        usageTrendQuery.data?.data ?? [],
        summaryTimeRange.start_timestamp,
        summaryTimeRange.end_timestamp
      ),
    [
      summaryTimeRange.end_timestamp,
      summaryTimeRange.start_timestamp,
      usageTrendQuery.data?.data,
    ]
  )

  const recentUsage = activitySeries.usage.reduce(
    (sum, quota) => sum + quota,
    0
  )

  const healthLevel = getHealthLevel(remainQuota, recentUsage)
  const healthCfg = HEALTH_CONFIG[healthLevel]
  const runwayDays = getRunwayDays(remainQuota, recentUsage)

  let runwayDisplay: string
  if (runwayDays !== null) {
    if (runwayDays < 1) {
      runwayDisplay = t('Less than 1 day left')
    } else if (runwayDays > 999) {
      runwayDisplay = `999+ ${t('days')}`
    } else {
      runwayDisplay = `~${formatNumber(Math.floor(runwayDays))} ${t('days')}`
    }
  } else if (remainQuota <= 0) {
    runwayDisplay = t('Balance depleted')
  } else {
    runwayDisplay = t('No recent usage')
  }

  const trendFailed =
    usageTrendQuery.isError || usageTrendQuery.data?.success === false

  return (
    <section className='@container/overview min-w-0 border-y'>
      <div className='grid min-w-0 gap-x-6 @3xl/overview:grid-cols-[minmax(0,1.8fr)_minmax(13.5rem,.82fr)]'>
        <WorkspaceActivityChart
          series={activitySeries}
          start={summaryTimeRange.start_timestamp}
          end={summaryTimeRange.end_timestamp}
          failed={trendFailed}
          loading={usageTrendQuery.isLoading}
          retry={() => void usageTrendQuery.refetch()}
        />
        <div className='bg-brand-ink my-4 flex min-w-0 flex-col justify-between gap-4 rounded-lg p-5 text-white sm:p-6'>
          <div>
            <p className='text-sm font-medium text-white/80'>
              {t('Available credits')}
            </p>
            <p className='mt-2 max-w-full overflow-x-auto text-4xl font-semibold tracking-tight whitespace-nowrap tabular-nums'>
              {formatQuota(remainQuota)}
            </p>
          </div>
          <div>
            <div className='flex flex-wrap items-center gap-x-3 gap-y-1 text-xs'>
              <span className='inline-flex items-center gap-1.5'>
                <span
                  aria-hidden='true'
                  className={cn('size-2 rounded-full', healthCfg.dotClass)}
                />
                {t(healthCfg.labelKey)}
              </span>
              {!trendFailed && !usageTrendQuery.isLoading && (
                <span className='text-white/75'>{runwayDisplay}</span>
              )}
            </div>
            <Button
              size='sm'
              className='bg-brand-citrus text-brand-ink hover:bg-brand-citrus/90 mt-4 w-full justify-between'
              render={<Link to='/wallet' />}
            >
              {t('Manage funds')} <ArrowRight data-icon='inline-end' />
            </Button>
          </div>
        </div>
      </div>
      <div className='flex flex-wrap items-center gap-x-6 gap-y-2 border-t py-3 text-sm'>
        <span className='text-muted-foreground'>
          {t('Lifetime usage')}{' '}
          <b className='text-foreground ms-1 font-medium tabular-nums'>
            {formatQuota(usedQuota)}
          </b>
        </span>
        <span className='text-muted-foreground'>
          {t('Lifetime requests')}{' '}
          <b className='text-foreground ms-1 font-medium tabular-nums'>
            {formatNumber(requestCount)}
          </b>
        </span>
        <Button
          size='sm'
          variant='ghost'
          className='sm:ms-auto'
          render={
            <Link to='/dashboard/$section' params={{ section: 'models' }} />
          }
        >
          {t('Explore analytics')} <ArrowRight data-icon='inline-end' />
        </Button>
      </div>
    </section>
  )
}
