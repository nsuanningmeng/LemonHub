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
import { HStack } from '@astryxdesign/core/HStack'
import { VStack } from '@astryxdesign/core/VStack'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ArrowRight, Flame, ShieldCheck, TrendingDown } from 'lucide-react'
import { useId, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { StaggerContainer, StaggerItem } from '@/components/page-transition'
import { Button, buttonVariants } from '@/components/ui/button'
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

const SUMMARY_SPARKLINE_BUCKETS = 12

type SummarySparklineKey = 'balance' | 'usage' | 'requests'

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
  currentBalance: number,
  start: number,
  end: number
): Record<SummarySparklineKey, number[]> {
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

  let balance = currentBalance
  const balanceTrend = Array.from(
    { length: SUMMARY_SPARKLINE_BUCKETS },
    () => 0
  )

  for (let index = SUMMARY_SPARKLINE_BUCKETS - 1; index >= 0; index--) {
    balanceTrend[index] = Math.max(0, balance)
    balance += usage[index]
  }

  return {
    balance: balanceTrend,
    usage,
    requests,
  }
}

function getSummarySparkline(
  key: string,
  sparklineData: Record<SummarySparklineKey, number[]>
): number[] | undefined {
  if (key === 'usage') return sparklineData.usage
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

type HealthLevel = 'healthy' | 'caution' | 'critical' | 'unknown'

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
  unknown: {
    dotClass: 'bg-current/50',
    labelKey: 'N/A',
  },
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
  const headingId = useId()
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
        remainQuota,
        summaryTimeRange.start_timestamp,
        summaryTimeRange.end_timestamp
      ),
    [
      remainQuota,
      summaryTimeRange.end_timestamp,
      summaryTimeRange.start_timestamp,
      usageTrendQuery.data?.data,
    ]
  )

  const recentUsage = useMemo(
    () =>
      (usageTrendQuery.data?.data ?? []).reduce(
        (total, item) => total + (Number(item.quota) || 0),
        0
      ),
    [usageTrendQuery.data?.data]
  )

  const usageUnavailable =
    usageTrendQuery.isError || usageTrendQuery.data?.success === false
  const usagePending = usageTrendQuery.isPending
  let healthLevel: HealthLevel = getHealthLevel(remainQuota, recentUsage)
  if (remainQuota > 0 && (usageUnavailable || usagePending)) {
    healthLevel = 'unknown'
  }
  const healthCfg = HEALTH_CONFIG[healthLevel]
  const runwayDays =
    usageUnavailable || usagePending
      ? null
      : getRunwayDays(remainQuota, recentUsage)

  let todayUsageDisplay = formatQuota(recentUsage)
  if (usagePending) todayUsageDisplay = t('Loading')
  if (usageUnavailable) todayUsageDisplay = t('N/A')
  let runwayDisplay: string
  if (runwayDays !== null) {
    if (runwayDays < 1) {
      runwayDisplay = t('Less than 1 day left')
    } else if (runwayDays > 999) {
      runwayDisplay = `999+ ${t('days')}`
    } else {
      runwayDisplay = `~${formatNumber(Math.floor(runwayDays))} ${t('days')}`
    }
  } else if (usageUnavailable || usagePending) {
    runwayDisplay = t('N/A')
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
      sparkline:
        config.key === 'todayUsage'
          ? sparklineData.usage
          : getSummarySparkline(config.key, sparklineData),
      sparklineVariant: 'line' as const,
    }
  })

  return (
    <section aria-labelledby={headingId}>
      <HStack
        gap={3}
        wrap='wrap'
        justify='between'
        align='center'
        className='mb-4'
      >
        <VStack gap={1}>
          <h3 id={headingId} className='text-sm font-semibold tracking-tight'>
            {t('Account pulse')}
          </h3>
        </VStack>
        <p className='text-muted-foreground font-mono text-xs'>
          {t('Last 24 hours')}
        </p>
      </HStack>
      <section
        className='grid min-w-0 gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]'
        aria-label={t('Usage at a glance')}
      >
        <VStack
          gap={6}
          padding={6}
          justify='between'
          className='surface-brand min-w-0 overflow-hidden rounded-2xl'
        >
          <HStack
            gap={3}
            justify='between'
            wrap='wrap'
            align='center'
            className='text-inherit'
          >
            <p className='text-xs font-medium tracking-wide'>
              {t('Credit remaining')}
            </p>
            <HStack gap={1.5} align='center' className='text-xs text-inherit'>
              <i
                className={cn('size-1.5 rounded-full', healthCfg.dotClass)}
                aria-hidden='true'
              />
              <p>{t(healthCfg.labelKey)}</p>
            </HStack>
          </HStack>
          <p
            className='overflow-x-auto font-mono text-3xl leading-none font-medium tracking-tight whitespace-nowrap tabular-nums sm:text-4xl'
            title={formatQuota(remainQuota)}
          >
            {formatQuota(remainQuota)}
          </p>
          <dl className='grid grid-cols-2 gap-4 border-t border-current/20 pt-4'>
            <VStack gap={2} className='text-inherit'>
              <dt className='flex items-center gap-1.5 text-xs'>
                <Flame className='size-3.5' aria-hidden='true' />
                {t('Last 24h usage')}
              </dt>
              <dd className='font-mono text-sm tabular-nums'>
                {todayUsageDisplay}
              </dd>
            </VStack>
            <VStack gap={2} className='text-inherit'>
              <dt className='flex items-center gap-1.5 text-xs'>
                {runwayDays !== null && runwayDays < 3 ? (
                  <TrendingDown className='size-3.5' aria-hidden='true' />
                ) : (
                  <ShieldCheck className='size-3.5' aria-hidden='true' />
                )}
                {t('Runway')}
              </dt>
              <dd className='font-mono text-sm tabular-nums'>
                {runwayDisplay}
              </dd>
            </VStack>
          </dl>
          {showWallet && (
            <Link
              to='/wallet'
              className={cn(
                buttonVariants(),
                'bg-brand-paper text-brand-ink hover:bg-brand-paper/90 w-full justify-between border-0'
              )}
            >
              {t('Manage funds')}
              <ArrowRight data-icon='inline-end' />
            </Link>
          )}
        </VStack>
        <StaggerContainer className='@container/summary grid min-w-0 grid-cols-1 gap-8 py-4 sm:grid-cols-3 lg:py-6'>
          {items.map((item) => (
            <StaggerItem key={item.key} className='min-w-0'>
              <StatCard
                title={item.title}
                value={item.value}
                description={
                  item.key === 'todayUsage' && usageUnavailable
                    ? t('Failed to fetch usage')
                    : item.desc
                }
                icon={item.icon}
                action={
                  item.key === 'todayUsage' && usageUnavailable ? (
                    <Button
                      size='sm'
                      variant='ghost'
                      onClick={() => void usageTrendQuery.refetch()}
                      disabled={usageTrendQuery.isFetching}
                    >
                      {t('Retry')}
                    </Button>
                  ) : undefined
                }
                tone={item.tone}
                sparkline={
                  usageUnavailable || usagePending ? undefined : item.sparkline
                }
                sparklineVariant={item.sparklineVariant}
                loading={loading || (item.key === 'todayUsage' && usagePending)}
                error={item.key === 'todayUsage' && usageUnavailable}
                compactMobile
              />
            </StaggerItem>
          ))}
        </StaggerContainer>
      </section>
    </section>
  )
}
