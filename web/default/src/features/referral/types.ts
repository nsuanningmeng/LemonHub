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
