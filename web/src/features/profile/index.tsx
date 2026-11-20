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
import '@/styles/account-workspace.css'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { useCaptcha } from '@/features/auth/hooks/use-captcha'
import { useStatus } from '@/hooks/use-status'
import { useAuthStore } from '@/stores/auth-store'

import { CheckinCalendarCard } from './components/checkin-calendar-card'
import { LanguagePreferencesCard } from './components/language-preferences-card'
import { LoginSessionsCard } from './components/login-sessions-card'
import { PasskeyCard } from './components/passkey-card'
import { ProfileHeader } from './components/profile-header'
import { ProfileSecurityCard } from './components/profile-security-card'
import { ProfileSettingsCard } from './components/profile-settings-card'
import { SidebarModulesCard } from './components/sidebar-modules-card'
import { TwoFACard } from './components/two-fa-card'
import { UsageActivityCard } from './components/usage-activity-card'
import { useProfile } from './hooks'

export function Profile() {
  const { t } = useTranslation()
  const { profile, loading, refreshProfile } = useProfile()
  const { status } = useStatus()
  const permissions = useAuthStore((s) => s.auth.user?.permissions)

  const checkinEnabled = status?.checkin_enabled === true
  const { isCaptchaEnabled } = useCaptcha()
  const canConfigureSidebar = permissions?.sidebar_settings !== false

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Account workspace')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='grid min-w-0 items-start gap-5 lg:grid-cols-[minmax(12.5rem,.6fr)_minmax(0,1.8fr)] lg:gap-8'>
          <aside className='min-w-0 lg:sticky lg:top-0'>
            <ProfileHeader profile={profile} loading={loading} />
          </aside>
          <Tabs defaultValue='account' className='min-w-0'>
            <TabsList
              variant='line'
              aria-label={t('Account settings')}
              className='min-h-12 w-full flex-wrap justify-start gap-x-1 border-b group-data-horizontal/tabs:h-auto sm:gap-x-3'
            >
              <TabsTrigger className='flex-none' value='account'>
                {t('Account')}
              </TabsTrigger>
              <TabsTrigger className='flex-none' value='security'>
                {t('Security')}
              </TabsTrigger>
              <TabsTrigger className='flex-none' value='activity'>
                {t('Activity')}
              </TabsTrigger>
              <TabsTrigger className='flex-none' value='preferences'>
                {t('Preferences')}
              </TabsTrigger>
            </TabsList>
            <TabsContent
              keepMounted
              value='account'
              className='profile-sections pt-4'
            >
              <ProfileSettingsCard
                section='bindings'
                profile={profile}
                loading={loading}
                onProfileUpdate={refreshProfile}
                quotaRemindEnabled={status?.quota_remind_enabled !== false}
                quotaRemindThreshold={status?.quota_remind_threshold ?? 0}
              />
              <LanguagePreferencesCard
                profile={profile}
                onProfileUpdate={refreshProfile}
              />
            </TabsContent>
            <TabsContent
              keepMounted
              value='security'
              className='profile-sections pt-4'
            >
              <PasskeyCard loading={loading} />
              <TwoFACard loading={loading} />
              <LoginSessionsCard />
              <ProfileSecurityCard profile={profile} loading={loading} />
            </TabsContent>
            <TabsContent
              keepMounted
              value='activity'
              className='profile-sections pt-4'
            >
              <UsageActivityCard />
              {checkinEnabled && (
                <CheckinCalendarCard
                  checkinEnabled={checkinEnabled}
                  captchaEnabled={isCaptchaEnabled}
                />
              )}
            </TabsContent>
            <TabsContent
              keepMounted
              value='preferences'
              className='profile-sections pt-4'
            >
              <ProfileSettingsCard
                section='notifications'
                profile={profile}
                loading={loading}
                onProfileUpdate={refreshProfile}
                quotaRemindEnabled={status?.quota_remind_enabled !== false}
                quotaRemindThreshold={status?.quota_remind_threshold ?? 0}
              />
              {canConfigureSidebar && <SidebarModulesCard />}
            </TabsContent>
          </Tabs>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
