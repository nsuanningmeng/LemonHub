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
import type { TFunction } from 'i18next'

import type { StatusVariant } from '@/components/status-badge'

// ============================================================================
// Wallet Log Types
// ============================================================================

/**
 * Wallet ledger entry. All money fields are stored in 厘 (int64, 0.001 CNY).
 * `amount` is signed (positive = credit, negative = debit).
 */
export type WalletLog = {
  id: number
  site_id: number
  type: number
  amount: number // 厘, signed
  balance_after: number // 厘
  related_id: number
  remark: string
  operator_user_id: number
  created_time: number
}

export const WALLET_LOG_TYPE = {
  RECHARGE: 1,
  GENERATE_DEDUCTION: 2,
  VOID_REFUND: 3,
  RECHARGE_DEDUCTION: 4,
  MANUAL_ADJUST: 5,
} as const

export const WALLET_LOG_TYPES: Record<
  number,
  { labelKey: string; variant: StatusVariant }
> = {
  [WALLET_LOG_TYPE.RECHARGE]: { labelKey: 'Recharge', variant: 'success' },
  [WALLET_LOG_TYPE.GENERATE_DEDUCTION]: {
    labelKey: 'Generation Deduction',
    variant: 'warning',
  },
  [WALLET_LOG_TYPE.VOID_REFUND]: { labelKey: 'Void Refund', variant: 'info' },
  [WALLET_LOG_TYPE.RECHARGE_DEDUCTION]: {
    labelKey: 'Goods Deduction',
    variant: 'neutral',
  },
  [WALLET_LOG_TYPE.MANUAL_ADJUST]: {
    labelKey: 'Manual Adjustment',
    variant: 'purple',
  },
}

export function getWalletLogTypeOptions(t: TFunction) {
  return Object.entries(WALLET_LOG_TYPES).map(([value, cfg]) => ({
    value,
    label: t(cfg.labelKey),
  }))
}

/** Convert 厘 (0.001 CNY) to a 元 display string with 3 decimals. */
export function formatMilliYuan(milli: number): string {
  return (milli / 1000).toFixed(3)
}
