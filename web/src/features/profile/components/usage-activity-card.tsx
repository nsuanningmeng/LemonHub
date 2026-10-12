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
import { Footprints, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { TitledCard } from '@/components/ui/titled-card'
import { toIntlLocale } from '@/i18n/languages'
import { useAuthStore } from '@/stores/auth-store'

import { getUsageActivity } from '../api'
import type { UsageActivity } from '../types'
import { UsageActivityChart } from './usage-activity-chart'

export function UsageActivityCard() {
  const { t } = useTranslation()
  const userId = useAuthStore((state) => state.auth.user?.id)
  const [timezone] = useState(
    () => Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
  )
  const activityQuery = useQuery({
    queryKey: ['profile', 'usage-activity', userId, timezone],
    enabled: userId != null,
    staleTime: 5 * 60 * 1000,
    queryFn: async ({ signal }) => {
      try {
        const response = await getUsageActivity(timezone, signal)
        if (!response.success || !response.data) {
          throw new Error(
            response.message || t('Unable to load usage activity')
          )
        }
        return response.data
      } catch (error) {
        if (signal.aborted) throw error
        // Keep an optional profile section's failure in its retry state instead
        // of triggering the application's global HTTP 500 route redirect.
        throw new Error(t('Unable to load usage activity'), { cause: error })
      }
    },
  })

  if (userId == null) return null

  return (
    <TitledCard
      title={t('Usage activity')}
      description={t('Your token activity over time')}
      icon={<Footprints className='size-4' aria-hidden='true' />}
      iconTone='info'
      disableHoverEffect
      className='min-w-0'
      action={
        <Button
          type='button'
          variant='outline'
          size='sm'
          disabled={activityQuery.isFetching}
          onClick={() => void activityQuery.refetch()}
          aria-label={t('Refresh usage activity')}
        >
          <RefreshCw
            className={
              activityQuery.isFetching ? 'size-4 animate-spin' : 'size-4'
            }
            aria-hidden='true'
          />
          {t('Refresh')}
        </Button>
      }
    >
      <div aria-busy={activityQuery.isFetching}>
        {activityQuery.isPending && (
          <div role='status' className='space-y-4'>
            <span className='sr-only'>{t('Loading usage activity')}</span>
            <Skeleton className='h-20 w-full' />
            <Skeleton className='h-48 w-full' />
          </div>
        )}
        {activityQuery.isError && (
          <div role='alert' className='flex flex-col items-start gap-3 py-4'>
            <p className='text-sm'>{t('Unable to load usage activity')}</p>
            <Button
              type='button'
              variant='outline'
              disabled={activityQuery.isFetching}
              onClick={() => void activityQuery.refetch()}
            >
              {t('Retry')}
            </Button>
          </div>
        )}
        {activityQuery.data && (
          <UsageActivityContent key={userId} activity={activityQuery.data} />
        )}
      </div>
    </TitledCard>
  )
}

function UsageActivityContent(props: { activity: UsageActivity }) {
  const { t, i18n } = useTranslation()
  const [selectedYear, setSelectedYear] = useState<number | null>(null)
  const today = props.activity.as_of.slice(0, 10)
  const currentYear = Number(today.slice(0, 4))
  const firstYear =
    Number(props.activity.daily_usage[0]?.date.slice(0, 4)) || currentYear
  const years = Array.from(
    { length: currentYear - firstYear + 1 },
    (_, index) => currentYear - index
  )
  const year =
    selectedYear != null && years.includes(selectedYear)
      ? selectedYear
      : currentYear
  const summary = props.activity.summary
  const locale = toIntlLocale(i18n.language)
  const numberFormat = new Intl.NumberFormat(locale)
  const compactFormat = new Intl.NumberFormat(locale, {
    notation: 'compact',
    maximumFractionDigits: 1,
  })
  const stats = [
    { label: t('Total tokens'), value: summary.total_tokens },
    { label: t('Peak daily tokens'), value: summary.peak_daily_tokens },
    {
      label: t('Current streak'),
      value: summary.current_streak_days,
      suffix: t('days'),
      detail: t('Longest streak: {{days}} days', {
        days: numberFormat.format(summary.longest_streak_days),
      }),
    },
    {
      label: t('Longest request'),
      value: summary.longest_request_seconds,
      suffix: t('seconds'),
    },
  ]

  return (
    <div className='@container/usage space-y-5'>
      <dl className='grid grid-cols-2 gap-x-4 gap-y-5 @xl/usage:grid-cols-4'>
        {stats.map((stat) => (
          <div key={stat.label} className='min-w-0'>
            <dt className='text-muted-foreground text-xs'>{stat.label}</dt>
            <dd
              className='mt-1 flex flex-wrap items-baseline gap-1 text-xl font-semibold [overflow-wrap:anywhere] tabular-nums'
              title={numberFormat.format(stat.value)}
              aria-label={[
                stat.label,
                numberFormat.format(stat.value),
                stat.suffix,
              ]
                .filter(Boolean)
                .join(': ')}
            >
              {compactFormat.format(stat.value)}
              {stat.suffix && (
                <span className='text-muted-foreground text-xs font-normal'>
                  {stat.suffix}
                </span>
              )}
            </dd>
            {stat.detail && (
              <p className='text-muted-foreground mt-1 text-xs [overflow-wrap:anywhere]'>
                {stat.detail}
              </p>
            )}
          </div>
        ))}
      </dl>
      {props.activity.daily_usage.length === 0 ? (
        <div className='rounded-lg border border-dashed px-4 py-10 text-center'>
          <p className='text-sm font-medium'>{t('No usage activity yet')}</p>
          <p className='text-muted-foreground mt-1 text-xs'>
            {t('Your activity will appear after you use the API.')}
          </p>
        </div>
      ) : (
        <>
          <div className='flex flex-wrap items-center justify-between gap-2'>
            <label className='flex items-center gap-2 text-sm'>
              {t('Activity year')}
              <select
                className='bg-background focus-visible:ring-ring rounded-md border px-2 py-1.5 text-sm focus-visible:ring-2'
                value={year}
                onChange={(event) =>
                  setSelectedYear(Number(event.target.value))
                }
              >
                {years.map((option) => (
                  <option key={option} value={option}>
                    {option}
                  </option>
                ))}
              </select>
            </label>
            <p className='text-muted-foreground text-xs'>
              {t('Time zone: {{timezone}}', {
                timezone: props.activity.timezone,
              })}
            </p>
          </div>
          <UsageActivityChart
            days={props.activity.daily_usage}
            year={year}
            today={today}
          />
        </>
      )}
      <p className='text-muted-foreground text-xs leading-relaxed'>
        {t(
          'Based on retained API usage logs. Includes input, cached input, and output tokens; deleted or unrecorded logs are not included.'
        )}
      </p>
    </div>
  )
}
