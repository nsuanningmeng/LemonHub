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
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { toIntlLocale } from '@/i18n/languages'
import { cn } from '@/lib/utils'

import { getActivityDate, getMondayOffset } from '../lib/usage-activity'
import type { UsageActivityDay } from '../types'

interface UsageActivityCalendarProps {
  days: UsageActivityDay[]
  year: number
  today: string
}

const HEAT_COLORS = [
  'bg-muted',
  'bg-teal-200 dark:bg-teal-900',
  'bg-teal-400 dark:bg-teal-700',
  'bg-teal-600 dark:bg-teal-500',
  'bg-teal-800 dark:bg-teal-300',
]

export function UsageActivityCalendar(props: UsageActivityCalendarProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.language)
  const [selectedDate, setSelectedDate] = useState<string | null>(null)
  const [focusedDate, setFocusedDate] = useState<string | null>(null)
  const buttons = useRef(new Map<string, HTMLButtonElement>())
  const availableDays = props.days.filter((day) => day.date <= props.today)
  const selected =
    availableDays.find((day) => day.date === selectedDate) ??
    availableDays.at(-1)
  const tabStopDate = availableDays.some((day) => day.date === focusedDate)
    ? focusedDate
    : availableDays.at(-1)?.date
  const maximum = Math.max(1, ...availableDays.map((day) => day.tokens))
  const offset = getMondayOffset(`${props.year}-01-01`)
  const columns = Math.ceil((offset + props.days.length) / 7)
  const dateFormat = new Intl.DateTimeFormat(locale, {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    timeZone: 'UTC',
  })
  const monthFormat = new Intl.DateTimeFormat(locale, {
    month: 'short',
    timeZone: 'UTC',
  })
  const weekdayFormat = new Intl.DateTimeFormat(locale, {
    weekday: 'narrow',
    timeZone: 'UTC',
  })
  const numberFormat = new Intl.NumberFormat(locale)

  return (
    <div className='space-y-3'>
      <div className='flex flex-wrap items-baseline justify-between gap-2'>
        <h3 className='text-sm font-medium'>{t('Activity calendar')}</h3>
        <span className='text-muted-foreground text-xs'>{props.year}</span>
      </div>
      <div
        role='group'
        aria-label={t('Activity calendar')}
        className='overflow-x-auto pb-2'
      >
        <div className='grid min-w-[620px] grid-cols-[1.5rem_1fr] gap-x-2 gap-y-1'>
          <div aria-hidden='true' />
          <div
            aria-hidden='true'
            className='text-muted-foreground grid h-5 gap-1 text-[10px]'
            style={{
              gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`,
            }}
          >
            {props.days.map(
              (day, index) =>
                day.date.endsWith('-01') && (
                  <span
                    key={day.date}
                    className='whitespace-nowrap'
                    style={{ gridColumn: Math.floor((index + offset) / 7) + 1 }}
                  >
                    {monthFormat.format(getActivityDate(day.date))}
                  </span>
                )
            )}
          </div>
          <div
            aria-hidden='true'
            className='text-muted-foreground grid grid-rows-7 items-center gap-1 text-center text-[9px]'
          >
            {[1, 2, 3, 4, 5, 6, 7].map((day) => (
              <span key={day}>
                {weekdayFormat.format(new Date(Date.UTC(2024, 0, day)))}
              </span>
            ))}
          </div>
          <div
            className='grid gap-1'
            style={{
              gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`,
              gridTemplateRows: 'repeat(7, minmax(0, 1fr))',
            }}
          >
            {props.days.map((day, index) => {
              const position = {
                gridColumn: Math.floor((index + offset) / 7) + 1,
                gridRow: ((index + offset) % 7) + 1,
              }
              if (day.date > props.today) {
                return (
                  <span
                    key={day.date}
                    aria-hidden='true'
                    className='border-border/50 aspect-square rounded-[2px] border border-dashed'
                    style={position}
                  />
                )
              }
              const level =
                day.tokens === 0
                  ? 0
                  : Math.max(1, Math.ceil((day.tokens / maximum) * 4))
              const label = t('{{date}}: {{tokens}} tokens', {
                date: dateFormat.format(getActivityDate(day.date)),
                tokens: numberFormat.format(day.tokens),
              })
              return (
                <button
                  key={day.date}
                  type='button'
                  ref={(element) => {
                    if (element) buttons.current.set(day.date, element)
                    else buttons.current.delete(day.date)
                  }}
                  aria-label={label}
                  title={label}
                  tabIndex={tabStopDate === day.date ? 0 : -1}
                  className={cn(
                    'ring-ring aspect-square cursor-pointer rounded-[2px] outline-none hover:ring-2 focus-visible:ring-2',
                    HEAT_COLORS[level]
                  )}
                  style={position}
                  onFocus={() => {
                    setSelectedDate(day.date)
                    setFocusedDate(day.date)
                  }}
                  onPointerEnter={() => setSelectedDate(day.date)}
                  onClick={() => {
                    setSelectedDate(day.date)
                    setFocusedDate(day.date)
                  }}
                  onKeyDown={(event) => {
                    let nextIndex = index
                    if (event.key === 'ArrowLeft') nextIndex -= 7
                    else if (event.key === 'ArrowRight') nextIndex += 7
                    else if (event.key === 'ArrowUp') nextIndex -= 1
                    else if (event.key === 'ArrowDown') nextIndex += 1
                    else if (event.key === 'Home') nextIndex = 0
                    else if (event.key === 'End') {
                      nextIndex = availableDays.length - 1
                    } else return
                    event.preventDefault()
                    const nextDay =
                      availableDays[
                        Math.max(
                          0,
                          Math.min(availableDays.length - 1, nextIndex)
                        )
                      ]
                    if (nextDay) buttons.current.get(nextDay.date)?.focus()
                  }}
                />
              )
            })}
          </div>
        </div>
      </div>
      <div className='flex flex-wrap items-center justify-between gap-2 text-xs'>
        <output
          aria-label={t('Selected day')}
          className='text-muted-foreground min-h-4 tabular-nums'
        >
          {selected
            ? t('{{date}}: {{tokens}} tokens', {
                date: dateFormat.format(getActivityDate(selected.date)),
                tokens: numberFormat.format(selected.tokens),
              })
            : t('No activity in this period')}
        </output>
        <div className='text-muted-foreground flex items-center gap-1.5'>
          <span>{t('Less')}</span>
          {HEAT_COLORS.map((color) => (
            <span
              key={color}
              aria-hidden='true'
              className={cn('size-2.5 rounded-[2px]', color)}
            />
          ))}
          <span>{t('More')}</span>
        </div>
      </div>
    </div>
  )
}
