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
import { z } from 'zod'
import type { TFunction } from 'i18next'

// ============================================================================
// Wallet Form Schemas
//
// Inputs are entered in 元 (CNY). Convert to 厘 (int64) before submitting:
//   厘 = Math.round(元 * 1000)
// ============================================================================

export function getRechargeFormSchema(t: TFunction) {
  return z.object({
    amount_yuan: z
      .number()
      .gt(0, t('Amount must be greater than 0')),
    remark: z.string().optional(),
  })
}

export type RechargeFormValues = {
  amount_yuan: number
  remark?: string
}

export const RECHARGE_FORM_DEFAULT_VALUES: RechargeFormValues = {
  amount_yuan: 0,
  remark: '',
}

export function getAdjustFormSchema(t: TFunction) {
  return z.object({
    amount_yuan: z
      .number()
      .refine((v) => v !== 0, t('Amount cannot be zero')),
    remark: z.string().min(1, t('Remark is required')),
  })
}

export type AdjustFormValues = {
  amount_yuan: number
  remark: string
}

export const ADJUST_FORM_DEFAULT_VALUES: AdjustFormValues = {
  amount_yuan: 0,
  remark: '',
}

/** Convert a 元 amount to 厘 (int64). */
export function yuanToMilli(yuan: number): number {
  return Math.round(yuan * 1000)
}
