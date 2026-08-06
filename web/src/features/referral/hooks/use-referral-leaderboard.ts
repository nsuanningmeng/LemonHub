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
import { useQuery } from '@tanstack/react-query'
import { getAffLeaderboard } from '@/features/wallet/api'
import type { AffLeaderboardItem } from '../types'

/**
 * Fetch the referral "Top Contributors" leaderboard — the current user's
 * invitees ranked by the commission they contributed.
 */
export function useReferralLeaderboard(limit = 10) {
  return useQuery<AffLeaderboardItem[]>({
    queryKey: ['referral', 'leaderboard', limit],
    queryFn: async () => {
      const res = await getAffLeaderboard(limit)
      if (res.success && res.data) {
        return res.data.items
      }
      throw new Error(res.message || 'Failed to load referral leaderboard')
    },
  })
}
