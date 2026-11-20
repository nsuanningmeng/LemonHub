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
        <div className='grid w-full items-start gap-6 lg:grid-cols-[minmax(15rem,.65fr)_minmax(0,1.6fr)]'>
          <RewardSummaryCards />
          <Tabs defaultValue='fixed' className='min-w-0 gap-4'>
            <TabsList
              variant='line'
              className='h-10 max-w-full'
              aria-label={t('Reward types')}
            >
              <TabsTrigger value='fixed' className='px-4'>
                {t('Reward credits')}
              </TabsTrigger>
              <TabsTrigger value='random' className='px-4'>
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
          <div className='min-w-0 lg:col-span-2'>
            <RewardHistory />
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
