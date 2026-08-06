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

/*
 * Centralized success-rate display logic for model performance metrics.
 *
 * Color thresholds and the "no data => 100%" behavior are configurable by the
 * admin (System Settings -> Operations -> Performance). The values are surfaced
 * to the browser through GET /api/status and read here via `useSuccessRateConfig`.
 *
 * Semantics (matches the backend defaults):
 *   rate >= green  -> green  (healthy)
 *   rate >= yellow -> yellow (warning)
 *   rate <  yellow -> red    (critical)
 * When a model has no requests in the window the rate is non-finite; if
 * `noDataAsFull` is enabled it is treated as 100% (green) instead of "no data".
 */
import { useMemo } from 'react'
import { useStatus } from '@/hooks/use-status'

export type SuccessRateConfig = {
  /** Success rate >= this renders green. */
  green: number
  /** Success rate >= this (but below green) renders yellow; below renders red. */
  yellow: number
  /** When true, a model with no requests is treated as 100% (green). */
  noDataAsFull: boolean
}

export const DEFAULT_SUCCESS_RATE_CONFIG: SuccessRateConfig = {
  green: 99.9,
  yellow: 99,
  noDataAsFull: true,
}

export type SuccessRateTone = 'success' | 'warning' | 'destructive' | 'muted'

/** Read the admin-configured success-rate display config from /api/status. */
export function useSuccessRateConfig(): SuccessRateConfig {
  const { status } = useStatus()
  return useMemo(() => {
    const green = Number(status?.perf_success_rate_green_threshold)
    const yellow = Number(status?.perf_success_rate_yellow_threshold)
    const noDataAsFull = status?.perf_no_data_as_full
    return {
      green:
        Number.isFinite(green) && green > 0
          ? green
          : DEFAULT_SUCCESS_RATE_CONFIG.green,
      yellow:
        Number.isFinite(yellow) && yellow > 0
          ? yellow
          : DEFAULT_SUCCESS_RATE_CONFIG.yellow,
      noDataAsFull:
        typeof noDataAsFull === 'boolean'
          ? noDataAsFull
          : DEFAULT_SUCCESS_RATE_CONFIG.noDataAsFull,
    }
  }, [status])
}

/**
 * Resolve a possibly-missing success rate. When there is no data and
 * `noDataAsFull` is enabled, returns 100; otherwise returns NaN so callers can
 * keep rendering a neutral placeholder.
 */
export function resolveSuccessRate(
  rate: number,
  config: SuccessRateConfig
): number {
  if (Number.isFinite(rate)) return rate
  return config.noDataAsFull ? 100 : NaN
}

export function successRateTone(
  rate: number,
  config: SuccessRateConfig
): SuccessRateTone {
  const resolved = resolveSuccessRate(rate, config)
  if (!Number.isFinite(resolved)) return 'muted'
  if (resolved >= config.green) return 'success'
  if (resolved >= config.yellow) return 'warning'
  return 'destructive'
}

// --- Semantic design-token class maps (dashboard panels) --------------------

const TEXT_TOKEN: Record<SuccessRateTone, string> = {
  success: 'text-success',
  warning: 'text-warning',
  destructive: 'text-destructive',
  muted: 'text-muted-foreground',
}

const DOT_TOKEN: Record<SuccessRateTone, string> = {
  success: 'bg-success',
  warning: 'bg-warning',
  destructive: 'bg-destructive',
  muted: 'bg-muted-foreground',
}

export function successRateTextClass(
  rate: number,
  config: SuccessRateConfig
): string {
  return TEXT_TOKEN[successRateTone(rate, config)]
}

export function successRateDotClass(
  rate: number,
  config: SuccessRateConfig
): string {
  return DOT_TOKEN[successRateTone(rate, config)]
}

// --- Raw Tailwind class maps (pricing / model-square components) -------------

const SOLID_DOT: Record<SuccessRateTone, string> = {
  success: 'bg-emerald-500',
  warning: 'bg-amber-500',
  destructive: 'bg-rose-500',
  muted: 'bg-muted-foreground/40',
}

const TONE_TEXT: Record<SuccessRateTone, string> = {
  success: 'text-emerald-600 dark:text-emerald-400',
  warning: 'text-amber-600 dark:text-amber-400',
  destructive: 'text-rose-600 dark:text-rose-400',
  muted: 'text-muted-foreground',
}

export function successRateSolidDotClass(
  rate: number,
  config: SuccessRateConfig
): string {
  return SOLID_DOT[successRateTone(rate, config)]
}

export function successRateToneTextClass(
  rate: number,
  config: SuccessRateConfig
): string {
  return TONE_TEXT[successRateTone(rate, config)]
}

/** Intent for the StatCard component (model details). */
export function successRateIntent(
  rate: number,
  config: SuccessRateConfig
): 'default' | 'warning' | 'success' | 'destructive' {
  const tone = successRateTone(rate, config)
  if (tone === 'muted') return 'default'
  return tone
}

/** Format a success rate as a percentage, applying the no-data resolution. */
export function formatSuccessRate(
  rate: number,
  config: SuccessRateConfig
): string {
  const resolved = resolveSuccessRate(rate, config)
  if (!Number.isFinite(resolved)) return '—'
  return `${resolved.toFixed(2)}%`
}
