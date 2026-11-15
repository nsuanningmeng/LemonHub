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
import { useState, type CSSProperties } from 'react'

import { AnnouncementBanner } from '@/components/announcement-banner'
import { CommandMenu } from '@/components/command-menu'
import { AnimatedOutlet } from '@/components/page-transition'
import { SkipToMain } from '@/components/skip-to-main'
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar'
import { LayoutProvider } from '@/context/layout-provider'
import { SearchProvider } from '@/context/search-provider'
import { RewardsNotification } from '@/features/rewards/components/rewards-notification'
import { useStatus } from '@/hooks/use-status'
import { getCookie } from '@/lib/cookies'
import { useAuthStore } from '@/stores/auth-store'

import { AppHeader } from './app-header'
import { AppSidebar } from './app-sidebar'

type AuthenticatedLayoutProps = {
  children?: React.ReactNode
}

const workspaceDimensions = {
  '--sidebar-width': 'var(--workspace-rail-width)',
  '--sidebar-width-icon': 'var(--workspace-rail-icon-width)',
} as CSSProperties

export function AuthenticatedLayout(props: AuthenticatedLayoutProps) {
  const { status } = useStatus()
  const [sidebarOpen, setSidebarOpen] = useState<boolean | null>(() => {
    const saved = getCookie('sidebar_state')
    if (saved === 'true') return true
    if (saved === 'false') return false
    return null
  })
  const open = sidebarOpen ?? status?.default_collapse_sidebar !== true
  const userId = useAuthStore((state) => state.auth.user?.id)
  if (userId === undefined) return null

  return (
    <LayoutProvider>
      <SearchProvider>
        <SidebarProvider
          open={open}
          onOpenChange={setSidebarOpen}
          style={workspaceDimensions}
          className='bg-sidebar h-svh min-h-0 overflow-hidden'
        >
          <SkipToMain />
          <HStack gap={0} className='min-h-0 w-full flex-1'>
            <AppSidebar />
            <SidebarInset
              id='content'
              tabIndex={-1}
              className='@container/content h-svh min-h-0 min-w-0 overflow-hidden md:peer-data-[variant=inset]:my-3 md:peer-data-[variant=inset]:me-3 md:peer-data-[variant=inset]:h-[calc(100svh-(var(--spacing)*6))] md:peer-data-[variant=inset]:rounded-2xl md:peer-data-[variant=inset]:shadow-none'
            >
              <AppHeader />
              <AnnouncementBanner />
              {props.children ?? <AnimatedOutlet />}
            </SidebarInset>
          </HStack>
        </SidebarProvider>
        <CommandMenu />
        {userId && <RewardsNotification key={userId} />}
      </SearchProvider>
    </LayoutProvider>
  )
}
