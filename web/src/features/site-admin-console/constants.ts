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
import { type StatusBadgeProps } from '@/components/status-badge'

// ============================================================================
// Redemption Status Configuration (site-admin scope)
// status: 1 enabled, 2 used, 3 disabled
// ============================================================================

export const REDEMPTION_STATUS = {
  ENABLED: 1,
  USED: 2,
  DISABLED: 3,
} as const

export const REDEMPTION_STATUSES: Record<
  number,
  Pick<StatusBadgeProps, 'variant'> & { labelKey: string; value: number }
> = {
  [REDEMPTION_STATUS.ENABLED]: {
    labelKey: 'Unused',
    variant: 'success',
    value: REDEMPTION_STATUS.ENABLED,
  },
  [REDEMPTION_STATUS.USED]: {
    labelKey: 'Used',
    variant: 'neutral',
    value: REDEMPTION_STATUS.USED,
  },
  [REDEMPTION_STATUS.DISABLED]: {
    labelKey: 'Disabled',
    variant: 'neutral',
    value: REDEMPTION_STATUS.DISABLED,
  },
} as const

// ============================================================================
// Validation Constants
// ============================================================================

export const REDEMPTION_VALIDATION = {
  NAME_MIN_LENGTH: 1,
  NAME_MAX_LENGTH: 20,
  COUNT_MIN: 1,
  COUNT_MAX: 100,
} as const

// ============================================================================
// Success Messages (i18n keys)
// ============================================================================

export const SUCCESS_MESSAGES = {
  REDEMPTION_CREATED: 'Redemption code(s) created successfully',
  REDEMPTION_VOIDED: 'Redemption code voided successfully',
  WARN_THRESHOLD_UPDATED: 'Warn threshold updated successfully',
  BRANDING_UPDATED: 'Branding updated successfully',
  PAY_CONFIG_UPDATED: 'Payment config saved successfully',
} as const

// ============================================================================
// Helpers
// ============================================================================

/** True when a redemption is past its expiry and still in the enabled state. */
export function isRedemptionExpired(
  expiredTime: number,
  status: number
): boolean {
  if (status !== REDEMPTION_STATUS.ENABLED) return false
  if (!expiredTime || expiredTime <= 0) return false
  return expiredTime * 1000 < Date.now()
}
