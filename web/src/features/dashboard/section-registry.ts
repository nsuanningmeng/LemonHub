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
import { redirect } from '@tanstack/react-router'
import type { TFunction } from 'i18next'

import { createSectionRegistry } from '@/features/system-settings/utils/section-registry'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

/**
 * Dashboard page section definitions
 */
const DASHBOARD_SECTIONS = [
  {
    id: 'overview',
    titleKey: 'Overview',
    build: () => null,
  },
  {
    id: 'models',
    titleKey: 'Model Call Analytics',
    build: () => null,
  },
  {
    id: 'flow',
    titleKey: 'Flow',
    build: () => null,
  },
  {
    id: 'users',
    titleKey: 'User Analytics',
    adminOnly: true,
    build: () => null,
  },
  {
    id: 'traffic',
    titleKey: 'Website traffic',
    adminOnly: true,
    build: () => null,
  },
] as const

export type DashboardSectionId = (typeof DASHBOARD_SECTIONS)[number]['id']

export function isAdminDashboardSection(id: string): boolean {
  return DASHBOARD_SECTIONS.some(
    (section) =>
      section.id === id && 'adminOnly' in section && section.adminOnly
  )
}

const dashboardRegistry = createSectionRegistry<
  DashboardSectionId,
  Record<string, never>,
  []
>({
  sections: DASHBOARD_SECTIONS,
  defaultSection: 'overview',
  basePath: '/dashboard',
  urlStyle: 'path',
})

export const DASHBOARD_SECTION_IDS = dashboardRegistry.sectionIds
export const DASHBOARD_DEFAULT_SECTION = dashboardRegistry.defaultSection
export const getDashboardSectionMeta = dashboardRegistry.getSectionMeta

export function requireDashboardSectionAccess({
  params,
}: {
  params: { section: string }
}) {
  if (!DASHBOARD_SECTION_IDS.some((id) => id === params.section)) {
    throw redirect({
      to: '/dashboard/$section',
      params: { section: DASHBOARD_DEFAULT_SECTION },
    })
  }
  const role = useAuthStore.getState().auth.user?.role ?? ROLE.GUEST
  if (isAdminDashboardSection(params.section) && role < ROLE.ADMIN) {
    throw redirect({ to: '/403' })
  }
}

export function getDashboardSectionNavItems(
  t: TFunction,
  options?: { isAdmin?: boolean }
) {
  const all = dashboardRegistry.getSectionNavItems(t)
  if (options?.isAdmin) return all
  return all.filter(
    (_, idx) => !isAdminDashboardSection(DASHBOARD_SECTIONS[idx].id)
  )
}
