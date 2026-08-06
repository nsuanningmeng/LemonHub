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
import { getAffAdminSummary } from '@/features/wallet/api'
import type { AffAdminSummary } from '../types'

/**
 * Fetch the site-wide referral overview. Admin-only: the caller mounts this hook only inside
 * the admin-gated section (see referral/index.tsx), and the endpoint is additionally guarded
 * by AdminAuth (403 for non-admins), so the hook does not gate by role itself.
 */
export function useReferralAdminSummary() {
  return useQuery<AffAdminSummary>({
    queryKey: ['referral', 'admin', 'summary'],
    queryFn: async () => {
      const res = await getAffAdminSummary()
      if (res.success && res.data) {
        return res.data
      }
      throw new Error(res.message || 'Failed to load referral overview')
    },
  })
}
