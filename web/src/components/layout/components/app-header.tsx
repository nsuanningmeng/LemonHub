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
import { HStack } from '@astryxdesign/core/HStack'
import { Text } from '@astryxdesign/core/Text'
import { useLocation } from '@tanstack/react-router'
import { ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { ConfigDrawer } from '@/components/config-drawer'
import { LanguageSwitcher } from '@/components/language-switcher'
import { NotificationPopover } from '@/components/notification-popover'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { Search } from '@/components/search'
import { useNotifications } from '@/hooks/use-notifications'
import { useSidebarView } from '@/hooks/use-sidebar-view'
import { useTopNavLinks } from '@/hooks/use-top-nav-links'

import { defaultTopNavLinks } from '../config/top-nav.config'
import { checkIsActive } from '../lib/url-utils'
import type { TopNavLink } from '../types'
import { Header } from './header'
import { SystemBrand } from './system-brand'
import { TopNav } from './top-nav'

type AppHeaderProps = {
  navLinks?: TopNavLink[]
  showTopNav?: boolean
  leftContent?: React.ReactNode
  showSearch?: boolean
  rightContent?: React.ReactNode
  showNotifications?: boolean
  showConfigDrawer?: boolean
  showProfileDropdown?: boolean
}

export function AppHeader(props: AppHeaderProps) {
  const { t } = useTranslation()
  const dynamicLinks = useTopNavLinks()
  const links =
    dynamicLinks.length > 0
      ? dynamicLinks
      : (props.navLinks ?? defaultTopNavLinks)
  const notifications = useNotifications()
  const href = useLocation({ select: (location) => location.href })
  const sidebarView = useSidebarView()
  const activeGroup = sidebarView.navGroups.find((group) =>
    group.items.some((item) => checkIsActive(href, item))
  )
  const activeItem = activeGroup?.items.find((item) =>
    checkIsActive(href, item)
  )

  return (
    <Header>
      {props.leftContent != null ? (
        <HStack gap={2} vAlign='center' className='min-w-0 flex-1'>
          {props.leftContent}
        </HStack>
      ) : (
        <>
          <HStack gap={2} vAlign='center' className='min-w-0 flex-1 md:hidden'>
            <SystemBrand variant='inline' />
          </HStack>
          <HStack
            gap={2}
            vAlign='center'
            className='hidden min-w-0 flex-1 md:flex'
          >
            <Text
              type='supporting'
              color='inherit'
              className='text-muted-foreground max-w-32 shrink-0 truncate text-sm'
            >
              {activeGroup?.title ?? t('Workspace')}
            </Text>
            <ChevronRight
              aria-hidden='true'
              className='text-muted-foreground/60 size-3 shrink-0'
            />
            <Text
              type='label'
              color='inherit'
              className='min-w-0 truncate text-sm font-medium'
            >
              {activeItem?.title ?? t('Workspace')}
            </Text>
          </HStack>
        </>
      )}

      {props.rightContent ?? (
        <HStack gap={1} vAlign='center' className='ms-auto shrink-0 sm:gap-2'>
          {(props.showTopNav ?? true) && (
            <HStack className='hidden lg:flex'>
              <TopNav links={links} />
            </HStack>
          )}
          {(props.showSearch ?? true) && (
            <Search className='border-border hover:bg-muted rounded-lg sm:w-32 lg:w-40 xl:w-44' />
          )}
          {(props.showNotifications ?? true) && (
            <NotificationPopover
              open={notifications.popoverOpen}
              onOpenChange={notifications.setPopoverOpen}
              unreadCount={notifications.unreadCount}
              activeTab={notifications.activeTab}
              onTabChange={notifications.setActiveTab}
              notice={notifications.notice}
              announcements={notifications.announcements}
              loading={notifications.loading}
            />
          )}
          <LanguageSwitcher />
          {(props.showConfigDrawer ?? true) && <ConfigDrawer />}
          {(props.showProfileDropdown ?? true) && <ProfileDropdown />}
        </HStack>
      )}
    </Header>
  )
}
