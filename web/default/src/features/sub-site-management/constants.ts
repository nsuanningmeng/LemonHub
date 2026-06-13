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
import { type TFunction } from 'i18next'
import { type StatusBadgeProps } from '@/components/status-badge'

// ============================================================================
// Site Status Configuration
// ============================================================================

export const SITE_STATUS = {
  NORMAL: 1,
  DISABLED: 2,
} as const

export const SITE_STATUS_VALUES = Object.values(SITE_STATUS).map(
  (value) => String(value)
) as `${number}`[]

// labelKey values are i18n keys; use t(config.labelKey) in components
export const SITE_STATUSES: Record<
  number,
  Pick<StatusBadgeProps, 'variant'> & {
    labelKey: string
    value: number
  }
> = {
  [SITE_STATUS.NORMAL]: {
    labelKey: 'Normal',
    variant: 'success',
    value: SITE_STATUS.NORMAL,
  },
  [SITE_STATUS.DISABLED]: {
    labelKey: 'Disabled',
    variant: 'neutral',
    value: SITE_STATUS.DISABLED,
  },
} as const

export function getSiteStatusOptions(t: TFunction) {
  return Object.values(SITE_STATUSES).map((config) => ({
    label: t(config.labelKey),
    value: String(config.value),
  }))
}

// ============================================================================
// Validation Constants
// ============================================================================

export const SITE_VALIDATION = {
  NAME_MIN_LENGTH: 1,
  NAME_MAX_LENGTH: 100,
  DOMAIN_MIN_COUNT: 1,
} as const

// ============================================================================
// Success Messages (i18n keys)
// ============================================================================

export const SUCCESS_MESSAGES = {
  SITE_CREATED: 'Sub-site created successfully',
  SITE_UPDATED: 'Sub-site updated successfully',
  SITE_DELETED: 'Sub-site deleted successfully',
} as const
