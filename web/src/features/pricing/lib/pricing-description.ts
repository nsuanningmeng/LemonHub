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
  MATCH_CONTAINS,
  MATCH_EQ,
  MATCH_EXISTS,
  MATCH_GTE,
  MATCH_LT,
  MATCH_RANGE,
  SOURCE_TIME,
  SOURCE_TOKEN,
  type ParsedTier,
  type RequestCondition,
  type RequestRuleGroup,
} from './billing-expr'

const TIME_FUNC_LABELS: Record<string, string> = {
  hour: 'Hour',
  minute: 'Minute',
  weekday: 'Weekday',
  month: 'Month',
  day: 'Day',
}

function describeCondition(
  cond: RequestCondition,
  t: (key: string) => string
): string {
  if (cond.source === SOURCE_TIME) {
    const fn = t(TIME_FUNC_LABELS[cond.timeFunc] || cond.timeFunc)
    const tz = cond.timezone || 'UTC'
    if (cond.mode === MATCH_RANGE) {
      const start =
        cond.timeFunc === 'hour'
          ? `${String(cond.rangeStart).padStart(2, '0')}:00`
          : cond.rangeStart
      const end =
        cond.timeFunc === 'hour'
          ? `${String(cond.rangeEnd).padStart(2, '0')}:00`
          : cond.rangeEnd
      if (Number(cond.rangeStart) > Number(cond.rangeEnd)) {
        return `(${fn} ≥ ${start} || ${fn} < ${end}) (${tz})`
      }
      return `${fn} [${start}, ${end}) (${tz})`
    }
    const opMap: Record<string, string> = {
      [MATCH_EQ]: '=',
      [MATCH_GTE]: '≥',
      [MATCH_LT]: '<',
    }
    return `${fn} ${opMap[cond.mode] || '='} ${cond.value} (${tz})`
  }
  const src = cond.source === 'header' ? t('Header') : t('Body param')
  const path = cond.path || ''
  if (cond.mode === MATCH_EXISTS) return `${src} ${path} ${t('Exists')}`
  if (cond.mode === MATCH_CONTAINS) {
    return `${src} ${path} ${t('Contains')} "${cond.value}"`
  }
  const opMap: Record<string, string> = {
    eq: '=',
    gt: '>',
    gte: '≥',
    lt: '<',
    lte: '≤',
  }
  if (cond.source === SOURCE_TOKEN) {
    const labels: Record<string, string> = {
      p: 'Billable input tokens',
      c: 'Billable output tokens',
      len: 'Full input length',
    }
    return `${t(labels[path] || path)} ${opMap[cond.mode] || '='} ${cond.value}`
  }
  return `${src} ${path} ${opMap[cond.mode] || '='} ${cond.value}`
}

export function formatTierCondition(
  tier: ParsedTier,
  t: (key: string) => string,
  showRawExpression = false
): string {
  if (tier.displayConditions?.length) {
    return tier.displayConditions
      .map((condition) => describeCondition(condition, t))
      .join(` ${t('and')} `)
  }
  if (showRawExpression && tier.conditionText) return tier.conditionText
  if (tier.conditions.length > 0) {
    const modes: Record<string, string> = {
      '<': 'lt',
      '<=': 'lte',
      '>': 'gt',
      '>=': 'gte',
    }
    return tier.conditions
      .map((condition) =>
        describeCondition(
          {
            source: SOURCE_TOKEN,
            path: condition.var,
            mode: modes[condition.op],
            value: String(condition.value),
          },
          t
        )
      )
      .join(` ${t('and')} `)
  }
  return tier.conditionText ? t('Dynamic Pricing') : ''
}

export function formatRequestRuleGroup(
  group: RequestRuleGroup,
  t: (key: string) => string,
  showRawExpression = false
): string {
  const description = (group.conditions || [])
    .map((condition) => describeCondition(condition, t))
    .join(` ${t('and')} `)
  if (description) return description
  if (showRawExpression) return group.conditionText || ''
  return t('Dynamic Pricing')
}
