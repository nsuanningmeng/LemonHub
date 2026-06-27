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
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { getAffAdminLeaderboard } from '@/features/wallet/api'
import type {
  AffAdminLeaderboardData,
  AffAdminLeaderboardParams,
} from '../types'

/**
 * Fetch the paginated site-wide inviter leaderboard. Admin-only: mounted only inside the
 * admin-gated section and additionally guarded by AdminAuth server-side. Keeps the previous
 * page's data while the next page/sort/search loads so the table does not flash empty.
 */
export function useReferralAdminLeaderboard(params: AffAdminLeaderboardParams) {
  return useQuery<AffAdminLeaderboardData>({
    queryKey: ['referral', 'admin', 'leaderboard', params],
    placeholderData: keepPreviousData,
    queryFn: async () => {
      const res = await getAffAdminLeaderboard(params)
      if (res.success && res.data) {
        return res.data
      }
      throw new Error(res.message || 'Failed to load referral leaderboard')
    },
  })
}
