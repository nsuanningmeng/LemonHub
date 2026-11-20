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
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { AppHeader } from './app-header'
import { AppSidebar } from './app-sidebar'
import { MainRegionContext } from './main-region-context'

type AuthenticatedLayoutProps = {
  children?: React.ReactNode
}

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
          className='flex-col'
        >
          <SkipToMain />
          <AppHeader />
          <div className='flex min-h-0 w-full flex-1'>
            <AppSidebar />
            <SidebarInset
              id='content'
              tabIndex={-1}
              className={cn(
                '@container/content',
                'h-[calc(100svh-var(--app-header-height,0px))]',
                'min-h-0 min-w-0 overflow-hidden focus:outline-none',
                'peer-data-[variant=inset]:h-[calc(100svh-var(--app-header-height,0px)-(var(--spacing)*4))]'
              )}
            >
              <MainRegionContext.Provider value>
                <AnnouncementBanner />
                {props.children ?? <AnimatedOutlet />}
              </MainRegionContext.Provider>
            </SidebarInset>
          </div>
        </SidebarProvider>
        <CommandMenu />
        {userId && <RewardsNotification key={userId} />}
      </SearchProvider>
    </LayoutProvider>
  )
}
