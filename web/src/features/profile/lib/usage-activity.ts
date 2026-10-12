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
import type { UsageActivityDay } from '../types'

export type UsageActivityMode = 'daily' | 'weekly' | 'cumulative'

export type UsageActivityPoint = {
  date: string
  endDate: string
  tokens: number
}

const DAY_MILLISECONDS = 24 * 60 * 60 * 1000

export function getActivityDate(date: string): Date {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date)) return new Date(Number.NaN)

  const parsed = new Date(`${date}T00:00:00.000Z`)
  if (!Number.isFinite(parsed.getTime())) return parsed
  if (parsed.toISOString().slice(0, 10) !== date) return new Date(Number.NaN)
  return parsed
}

export function getMondayOffset(date: string): number {
  return (getActivityDate(date).getUTCDay() + 6) % 7
}

function aggregateUsageActivityDays(
  days: UsageActivityDay[],
  today: string
): Map<string, number> {
  const totals = new Map<string, number>()
  for (const day of days) {
    if (
      !Number.isFinite(day.tokens) ||
      day.tokens < 0 ||
      !Number.isFinite(getActivityDate(day.date).getTime()) ||
      day.date > today
    ) {
      continue
    }
    totals.set(day.date, (totals.get(day.date) ?? 0) + day.tokens)
  }
  return totals
}

export function getUsageActivityDays(
  days: UsageActivityDay[],
  year: number,
  today: string
): UsageActivityDay[] {
  if (
    !Number.isInteger(year) ||
    year < 0 ||
    year > 9999 ||
    !Number.isFinite(getActivityDate(today).getTime())
  ) {
    return []
  }

  const yearText = String(year).padStart(4, '0')
  const firstDay = getActivityDate(`${yearText}-01-01`).getTime()
  const lastDay = getActivityDate(`${yearText}-12-31`).getTime()
  const totals = aggregateUsageActivityDays(days, today)
  const calendar: UsageActivityDay[] = []

  for (
    let timestamp = firstDay;
    timestamp <= lastDay;
    timestamp += DAY_MILLISECONDS
  ) {
    const date = new Date(timestamp).toISOString().slice(0, 10)
    calendar.push({ date, tokens: totals.get(date) ?? 0 })
  }
  return calendar
}

export function getUsageActivitySeries(
  days: UsageActivityDay[],
  year: number,
  today: string,
  mode: UsageActivityMode
): UsageActivityPoint[] {
  const calendar = getUsageActivityDays(days, year, today).filter(
    (day) => day.date <= today
  )
  if (calendar.length === 0) return []

  if (mode === 'weekly') {
    const weeks: UsageActivityPoint[] = []
    for (const day of calendar) {
      const previous = weeks.at(-1)
      if (!previous || getMondayOffset(day.date) === 0) {
        weeks.push({ date: day.date, endDate: day.date, tokens: day.tokens })
        continue
      }
      previous.endDate = day.date
      previous.tokens += day.tokens
    }
    return weeks
  }

  let cumulative = 0
  if (mode === 'cumulative') {
    const totals = aggregateUsageActivityDays(days, today)
    for (const [date, tokens] of totals) {
      if (date < calendar[0].date) cumulative += tokens
    }
  }

  return calendar.map((day) => {
    cumulative += day.tokens
    return {
      date: day.date,
      endDate: day.date,
      tokens: mode === 'cumulative' ? cumulative : day.tokens,
    }
  })
}
