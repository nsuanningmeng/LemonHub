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
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type PointerEvent,
} from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { toIntlLocale } from '@/i18n/languages'

import {
  getActivityDate,
  getUsageActivityDays,
  getUsageActivitySeries,
  type UsageActivityMode,
} from '../lib/usage-activity'
import type { UsageActivityDay } from '../types'
import { UsageActivityCalendar } from './usage-activity-calendar'

interface UsageActivityChartProps {
  days: UsageActivityDay[]
  year: number
  today: string
}

const DAY_MS = 86_400_000
const PLOT_LEFT = 72
const PLOT_TOP = 12
const PLOT_HEIGHT = 168

export function UsageActivityChart(props: UsageActivityChartProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.language)
  const [mode, setMode] = useState<UsageActivityMode>('daily')
  const [selectedDate, setSelectedDate] = useState<string | null>(null)
  const chart = useRef<HTMLDivElement>(null)
  const [chartWidth, setChartWidth] = useState(720)
  const plotWidth = Math.max(1, chartWidth - PLOT_LEFT - 12)
  const calendarDays = useMemo(
    () => getUsageActivityDays(props.days, props.year, props.today),
    [props.days, props.year, props.today]
  )
  const series = useMemo(
    () => getUsageActivitySeries(props.days, props.year, props.today, mode),
    [props.days, props.year, props.today, mode]
  )
  const hasSeries = series.length > 0
  useLayoutEffect(() => {
    const element = chart.current
    if (!element) return
    const width = element.getBoundingClientRect().width
    if (width > 0) setChartWidth(width)
    const observer = new ResizeObserver((entries) => {
      const nextWidth = entries[0]?.contentRect.width
      if (nextWidth && nextWidth > 0) setChartWidth(nextWidth)
    })
    observer.observe(element)
    return () => observer.disconnect()
  }, [hasSeries])
  const dateFormat = new Intl.DateTimeFormat(locale, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    timeZone: 'UTC',
  })
  const monthFormat = new Intl.DateTimeFormat(locale, {
    month: 'short',
    timeZone: 'UTC',
  })
  const numberFormat = new Intl.NumberFormat(locale)
  const compactFormat = new Intl.NumberFormat(locale, {
    notation: 'compact',
    maximumFractionDigits: 1,
  })
  const start = Date.UTC(props.year, 0, 1)
  const daysInYear = calendarDays.length
  const maximum = Math.max(1, ...series.map((point) => point.tokens))
  const matchingIndex = series.findIndex(
    (point) =>
      selectedDate !== null &&
      point.date <= selectedDate &&
      point.endDate >= selectedDate
  )
  const selectedIndex = matchingIndex >= 0 ? matchingIndex : series.length - 1
  const selected = series[selectedIndex]
  let selectedLabel = t('No activity in this period')
  if (selected) {
    let period = dateFormat.format(getActivityDate(selected.date))
    if (selected.endDate !== selected.date) {
      period += ` – ${dateFormat.format(getActivityDate(selected.endDate))}`
    }
    selectedLabel = t('{{date}}: {{tokens}} tokens', {
      date: period,
      tokens: numberFormat.format(selected.tokens),
    })
  }
  const plotted = series.map((point) => {
    const dayIndex = (getActivityDate(point.date).getTime() - start) / DAY_MS
    const duration =
      (getActivityDate(point.endDate).getTime() -
        getActivityDate(point.date).getTime()) /
        DAY_MS +
      1
    return {
      ...point,
      x: PLOT_LEFT + ((dayIndex + duration / 2) / daysInYear) * plotWidth,
      y: PLOT_TOP + PLOT_HEIGHT * (1 - point.tokens / maximum),
      width: Math.max(0.5, (duration / daysInYear) * plotWidth - 1),
    }
  })
  const selectedPoint = plotted[selectedIndex]
  const line = plotted.map((point) => `${point.x},${point.y}`).join(' ')
  const firstPoint = plotted[0]
  const lastPoint = plotted.at(-1)

  const inspectChartAtPointer = (event: PointerEvent<SVGSVGElement>): void => {
    const bounds = event.currentTarget.getBoundingClientRect()
    if (bounds.width === 0) return
    const position = ((event.clientX - bounds.left) / bounds.width) * chartWidth
    const dayIndex = Math.floor(
      ((position - PLOT_LEFT) / plotWidth) * daysInYear
    )
    const date =
      calendarDays[Math.max(0, Math.min(daysInYear - 1, dayIndex))]?.date
    if (date) setSelectedDate(date)
  }

  return (
    <div className='min-w-0 space-y-6'>
      <div className='space-y-3'>
        <div
          role='group'
          aria-label={t('Activity view')}
          className='bg-muted inline-flex max-w-full flex-wrap gap-1 rounded-lg p-1'
        >
          <Button
            type='button'
            size='sm'
            variant={mode === 'daily' ? 'outline' : 'ghost'}
            aria-pressed={mode === 'daily'}
            onClick={() => setMode('daily')}
          >
            {t('Daily')}
          </Button>
          <Button
            type='button'
            size='sm'
            variant={mode === 'weekly' ? 'outline' : 'ghost'}
            aria-pressed={mode === 'weekly'}
            onClick={() => setMode('weekly')}
          >
            {t('Weekly')}
          </Button>
          <Button
            type='button'
            size='sm'
            variant={mode === 'cumulative' ? 'outline' : 'ghost'}
            aria-pressed={mode === 'cumulative'}
            onClick={() => setMode('cumulative')}
          >
            {t('Cumulative')}
          </Button>
        </div>
        <output
          aria-label={t('Selected chart data')}
          className='text-muted-foreground block min-h-8 text-xs tabular-nums sm:min-h-4'
        >
          {selectedLabel}
        </output>
        {selected && (
          <div
            ref={chart}
            role='slider'
            tabIndex={0}
            aria-label={t('Explore chart data')}
            aria-description={t('Use arrow keys to explore dates.')}
            aria-valuemin={0}
            aria-valuemax={series.length - 1}
            aria-valuenow={selectedIndex}
            aria-valuetext={selectedLabel}
            className='focus-visible:ring-ring rounded-md outline-none focus-visible:ring-2'
            onKeyDown={(event) => {
              let nextIndex = selectedIndex
              if (event.key === 'ArrowLeft' || event.key === 'ArrowDown') {
                nextIndex -= 1
              } else if (
                event.key === 'ArrowRight' ||
                event.key === 'ArrowUp'
              ) {
                nextIndex += 1
              } else if (event.key === 'Home') nextIndex = 0
              else if (event.key === 'End') nextIndex = series.length - 1
              else return
              event.preventDefault()
              setSelectedDate(
                series[Math.max(0, Math.min(series.length - 1, nextIndex))].date
              )
            }}
          >
            <svg
              viewBox={`0 0 ${chartWidth} 216`}
              aria-hidden='true'
              className='block h-54 w-full overflow-visible'
              onPointerDown={inspectChartAtPointer}
              onPointerMove={inspectChartAtPointer}
            >
              {[0, 0.5, 1].map((fraction) => (
                <g key={fraction}>
                  <line
                    x1={PLOT_LEFT}
                    x2={PLOT_LEFT + plotWidth}
                    y1={PLOT_TOP + fraction * PLOT_HEIGHT}
                    y2={PLOT_TOP + fraction * PLOT_HEIGHT}
                    className='stroke-border'
                    strokeDasharray='3 4'
                  />
                  <text
                    x={PLOT_LEFT - 8}
                    y={PLOT_TOP + fraction * PLOT_HEIGHT + 4}
                    textAnchor='end'
                    fontSize={11}
                    className='fill-muted-foreground'
                  >
                    {compactFormat.format(maximum * (1 - fraction))}
                  </text>
                </g>
              ))}
              {mode === 'cumulative' && firstPoint && lastPoint ? (
                <g className='text-teal-600 dark:text-teal-400'>
                  <polygon
                    points={`${firstPoint.x},${PLOT_TOP + PLOT_HEIGHT} ${line} ${lastPoint.x},${PLOT_TOP + PLOT_HEIGHT}`}
                    fill='currentColor'
                    fillOpacity={0.12}
                  />
                  <polyline
                    points={line}
                    fill='none'
                    stroke='currentColor'
                    strokeWidth={2}
                    strokeLinejoin='round'
                  />
                </g>
              ) : (
                plotted.map((point) => (
                  <rect
                    key={point.date}
                    x={point.x - point.width / 2}
                    y={point.y}
                    width={point.width}
                    height={PLOT_TOP + PLOT_HEIGHT - point.y}
                    rx={0.5}
                    className='fill-teal-600 dark:fill-teal-400'
                  />
                ))
              )}
              {selectedPoint && (
                <g className='text-teal-800 dark:text-teal-200'>
                  <line
                    x1={selectedPoint.x}
                    x2={selectedPoint.x}
                    y1={PLOT_TOP}
                    y2={PLOT_TOP + PLOT_HEIGHT}
                    stroke='currentColor'
                    strokeOpacity={0.5}
                    strokeDasharray='3 3'
                  />
                  <circle
                    cx={selectedPoint.x}
                    cy={selectedPoint.y}
                    r={3}
                    fill='currentColor'
                  />
                </g>
              )}
              {calendarDays
                .filter(
                  (day) =>
                    day.date.endsWith('-01') &&
                    Number(day.date.slice(5, 7)) % 2 === 1
                )
                .map((day) => (
                  <text
                    key={day.date}
                    x={
                      PLOT_LEFT +
                      ((getActivityDate(day.date).getTime() - start) /
                        DAY_MS /
                        daysInYear) *
                        plotWidth
                    }
                    y={204}
                    fontSize={11}
                    className='fill-muted-foreground'
                  >
                    {monthFormat.format(getActivityDate(day.date))}
                  </text>
                ))}
            </svg>
          </div>
        )}
      </div>
      <UsageActivityCalendar
        days={calendarDays}
        year={props.year}
        today={props.today}
      />
    </div>
  )
}
