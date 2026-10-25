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

import { SectionPageLayout } from '@/components/layout/components/section-page-layout'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { PendingRewards } from './components/pending-rewards'
import { RewardHistory } from './components/reward-history'
import { RewardSummaryCards } from './components/reward-summary'

export function Rewards() {
  const { t } = useTranslation()
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('My Rewards')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mx-auto flex w-full max-w-7xl flex-col gap-7 py-3 sm:gap-8 sm:py-5'>
          <RewardSummaryCards />
          <Tabs defaultValue='fixed' className='gap-6'>
            <TabsList
              className='h-11 rounded-2xl p-1'
              aria-label={t('Reward types')}
            >
              <TabsTrigger value='fixed' className='rounded-xl px-6'>
                {t('Reward credits')}
              </TabsTrigger>
              <TabsTrigger value='random' className='rounded-xl px-6'>
                {t('Red packets')}
              </TabsTrigger>
            </TabsList>
            <TabsContent value='fixed'>
              <PendingRewards type='fixed' />
            </TabsContent>
            <TabsContent value='random'>
              <PendingRewards type='random' />
            </TabsContent>
          </Tabs>
          <RewardHistory />
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
