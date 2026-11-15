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
import { Text } from '@astryxdesign/core/Text'
import { Command } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useTranslation } from 'react-i18next'

import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
} from '@/components/ui/sidebar'
import { useLayout } from '@/context/layout-provider'
import { useSearch } from '@/context/search-provider'
import { useSidebarView } from '@/hooks/use-sidebar-view'
import { MOTION_TRANSITION, MOTION_VARIANTS } from '@/lib/motion'

import { NavGroup } from './nav-group'
import { SidebarViewHeader } from './sidebar-view-header'
import { SystemBrand } from './system-brand'

export function AppSidebar() {
  const { t } = useTranslation()
  const layout = useLayout()
  const sidebarView = useSidebarView()
  const search = useSearch()
  const shouldReduce = useReducedMotion()

  return (
    <Sidebar
      collapsible={layout.collapsible}
      variant={layout.variant}
      className='border-sidebar-border top-0 h-svh py-3'
    >
      <SidebarHeader className='mx-3 mt-3 mb-5 p-1.5 group-data-[collapsible=icon]:mx-1 group-data-[collapsible=icon]:p-0 md:mt-0'>
        <SystemBrand variant='sidebar' />
      </SidebarHeader>

      {sidebarView.view && <SidebarViewHeader view={sidebarView.view} />}

      <SidebarContent className='gap-5 pb-4'>
        <AnimatePresence mode='wait' initial={false}>
          <motion.nav
            aria-label={t('Workspace navigation')}
            key={sidebarView.key}
            initial={
              shouldReduce ? false : MOTION_VARIANTS.sidebarSlide.initial
            }
            animate={MOTION_VARIANTS.sidebarSlide.animate}
            exit={shouldReduce ? undefined : MOTION_VARIANTS.sidebarSlide.exit}
            transition={MOTION_TRANSITION.fast}
            className='flex flex-col gap-5'
          >
            {sidebarView.navGroups.map((group) => (
              <NavGroup key={group.id || group.title} {...group} />
            ))}
          </motion.nav>
        </AnimatePresence>
      </SidebarContent>

      <SidebarFooter className='mx-3 mb-3 gap-2 p-2 group-data-[collapsible=icon]:mx-1 group-data-[collapsible=icon]:p-0 md:mb-0'>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              tooltip={t('Command palette')}
              onClick={() => search.setOpen(true)}
              className='text-muted-foreground hover:bg-tone-violet hover:text-tone-violet-strong h-10 rounded-lg font-medium'
            >
              <Command aria-hidden='true' className='size-4 shrink-0' />
              <Text
                type='inherit'
                color='inherit'
                className='min-w-0 flex-1 truncate'
              >
                {t('Command palette')}
              </Text>
              <kbd className='text-muted-foreground px-1 py-0.5 font-mono text-xs group-data-[collapsible=icon]:hidden'>
                ⌘ K
              </kbd>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  )
}
