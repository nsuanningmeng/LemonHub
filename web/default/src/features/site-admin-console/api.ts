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
import { api } from '@/lib/api'
import type {
  ApiResponse,
  BrandingPayload,
  CreateRedemptionPayload,
  GetRedemptionsParams,
  GetRedemptionsResponse,
  GetWalletLogsParams,
  GetWalletLogsResponse,
  SearchRedemptionsParams,
  SiteAdminDashboard,
} from './types'

// ============================================================================
// Site-admin Console API (scope = current site, enforced by SiteAdminAuth)
// ============================================================================

// Get the dashboard summary for the current site
export async function getDashboard(): Promise<ApiResponse<SiteAdminDashboard>> {
  const res = await api.get('/api/site-admin/dashboard')
  return res.data
}

// Get paginated wallet ledger for the current site
export async function getWalletLogs(
  params: GetWalletLogsParams = {}
): Promise<GetWalletLogsResponse> {
  const { p = 1, page_size = 10, type } = params
  const typeQuery = type !== undefined ? `&type=${type}` : ''
  const res = await api.get(
    `/api/site-admin/wallet/logs?p=${p}&page_size=${page_size}${typeQuery}`
  )
  return res.data
}

// Update the wallet low-balance warning threshold (厘)
export async function updateWarnThreshold(
  threshold: number
): Promise<ApiResponse> {
  const res = await api.put('/api/site-admin/wallet/warn-threshold', {
    threshold,
  })
  return res.data
}

// Get paginated redemption codes for the current site
export async function getRedemptions(
  params: GetRedemptionsParams = {}
): Promise<GetRedemptionsResponse> {
  const { p = 1, page_size = 10 } = params
  const res = await api.get(
    `/api/site-admin/redemption/?p=${p}&page_size=${page_size}`
  )
  return res.data
}

// Search redemption codes by keyword
export async function searchRedemptions(
  params: SearchRedemptionsParams
): Promise<GetRedemptionsResponse> {
  const { keyword = '', p = 1, page_size = 10 } = params
  const res = await api.get(
    `/api/site-admin/redemption/search?keyword=${encodeURIComponent(keyword)}&p=${p}&page_size=${page_size}`
  )
  return res.data
}

// Batch-create redemption codes. Returns the generated keys on success;
// on insufficient balance the backend returns success=false + message.
export async function createRedemptions(
  data: CreateRedemptionPayload
): Promise<ApiResponse<string[]>> {
  const res = await api.post('/api/site-admin/redemption/', data)
  return res.data
}

// Void (disable) a redemption code
export async function voidRedemption(id: number): Promise<ApiResponse> {
  const res = await api.post(`/api/site-admin/redemption/${id}/void`)
  return res.data
}

// Download redemption codes as a CSV blob (auth header attached via interceptor)
export async function exportRedemptions(): Promise<Blob> {
  const res = await api.get('/api/site-admin/redemption/export', {
    responseType: 'blob',
    skipBusinessError: true,
  })
  return res.data
}

// Update the current site branding
export async function updateBranding(
  data: BrandingPayload
): Promise<ApiResponse> {
  const res = await api.put('/api/site-admin/branding', data)
  return res.data
}
