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
import { type WalletLog } from '@/components/wallet-logs-table'

export { type WalletLog }

// ============================================================================
// API Envelope
// ============================================================================

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export interface PageInfo<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

// ============================================================================
// Dashboard
// ============================================================================

export interface SiteAdminDashboard {
  id: number
  name: string
  logo: string
  notice: string
  footer: string
  home_badge: string
  home_title_line1: string
  home_title_line2: string
  status: number // 1 normal, 2 disabled
  discount_rate: number // basis of 10000
  wallet_balance: number // 厘
  wallet_warn_threshold: number // 厘
  model_price_rate: number // basis of 10000; per-call model price markup (>= 10000 = main retail)
  model_price_rate_max: number // basis of 10000; admin cap (0 = no cap)
  domains: string[]
}

// ============================================================================
// Redemption (site-admin scope)
// ============================================================================

export const redemptionSchema = z.object({
  id: z.number(),
  site_id: z.number(),
  user_id: z.number(),
  key: z.string(),
  status: z.number(), // 1 enabled, 2 used, 3 disabled
  name: z.string(),
  quota: z.number(),
  cost_amount: z.number(), // 厘
  created_time: z.number(),
  redeemed_time: z.number(),
  used_user_id: z.number(),
  expired_time: z.number(), // 0 = never
})

export type Redemption = z.infer<typeof redemptionSchema>

export interface GetRedemptionsParams {
  p?: number
  page_size?: number
}

export interface SearchRedemptionsParams {
  keyword?: string
  p?: number
  page_size?: number
}

export interface GetRedemptionsResponse {
  success: boolean
  message?: string
  data?: PageInfo<Redemption>
}

export interface CreateRedemptionPayload {
  name: string
  quota: number
  count: number
  expired_time: number // Unix seconds, 0 = never
}

// ============================================================================
// Wallet
// ============================================================================

export interface GetWalletLogsParams {
  p?: number
  page_size?: number
  type?: number
}

export interface GetWalletLogsResponse {
  success: boolean
  message?: string
  data?: PageInfo<WalletLog>
}

// ============================================================================
// Branding
// ============================================================================

export interface BrandingPayload {
  name: string
  logo: string
  notice: string
  footer: string
  home_badge: string
  home_title_line1: string
  home_title_line2: string
}

// ============================================================================
// Pay Config
// ============================================================================

export interface PayConfig {
  epay_id: string
  epay_key: string
  pay_address: string
  pay_methods: string[]
}
