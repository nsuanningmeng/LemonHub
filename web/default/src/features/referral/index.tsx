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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SectionPageLayout } from '@/components/layout'
import { TransferDialog } from '@/features/wallet/components/dialogs/transfer-dialog'
import { useAffiliate } from '@/features/wallet/hooks'
import { ReferralLeaderboardTable } from './components/referral-leaderboard-table'
import { ReferralRules } from './components/referral-rules'
import { ReferralStatsCards } from './components/referral-stats-cards'
import { useReferralLeaderboard } from './hooks/use-referral-leaderboard'
import { useReferralStats } from './hooks/use-referral-stats'

export function Referral() {
  const { t } = useTranslation()
  const { affiliateLink, transferQuota, transferring } = useAffiliate()
  const stats = useReferralStats()
  const leaderboard = useReferralLeaderboard(10)
  const [transferDialogOpen, setTransferDialogOpen] = useState(false)

  const handleTransfer = async (amount: number): Promise<boolean> => {
    const success = await transferQuota(amount)
    if (success) {
      await stats.refetch()
    }
    return success
  }

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          {t('Referral Dashboard')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <div className='mx-auto flex w-full max-w-5xl flex-col gap-4 sm:gap-5'>
            <p className='text-muted-foreground text-sm'>
              {t('Invite friends — earn rewards on their top-ups')}
            </p>

            <ReferralStatsCards
              stats={stats.data}
              loading={stats.isLoading}
              affiliateLink={affiliateLink}
              onTransfer={() => setTransferDialogOpen(true)}
            />

            <ReferralLeaderboardTable
              items={leaderboard.data}
              loading={leaderboard.isLoading}
            />

            <ReferralRules />
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <TransferDialog
        open={transferDialogOpen}
        onOpenChange={setTransferDialogOpen}
        onConfirm={handleTransfer}
        availableQuota={stats.data?.pending_quota ?? 0}
        transferring={transferring}
      />
    </>
  )
}
