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
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { getUserQuotaDates } from '@/features/dashboard/api'
import { useSummaryCardsConfig } from '@/features/dashboard/hooks/use-dashboard-config'
import type { QuotaDataItem } from '@/features/dashboard/types'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { useStatus } from '@/hooks/use-status'
import { getCurrencyLabel } from '@/lib/currency'
import { formatNumber, formatQuota } from '@/lib/format'
import { computeTimeRange } from '@/lib/time'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { StatCard } from '../ui/stat-card'
import {
  WorkspaceActivityChart,
  type ActivityMetric,
} from './workspace-activity-chart'

const SUMMARY_SPARKLINE_BUCKETS = 12

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

function buildSummarySparklines(
  data: QuotaDataItem[],
  start: number,
  end: number
): Record<ActivityMetric, number[]> {
  const usage = Array.from({ length: SUMMARY_SPARKLINE_BUCKETS }, () => 0)
  const requests = Array.from({ length: SUMMARY_SPARKLINE_BUCKETS }, () => 0)

  for (const item of data) {
    const timestamp = Number(item.created_at) || start
    const index = getBucketIndex(
      timestamp,
      start,
      end,
      SUMMARY_SPARKLINE_BUCKETS
    )
    usage[index] += Number(item.quota) || 0
    requests[index] += Number(item.count) || 0
  }

  return { usage, requests }
}

