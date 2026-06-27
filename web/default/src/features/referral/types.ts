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
// ============================================================================
// Referral Type Definitions
// ============================================================================
import type { ApiResponse } from '@/features/wallet/types'

/**
 * Aggregated referral statistics for the current user.
 * All `*_quota` fields are quota units and should be rendered with
 * `formatQuota` from `@/lib/format`.
 */
export interface AffStats {
  /** Affiliate / referral code */
  aff_code: string
  /** Pending (not yet transferred) reward quota */
  pending_quota: number
  /** Total reward quota earned historically */
  total_earned_quota: number
  /** Number of invitees that activated (first successful top-up) */
  activated_count: number
  /** Total number of invited users */
  total_invited: number
  /** Commission quota earned in the current month */
  month_commission_quota: number
}

/**
 * A single row in the "Top Contributors" leaderboard.
 */
export interface AffLeaderboardItem {
  /** Invitee user ID */
  invitee_id: number
  /** Invitee display name (already masked by the backend) */
  username: string
  /** Total commission quota this invitee contributed */
  commission_quota: number
  /** Number of top-ups this invitee made */
  recharge_count: number
  /** Last activity timestamp (unix seconds) */
  last_at: number
}

/**
 * Leaderboard response payload.
 */
export interface AffLeaderboardData {
  items: AffLeaderboardItem[]
}

export type AffStatsResponse = ApiResponse<AffStats>
export type AffLeaderboardResponse = ApiResponse<AffLeaderboardData>

// ============================================================================
// Admin — site-wide referral views (admin only)
// ============================================================================

/** Site-wide referral overview surfaced on the admin section of the referral page. */
export interface AffAdminSummary {
  /** Lifetime commission paid to all inviters (quota) */
  total_commission_paid: number
  /** Currently un-transferred reward quota across all inviters */
  total_pending_quota: number
  /** Total activated invitees across the site */
  total_activated: number
  /** Number of distinct inviters (users who invited at least one person) */
  inviter_count: number
  /** Commission credited this calendar month (quota) */
  month_commission_quota: number
}

/**
 * One inviter row of the site-wide leaderboard.
 * Unlike {@link AffLeaderboardItem}, the username is NOT masked (admin-only view).
 */
export interface AffAdminLeaderboardItem {
  inviter_id: number
  username: string
  display_name: string
  /** Lifetime commission earned (quota) */
  total_earned_quota: number
  /** Un-transferred reward quota (quota) */
  pending_quota: number
  /** Number of activated invitees */
  activated_count: number
  /** Total invited users */
  total_invited: number
  /** Commission earned this calendar month (quota) */
  month_commission_quota: number
  /** Lifetime cash-settled commission, gross owed before any settlements (quota) */
  cash_commission_total?: number
  /** Total already settled off-platform in cash (quota) */
  cash_commission_paid?: number
  /**
   * OUTSTANDING cash balance = total − paid, clamped ≥ 0 (quota). 0 for normal
   * inviters; for cash-settled promoters this is the cash the operator still owes.
   */
  cash_commission_owed?: number
  /** Whether this inviter is a cash-settled promoter (off-platform cash settlement) */
  is_cash_settled?: boolean
  /** Last referral activity timestamp (unix seconds, 0 if none) */
  last_at: number
}

export interface AffAdminLeaderboardData {
  items: AffAdminLeaderboardItem[]
  total: number
  page: number
  page_size: number
}

/** Server-side sortable columns for the admin leaderboard. */
export type AffAdminSortColumn =
  | 'total_earned'
  | 'pending'
  | 'activated'
  | 'username'
export type AffAdminSortOrder = 'asc' | 'desc'

export interface AffAdminLeaderboardParams {
  page: number
  pageSize: number
  keyword?: string
  sort?: AffAdminSortColumn
  order?: AffAdminSortOrder
}

export type AffAdminSummaryResponse = ApiResponse<AffAdminSummary>
export type AffAdminLeaderboardResponse = ApiResponse<AffAdminLeaderboardData>

// ============================================================================
// Admin — off-platform cash settlements for cash-settled promoters
// ============================================================================

/**
 * A single off-platform cash settlement recorded against a cash-settled promoter.
 * Each settlement reduces that promoter's outstanding cash balance.
 */
export interface AffCashPayout {
  id: number
  /** Inviter (promoter) this settlement belongs to */
  inviter_id: number
  /** Settled amount (quota units) */
  amount: number
  /** Optional free-text note recorded by the operator */
  note: string
  /** Admin user id who recorded the settlement */
  operator_id: number
  /** Settlement timestamp (unix seconds) */
  created_at: number
}

export interface AffCashPayoutListData {
  items: AffCashPayout[]
}

/** Payload for recording a new cash settlement. `amount` is in quota units. */
export interface AffCashPayoutRequest {
  inviter_id: number
  amount: number
  note?: string
}

export type AffCashPayoutListResponse = ApiResponse<AffCashPayoutListData>
export type AffCashPayoutResponse = ApiResponse<AffCashPayout>
