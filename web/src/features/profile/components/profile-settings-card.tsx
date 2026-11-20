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
import { Link2, Settings } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { TitledCard } from '@/components/ui/titled-card'

import type { UserProfile } from '../types'
import { AccountBindingsTab } from './tabs/account-bindings-tab'
import { NotificationTab } from './tabs/notification-tab'

// ============================================================================
// Profile Settings Card Component
// ============================================================================

interface ProfileSettingsCardProps {
  section: 'bindings' | 'notifications'
  profile: UserProfile | null
  loading: boolean
  onProfileUpdate: () => void
  quotaRemindEnabled: boolean
  quotaRemindThreshold: number
}

export function ProfileSettingsCard({
  section,
  profile,
  loading,
  onProfileUpdate,
  quotaRemindEnabled,
  quotaRemindThreshold,
}: ProfileSettingsCardProps) {
  const { t } = useTranslation()

  if (loading) {
    return (
      <Card data-card-hover='false' className='gap-0 overflow-hidden py-0'>
        <CardHeader className='border-b p-3 !pb-3 sm:p-5 sm:!pb-5'>
          <Skeleton className='h-6 w-32' />
          <Skeleton className='mt-2 h-4 w-48' />
        </CardHeader>
        <CardContent className='space-y-4 p-3 sm:p-5'>
          <Skeleton className='h-10 w-full' />
          {['bindings', 'preferences', 'notifications'].map((key) => (
            <Skeleton key={key} className='h-20 w-full' />
          ))}
        </CardContent>
      </Card>
    )
  }

  const bindings = section === 'bindings'
  return (
    <TitledCard
      title={bindings ? t('Connected accounts') : t('Notification preferences')}
      description={
        bindings
          ? t('Manage the services you use to sign in.')
          : t('Choose where account and usage alerts are delivered.')
      }
      icon={bindings ? <Link2 /> : <Settings />}
      disableHoverEffect
    >
      {bindings ? (
        <AccountBindingsTab profile={profile} onUpdate={onProfileUpdate} />
      ) : (
        <NotificationTab
          profile={profile}
          onUpdate={onProfileUpdate}
          quotaRemindEnabled={quotaRemindEnabled}
          quotaRemindThreshold={quotaRemindThreshold}
        />
      )}
    </TitledCard>
  )
}
