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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { SectionPageLayout } from '@/components/layout'
import { BrandingTab } from './components/branding-tab'
import { OverviewTab } from './components/overview-tab'
import { PayConfigTab } from './components/pay-config-tab'
import { RedemptionTab } from './components/redemption-tab'

export function SiteAdminConsole() {
  const { t } = useTranslation()
  const [tab, setTab] = useState('overview')

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Agent Console')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <Tabs
          value={tab}
          onValueChange={(value) => setTab(String(value))}
          className='gap-4'
        >
          <TabsList>
            <TabsTrigger value='overview'>
              {t('Overview & Wallet')}
            </TabsTrigger>
            <TabsTrigger value='redemption'>{t('Redemption Codes')}</TabsTrigger>
            <TabsTrigger value='branding'>{t('Branding Settings')}</TabsTrigger>
            <TabsTrigger value='pay-config'>{t('Payment Settings')}</TabsTrigger>
          </TabsList>

          <TabsContent value='overview'>
            <OverviewTab />
          </TabsContent>
          <TabsContent value='redemption'>
            <RedemptionTab />
          </TabsContent>
          <TabsContent value='branding'>
            <BrandingTab />
          </TabsContent>
          <TabsContent value='pay-config'>
            <PayConfigTab />
          </TabsContent>
        </Tabs>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
