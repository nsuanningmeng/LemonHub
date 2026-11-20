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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber, formatQuota } from '@/lib/format'

type ActivitySeries = 'usage' | 'requests'

interface WorkspaceActivityChartProps {
  series: Record<ActivitySeries, number[]>
  start: number
  end: number
  loading: boolean
  failed: boolean
  retry: () => void
}

export function WorkspaceActivityChart(props: WorkspaceActivityChartProps) {
  const { t, i18n } = useTranslation()
  const [selected, setSelected] = useState<ActivitySeries>('usage')
  const values = props.series[selected]
  const formatValue = selected === 'usage' ? formatQuota : formatNumber
  const total = values.reduce((sum, value) => sum + value, 0)
  const max = Math.max(...values, 0)
  const hasActivity =
    props.series.usage.some(Boolean) || props.series.requests.some(Boolean)
  const time = new Intl.DateTimeFormat(
    toIntlLocale(i18n.resolvedLanguage || i18n.language),
    { hour: '2-digit', minute: '2-digit' }
  )
  const bucketWidth = 600 / Math.max(1, values.length)
  const bucketSeconds = (props.end - props.start) / Math.max(1, values.length)
  const points = values.map((value, index) => {
    const from = time.format(1000 * (props.start + index * bucketSeconds))
    const to = time.format(1000 * (props.start + (index + 1) * bucketSeconds))
    return {
      x: (index + 0.5) * bucketWidth,
      y: 99 - (max ? value / max : 0) * 90,
      value,
      time: `${from}–${to}`,
    }
  })
  const line = points.map((p, i) => `${i ? 'L' : 'M'} ${p.x} ${p.y}`).join(' ')

  return (
    <div className='min-w-0 py-4 sm:py-5'>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div>
          <h3 className='text-base font-semibold'>{t('Workspace activity')}</h3>
          <p className='text-muted-foreground mt-0.5 text-xs'>
            {t('Last 24 hours')}
          </p>
        </div>
        <div
          className='bg-muted inline-flex rounded-md p-1'
          role='group'
          aria-label={t('Chart metric')}
        >
          {(['usage', 'requests'] as const).map((type) => (
            <button
              type='button'
              key={type}
              aria-pressed={type === selected}
              onClick={() => setSelected(type)}
              className='text-muted-foreground hover:text-foreground aria-pressed:bg-card aria-pressed:text-foreground focus-visible:ring-ring min-h-7 rounded px-3 text-xs font-medium outline-none focus-visible:ring-2'
            >
              {type === 'usage' ? t('Usage') : t('Requests')}
            </button>
          ))}
        </div>
      </div>
      {props.loading && (
        <Skeleton aria-label={t('Loading')} className='my-6 h-36 w-full' />
      )}
      {!props.loading && props.failed && (
        <div className='my-6 flex min-h-36 items-center justify-center gap-4'>
          <span role='alert'>{t('Failed to load')}</span>
          <Button variant='outline' size='sm' onClick={props.retry}>
            {t('Retry')}
          </Button>
        </div>
      )}
      {!props.loading && !props.failed && !hasActivity && (
        <p className='text-muted-foreground my-3 flex min-h-36 items-center justify-center text-sm'>
          {t('No activity in this period')}
        </p>
      )}
      {!props.loading && !props.failed && hasActivity && (
        <figure className='mt-3 min-w-0'>
          <figcaption className='text-3xl font-semibold tracking-tight tabular-nums sm:text-4xl'>
            {formatValue(total)}
          </figcaption>
          <div className='mt-4 grid grid-cols-[auto_minmax(0,1fr)] gap-x-2 text-xs'>
            <div
              aria-hidden='true'
              className='text-muted-foreground flex flex-col justify-between pb-1 text-end text-[11px] tabular-nums'
            >
              <span>{max > 0 && formatValue(max)}</span>
              <span>0</span>
            </div>
            <svg
              role='img'
              aria-label={
                selected === 'usage'
                  ? t('Usage by time of day')
                  : t('Requests by time of day')
              }
              viewBox='0 0 600 102'
              preserveAspectRatio='none'
              className='h-24 w-full overflow-visible sm:h-32'
            >
              {[9, 54, 99].map((y) => (
                <line
                  key={y}
                  x1='0'
                  x2='600'
                  y1={y}
                  y2={y}
                  stroke='var(--border)'
                  strokeDasharray={y === 99 ? undefined : '3 4'}
                  vectorEffect='non-scaling-stroke'
                />
              ))}
              <path
                d={`${line} L ${points.at(-1)?.x ?? 600} 99 L ${points[0]?.x ?? 0} 99 Z`}
                fill='var(--brand-citrus)'
                opacity='.48'
              />
              <path
                d={line}
                fill='none'
                stroke='var(--foreground)'
                strokeWidth='2.25'
                strokeLinecap='round'
                strokeLinejoin='round'
                vectorEffect='non-scaling-stroke'
              />
              {points.map((point) => (
                <rect
                  key={point.x}
                  x={point.x - bucketWidth / 2}
                  y={0}
                  width={bucketWidth}
                  height={102}
                  fill='transparent'
                >
                  <title>
                    {point.time} {formatValue(point.value)}
                  </title>
                </rect>
              ))}
            </svg>
            <div
              className='text-muted-foreground col-start-2 mt-2 flex justify-between text-[11px] tabular-nums'
              aria-hidden='true'
            >
              {[0, 0.25, 0.5, 0.75, 1].map((fraction) => (
                <span key={fraction}>
                  {time.format(
                    1000 * (props.start + (props.end - props.start) * fraction)
                  )}
                </span>
              ))}
            </div>
          </div>
          <dl className='sr-only'>
            {points.map((point) => (
              <div key={point.x}>
                <dt>{point.time}</dt>
                <dd>{formatValue(point.value)}</dd>
              </div>
            ))}
          </dl>
        </figure>
      )}
    </div>
  )
}