function getSummarySparkline(
  key: string,
  sparklineData: Record<ActivityMetric, number[]>
): number[] | undefined {
  if (key === 'usage' || key === 'todayUsage') return sparklineData.usage
  if (key === 'requests') return sparklineData.requests
  return undefined
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
  const [chartMetric, setChartMetric] = useState<ActivityMetric>('usage')
  const user = useAuthStore((state) => state.auth.user)
  const { loading } = useStatus()
  const currency = useSystemConfigStore((state) => state.config.currency)
  const showWallet = useIsSidebarModuleVisible('/wallet')

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
      getUserQuotaDates(
        {
          start_timestamp: summaryTimeRange.start_timestamp,
          end_timestamp: summaryTimeRange.end_timestamp,
          default_time: 'hour',
        },
        false,
        { skipBusinessError: true, skipErrorHandler: true }
      ).catch((cause: unknown) => {
        // Retain known account totals and let the usage card offer local recovery.
        throw new Error('Failed to fetch usage', { cause })
      }),
    staleTime: 60 * 1000,
    retry: false,
  })

  const summaryValues = {
    usedDisplay: formatQuota(usedQuota),
    requestCountDisplay: formatNumber(requestCount),
  }
  const currencyEnabled = currency.quotaDisplayType !== 'TOKENS'
  const currencyLabel = currencyEnabled ? getCurrencyLabel() : 'Tokens'

  const sparklineData = useMemo(
    () =>
      buildSummarySparklines(
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

  const trendFailed =
    usageTrendQuery.isError || usageTrendQuery.data?.success === false
  const trendReady = !usageTrendQuery.isPending && !trendFailed
  const recentUsage = sparklineData.usage.reduce((sum, quota) => sum + quota, 0)
  const healthLevel = getHealthLevel(remainQuota, recentUsage)
  const healthCfg = HEALTH_CONFIG[healthLevel]
  const runwayDays = getRunwayDays(remainQuota, recentUsage)

  const todayUsageDisplay = formatQuota(recentUsage)
  let runwayDisplay: string
  if (trendFailed) {
    runwayDisplay = t('Failed to fetch usage')
  } else if (!trendReady) {
    runwayDisplay = t('Loading')
  } else if (runwayDays !== null) {
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

  const items = useSummaryCardsConfig({
    ...summaryValues,
    todayUsageDisplay,
    currencyEnabled,
    currencyLabel,
  }).map((config, index) => {
    const tones = ['accent-1', 'accent-2', 'accent-3'] as const

    return {
      key: config.key,
      title: config.title,
      value: config.value,
      desc: config.description,
      icon: config.icon,
      tone: tones[index] ?? 'accent-3',
      sparkline: trendReady
        ? getSummarySparkline(config.key, sparklineData)
        : undefined,
      sparklineVariant: 'line' as const,
    }
  })

  return (
    <Dialog>
      <section
        aria-label={t('Usage at a glance')}
        className='bg-card @container/overview min-w-0 overflow-hidden rounded-lg border'
      >
        <div className='flex flex-wrap items-center justify-between gap-2 border-b px-4 py-3 sm:px-5'>
          <h3 className='text-base font-semibold tracking-tight'>
            {t('Usage at a glance')}
          </h3>
          <Button
            variant='ghost'
            size='sm'
            render={
              <Link to='/dashboard/$section' params={{ section: 'models' }} />
            }
          >
            {t('Model Call Analytics')} <ArrowRight data-icon='inline-end' />
          </Button>
        </div>
        <div className='grid @3xl/overview:grid-cols-[minmax(12rem,.85fr)_minmax(0,2fr)]'>
          <div className='bg-muted/60 flex min-w-0 flex-col justify-between gap-4 border-b p-4 sm:p-5 @3xl/overview:border-r @3xl/overview:border-b-0'>
            <div>
              <div className='flex flex-wrap items-center justify-between gap-2'>
                <h4 className='text-sm font-medium'>{t('Credit remaining')}</h4>
                {(trendReady || remainQuota <= 0) && (
                  <span className='text-muted-foreground inline-flex items-center gap-1.5 text-xs'>
                    <span
                      className={cn(
                        'size-1.5 rounded-full',
                        healthCfg.dotClass
                      )}
                      aria-hidden='true'
                    />
                    {t(healthCfg.labelKey)}
                  </span>
                )}
              </div>
              <p className='mt-2 overflow-x-auto text-3xl font-semibold tracking-tight whitespace-nowrap tabular-nums sm:text-4xl'>
                {formatQuota(remainQuota)}
              </p>
            </div>
            <div className='flex flex-wrap items-center justify-between gap-2'>
              <div className='text-muted-foreground text-xs'>
                {t('Runway')}{' '}
                <span className='text-foreground ms-1 font-medium'>
                  {runwayDisplay}
                </span>
                {trendFailed && (
                  <Button
                    variant='ghost'
                    size='sm'
                    onClick={() => void usageTrendQuery.refetch()}
                  >
                    {t('Retry')}
                  </Button>
                )}
              </div>
              {showWallet && (
                <Button
                  size='sm'
                  variant='outline'
                  render={<Link to='/wallet' />}
                >
                  {t('Wallet')} <ArrowRight data-icon='inline-end' />
                </Button>
              )}
            </div>
          </div>
          <div className='@container/summary min-w-0'>
            <div className='grid h-full grid-cols-1 divide-y @md/summary:grid-cols-3 @md/summary:divide-x @md/summary:divide-y-0'>
              {items.map((item) => {
                const metric = item.key === 'requests' ? 'requests' : 'usage'
                const trendLabel =
                  metric === 'requests'
                    ? t('Requests by time of day')
                    : t('Usage by time of day')
                return (
                  <div key={item.key} className='relative min-w-0 p-4 sm:p-5'>
                    {(item.key === 'todayUsage' || item.key === 'requests') && (
                      <DialogTrigger
                        type='button'
                        aria-label={`${trendLabel} (${t('Last 24 hours')})`}
                        title={`${trendLabel} (${t('Last 24 hours')})`}
                        onClick={() => setChartMetric(metric)}
                        className='hover:bg-muted/35 focus-visible:ring-ring absolute inset-0 z-1 cursor-pointer rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-inset'
                      />
                    )}
                    <StatCard
                      title={item.title}
                      value={item.value}
                      description={item.desc}
                      icon={item.icon}
                      tone={item.tone}
                      sparkline={item.sparkline}
                      sparklineVariant={item.sparklineVariant}
                      loading={
                        loading ||
                        (item.key === 'todayUsage' && usageTrendQuery.isPending)
                      }
                      error={item.key === 'todayUsage' && trendFailed}
                      compactMobile
                    />
                  </div>
                )
              })}
            </div>
          </div>
        </div>
      </section>
      <DialogContent className='sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>{t('Workspace activity')}</DialogTitle>
          <DialogDescription>{t('Last 24 hours')}</DialogDescription>
        </DialogHeader>
        <WorkspaceActivityChart
          selected={chartMetric}
          onSelect={setChartMetric}
          series={sparklineData}
          start={summaryTimeRange.start_timestamp}
          end={summaryTimeRange.end_timestamp}
          loading={usageTrendQuery.isPending}
          failed={trendFailed}
          retry={() => void usageTrendQuery.refetch()}
        />
      </DialogContent>
    </Dialog>
  )
}
