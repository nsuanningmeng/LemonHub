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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import {
  getAffAdminCashPayouts,
  recordAffAdminCashPayout,
} from '@/features/wallet/api'

import type { AffCashPayout, AffCashPayoutRequest } from '../types'

/**
 * Fetch the off-platform cash settlement history for a single promoter (admin only).
 * Pass `null` to keep the query idle (e.g. while the settlement dialog is closed). Like the
 * sibling admin hooks it relies on AdminAuth server-side rather than gating by role here.
 */
export function useReferralAdminCashPayouts(inviterId: number | null) {
  return useQuery<AffCashPayout[]>({
    queryKey: ['referral', 'admin', 'cash-payouts', inviterId],
    enabled: inviterId != null,
    queryFn: async () => {
      const res = await getAffAdminCashPayouts(inviterId as number)
      if (res.success && res.data) {
        return res.data.items
      }
      throw new Error(res.message || 'Failed to load settlement history')
    },
  })
}

/**
 * Record an off-platform cash settlement. On a successful response it invalidates the admin
 * referral queries (leaderboard, summary, and per-promoter settlement history) so the
 * outstanding balance and history refetch. The caller handles success/error UX (toast, close).
 */
export function useRecordReferralAdminCashPayout() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (request: AffCashPayoutRequest) =>
      recordAffAdminCashPayout(request),
    onSuccess: (res) => {
      if (res.success) {
        queryClient.invalidateQueries({ queryKey: ['referral', 'admin'] })
      }
    },
  })
}
