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
import { SectionPageLayout } from '@/components/layout'
import { SubSiteDialogs } from './components/sub-site-dialogs'
import { SubSitePrimaryButtons } from './components/sub-site-primary-buttons'
import { SubSiteProvider } from './components/sub-site-provider'
import { SubSiteTable } from './components/sub-site-table'

export function SubSiteManagement() {
  const { t } = useTranslation()
  return (
    <SubSiteProvider>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>
          {t('Sub-site Management')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <SubSitePrimaryButtons />
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <SubSiteTable />
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <SubSiteDialogs />
    </SubSiteProvider>
  )
}
