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
import { Gauge, HeartPulse, Timer } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { getPerfMetricsSummary } from '@/features/performance-metrics/api'
import {
  formatLatency,
  formatThroughput,
} from '@/features/performance-metrics/lib/format'
import {
  formatSuccessRate,
  successRateDotClass,
  successRateTextClass,
  useSuccessRateConfig,
} from '@/features/performance-metrics/lib/success-rate'
import type { PerfModelSummary } from '@/features/performance-metrics/types'
import { cn } from '@/lib/utils'

const PERFORMANCE_WINDOW_HOURS = 24
const TOP_MODEL_LIMIT = 6

type WeightedMetric = 'avg_latency_ms' | 'avg_tps' | 'success_rate'

function simpleAverage(
  rows: PerfModelSummary[],
  metric: WeightedMetric,
  isValid: (value: number) => boolean
): number {
  let total = 0
  let count = 0
  for (const row of rows) {
    const value = Number(row[metric])
    if (!isValid(value)) continue
    total += value
    count++
  }
  return count > 0 ? total / count : Number.NaN
}

export function PerformanceHealthPanel() {
  const { t } = useTranslation()
  const srConfig = useSuccessRateConfig()
  const metricsQuery = useQuery({
    queryKey: ['perf-metrics-summary', PERFORMANCE_WINDOW_HOURS],
    queryFn: () => getPerfMetricsSummary(PERFORMANCE_WINDOW_HOURS),
    staleTime: 60 * 1000,
    retry: false,
  })

  const models = useMemo(
    () => metricsQuery.data?.data.models ?? [],
    [metricsQuery.data]
  )

  const summary = useMemo(() => {
    return {
      avgLatencyMs: Math.round(
        simpleAverage(
          models,
          'avg_latency_ms',
          (v) => Number.isFinite(v) && v > 0
        )
      ),
      avgTps: simpleAverage(
        models,
        'avg_tps',
        (v) => Number.isFinite(v) && v > 0
      ),
      successRate: simpleAverage(models, 'success_rate', Number.isFinite),
    }
  }, [models])

  const topModels = useMemo(() => models.slice(0, TOP_MODEL_LIMIT), [models])
  const loading = metricsQuery.isLoading
  const hasData = models.length > 0

  return (
    <VStack as='section' gap={5} className='min-w-0 px-3 sm:px-5'>
      <HStack gap={3} align='center' wrap='wrap' justify='between'>
        <h4 className='text-tone-teal-strong text-sm font-semibold'>
          {t('Performance health')}
        </h4>
        <span className='text-muted-foreground text-xs'>
          {t('Last 24 hours')}
        </span>
      </HStack>
      <dl className='grid grid-cols-1 gap-5 sm:grid-cols-3'>
        <MetricCell
          icon={HeartPulse}
          label={t('Success rate')}
          value={formatSuccessRate(summary.successRate, srConfig)}
          loading={loading}
          valueClassName={successRateTextClass(summary.successRate, srConfig)}
          tone='text-tone-teal-strong'
        />
        <MetricCell
          icon={Timer}
          label={t('Average latency')}
          value={formatLatency(summary.avgLatencyMs)}
          loading={loading}
          tone='text-tone-peach-strong'
        />
        <MetricCell
          icon={Gauge}
          label={t('Throughput')}
          value={formatThroughput(summary.avgTps)}
          loading={loading}
          tone='text-tone-blue-strong'
        />
      </dl>
      {loading ? (
        <VStack gap={1}>
          {['success', 'latency', 'throughput'].map((key) => (
            <Skeleton key={key} className='h-5 w-full rounded' />
          ))}
        </VStack>
      ) : (
        hasData && (
          <VStack gap={2}>
            <p className='text-muted-foreground text-xs font-medium'>
              {t('Top models by traffic')}
            </p>
            <ul className='grid grid-cols-1 gap-x-6 gap-y-2 sm:grid-cols-2'>
              {topModels.map((model) => (
                <HStack
                  as='li'
                  key={model.model_name}
                  gap={2}
                  align='center'
                  justify='between'
                >
                  <span
                    className='min-w-0 flex-1 truncate font-mono text-xs'
                    title={model.model_name}
                  >
                    {model.model_name}
                  </span>
                  <span className='inline-flex shrink-0 items-center gap-1.5'>
                    <span
                      className={cn(
                        'size-1.5 rounded-full',
                        successRateDotClass(model.success_rate, srConfig)
                      )}
                      aria-hidden='true'
                    />
                    <span
                      className={cn(
                        'font-mono text-xs tabular-nums',
                        successRateTextClass(model.success_rate, srConfig)
                      )}
                    >
                      {formatSuccessRate(model.success_rate, srConfig)}
                    </span>
                  </span>
                </HStack>
              ))}
            </ul>
          </VStack>
        )
      )}
    </VStack>
  )
}

function MetricCell(props: {
  icon: React.ComponentType<{ className?: string }>
  label: string
  value: string
  loading: boolean
  valueClassName?: string
  tone: string
}) {
  const Icon = props.icon
  return (
    <VStack gap={2} className='min-w-0'>
      <HStack
        as='dt'
        gap={2}
        align='center'
        className='text-muted-foreground text-xs'
      >
        <Icon className={cn('size-4 shrink-0', props.tone)} />
        {props.label}
      </HStack>
      <dd>
        {props.loading ? (
          <Skeleton className='h-6 w-16' />
        ) : (
          <p
            className={cn(
              'overflow-x-auto font-mono text-xl font-medium tracking-tight whitespace-nowrap tabular-nums',
              props.valueClassName ?? props.tone
            )}
          >
            {props.value}
          </p>
        )}
      </dd>
    </VStack>
  )
}
