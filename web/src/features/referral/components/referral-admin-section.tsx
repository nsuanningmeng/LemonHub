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
import { useTranslation } from 'react-i18next'
import { ReferralAdminLeaderboard } from './referral-admin-leaderboard'
import { ReferralAdminSummaryCards } from './referral-admin-summary-cards'

/**
 * Admin-only section appended to the referral page: a site-wide overview plus the global
 * inviter leaderboard. The caller gates rendering on admin role; the backend endpoints are
 * additionally guarded by AdminAuth, so this is the convenience layer, not the security boundary.
 */
export function ReferralAdminSection() {
  const { t } = useTranslation()
  return (
    <section className='flex flex-col gap-4 sm:gap-5'>
      <div className='flex items-center gap-3'>
        <div className='bg-border h-px flex-1' />
        <span className='text-muted-foreground text-xs font-medium tracking-wider uppercase'>
          {t('Admin · Site-wide Referrals')}
        </span>
        <div className='bg-border h-px flex-1' />
      </div>
      <ReferralAdminSummaryCards />
      <ReferralAdminLeaderboard />
    </section>
  )
}
