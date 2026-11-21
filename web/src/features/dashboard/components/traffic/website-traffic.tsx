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
import { Eye, RefreshCw, UserPlus, Users } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { getWebsiteTraffic } from '@/features/dashboard/api'
import { toIntlLocale } from '@/i18n/languages'

import { PanelWrapper } from '../ui/panel-wrapper'
import { StatCard } from '../ui/stat-card'

export default function WebsiteTraffic() {
  const { t, i18n } = useTranslation()
  const [days, setDays] = useState(7)
  const query = useQuery({
    queryKey: ['dashboard', 'site-analytics', days],
    queryFn: () => getWebsiteTraffic(days),
    staleTime: 60_000,
    retry: false,
  })
  const metrics = [
    {
      key: 'page_views',
      title: t('Page views'),
      description: t('Page loads and navigations on this site'),
      icon: Eye,
      tone: 'accent-1',
    },
    {
      key: 'unique_visitors',
      title: t('Unique visitors'),
      description: t('Browsers counted once in the selected period'),
      icon: Users,
      tone: 'accent-2',
    },
    {
      key: 'new_visitors',
      title: t('New visitors'),
      description: t('Browsers first seen during the selected period'),
      icon: UserPlus,
      tone: 'accent-3',
    },
  ] as const
  const rangeOptions = [
    { value: 1, label: t('Today') },
    { value: 7, label: t('7 Days') },
    { value: 30, label: t('30 Days') },
  ]
  const report = query.data
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const formatCount = new Intl.NumberFormat(locale).format

  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <div role='group' aria-label={t('Date Range')} className='flex gap-1'>
          {rangeOptions.map((option) => (
            <Button
              key={option.value}
              size='sm'
              variant={days === option.value ? 'secondary' : 'ghost'}
              aria-pressed={days === option.value}
              onClick={() => setDays(option.value)}
            >
              {option.label}
            </Button>
          ))}
        </div>
        <Button
          variant='outline'
          size='sm'
          disabled={query.isFetching}
          onClick={() => void query.refetch()}
        >
          <RefreshCw aria-hidden='true' />
          {query.isError ? t('Retry') : t('Refresh')}
        </Button>
      </div>

      {query.isPending && (
        <div role='status' aria-label={t('Loading')}>
          <Skeleton className='h-36 w-full' />
        </div>
      )}
      {query.isError && (
        <p
          role='alert'
          className='text-destructive rounded-xl border p-4 text-sm'
        >
          {t('Failed to load website traffic')}
        </p>
      )}
      {report && !query.isError && (
        <>
          <div className='@container/summary grid gap-3 sm:grid-cols-3'>
            {metrics.map((metric) => (
              <div
                key={metric.key}
                role='group'
                aria-label={metric.title}
                className='bg-card rounded-xl border p-4 shadow-xs'
              >
                <StatCard
                  title={metric.title}
                  description={metric.description}
                  icon={metric.icon}
                  value={formatCount(report.summary[metric.key])}
                  tone={metric.tone}
                  sparkline={report.daily.map((day) => day[metric.key])}
                  sparklineVariant='line'
                />
              </div>
            ))}
          </div>
          <dl className='flex flex-wrap gap-x-6 gap-y-2 px-1 text-sm'>
            <div className='flex items-baseline gap-2'>
              <dt
                className='text-muted-foreground'
                title={t(
                  'Browsers that visited before this period and returned during it'
                )}
              >
                {t('Returning visitors')}
              </dt>
              <dd className='font-medium tabular-nums'>
                {formatCount(
                  report.summary.unique_visitors - report.summary.new_visitors
                )}
              </dd>
            </div>
            <div className='flex items-baseline gap-2'>
              <dt className='text-muted-foreground'>
                {t('Page views per visitor')}
              </dt>
              <dd className='font-medium tabular-nums'>
                {report.summary.unique_visitors === 0
                  ? '—'
                  : (
                      report.summary.page_views / report.summary.unique_visitors
                    ).toLocaleString(locale, {
                      maximumFractionDigits: 1,
                    })}
              </dd>
            </div>
          </dl>
          <PanelWrapper
            title={t('Daily traffic (UTC)')}
            empty={report.summary.page_views === 0}
            emptyMessage={t('No page views recorded in this period.')}
          >
            <Table aria-label={t('Daily traffic (UTC)')}>
              <TableHeader>
                <TableRow>
                  <TableHead scope='col' className='pl-4'>
                    {t('Date (UTC)')}
                  </TableHead>
                  {metrics.map((metric) => (
                    <TableHead
                      key={metric.key}
                      scope='col'
                      className='text-right last:pr-4'
                    >
                      {metric.title}
                    </TableHead>
                  ))}
                </TableRow>
              </TableHeader>
              <TableBody className='[&>tr]:h-10'>
                {[...report.daily].reverse().map((day) => (
                  <TableRow key={day.date}>
                    <TableHead scope='row' className='pl-4 font-normal'>
                      <time dateTime={day.date}>{day.date}</time>
                    </TableHead>
                    {metrics.map((metric) => (
                      <TableCell
                        key={metric.key}
                        className='text-right last:pr-4'
                      >
                        {formatCount(day[metric.key])}
                      </TableCell>
                    ))}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </PanelWrapper>
        </>
      )}
    </div>
  )
}
