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
  Site,
  ApiResponse,
  GetSitesParams,
  GetSitesResponse,
  SearchSitesParams,
  SiteCreatePayload,
  SiteUpdatePayload,
  RechargeWalletPayload,
  AdjustWalletPayload,
  WalletMutationResponse,
  GetWalletLogsParams,
  GetWalletLogsResponse,
  ReconcileResult,
} from './types'

// ============================================================================
// Sub-site Management API
// ============================================================================

// Get paginated sub-sites list
export async function getSites(
  params: GetSitesParams = {}
): Promise<GetSitesResponse> {
  const { p = 1, page_size = 10 } = params
  const res = await api.get(`/api/site/?p=${p}&page_size=${page_size}`)
  return res.data
}

// Search sub-sites by keyword
export async function searchSites(
  params: SearchSitesParams
): Promise<GetSitesResponse> {
  const { keyword = '', p = 1, page_size = 10 } = params
  const res = await api.get(
    `/api/site/search?keyword=${encodeURIComponent(keyword)}&p=${p}&page_size=${page_size}`
  )
  return res.data
}

// Get single sub-site by ID
export async function getSite(id: number): Promise<ApiResponse<Site>> {
  const res = await api.get(`/api/site/${id}`)
  return res.data
}

// Create sub-site
export async function createSite(
  data: SiteCreatePayload
): Promise<ApiResponse<Site>> {
  const res = await api.post('/api/site/', data)
  return res.data
}

// Update sub-site
export async function updateSite(
  data: SiteUpdatePayload
): Promise<ApiResponse<Site>> {
  const res = await api.put('/api/site/', data)
  return res.data
}

// Delete sub-site
export async function deleteSite(id: number): Promise<ApiResponse> {
  const res = await api.delete(`/api/site/${id}`)
  return res.data
}

// ============================================================================
// Sub-site Wallet API (amounts in 厘 = 0.001 CNY)
// ============================================================================

// Recharge a sub-site wallet
export async function rechargeSiteWallet(
  id: number,
  data: RechargeWalletPayload
): Promise<WalletMutationResponse> {
  const res = await api.post(`/api/site/${id}/wallet/recharge`, data)
  return res.data
}

// Manually adjust a sub-site wallet (amount may be negative; remark required)
export async function adjustSiteWallet(
  id: number,
  data: AdjustWalletPayload
): Promise<WalletMutationResponse> {
  const res = await api.post(`/api/site/${id}/wallet/adjust`, data)
  return res.data
}

// Get paginated wallet ledger for a sub-site
export async function getSiteWalletLogs(
  id: number,
  params: GetWalletLogsParams = {}
): Promise<GetWalletLogsResponse> {
  const { p = 1, page_size = 10, type } = params
  const typeQuery = type !== undefined ? `&type=${type}` : ''
  const res = await api.get(
    `/api/site/${id}/wallet/logs?p=${p}&page_size=${page_size}${typeQuery}`
  )
  return res.data
}

// Reconcile all sub-site wallets against their ledgers
export async function reconcileSites(): Promise<ApiResponse<ReconcileResult[]>> {
  const res = await api.get('/api/site/reconcile')
  return res.data
}
