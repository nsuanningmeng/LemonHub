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

// ============================================================================
// Site Schema & Types
// ============================================================================

export const siteSchema = z.object({
  id: z.number(),
  name: z.string(),
  logo: z.string(),
  notice: z.string(),
  footer: z.string(),
  owner_user_id: z.number(),
  owner_username: z.string(),
  status: z.number(), // 1: normal, 2: disabled
  wallet_balance: z.number(), // integer 厘 (0.001 CNY)
  discount_rate: z.number(), // basis of 10000; 10000 = no discount
  wallet_warn_threshold: z.number(),
  pay_config: z.string(),
  // Backend returns the bound domains as a flat string array (read = write shape).
  domains: z.array(z.string()),
  created_time: z.number(), // Unix seconds
  updated_time: z.number(),
})

export type Site = z.infer<typeof siteSchema>

// ============================================================================
// API Request/Response Types
// ============================================================================

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export interface GetSitesParams {
  p?: number
  page_size?: number
}

export interface GetSitesResponse {
  success: boolean
  message?: string
  data?: {
    items: Site[]
    total: number
    page: number
    page_size: number
  }
}

export interface SearchSitesParams {
  keyword?: string
  p?: number
  page_size?: number
}

export interface SiteCreatePayload {
  name: string
  domains: string[]
  owner_username: string
  logo?: string
  notice?: string
  footer?: string
  discount_rate?: number
  status?: number
  wallet_warn_threshold?: number
  pay_config?: string
}

export interface SiteUpdatePayload {
  id: number
  name: string
  domains: string[]
  owner_username?: string
  logo?: string
  notice?: string
  footer?: string
  discount_rate?: number
  status?: number
  wallet_warn_threshold?: number
  pay_config?: string
}

// ============================================================================
// Dialog Types
// ============================================================================

export type SubSiteDialogType = 'create' | 'update' | 'delete'
