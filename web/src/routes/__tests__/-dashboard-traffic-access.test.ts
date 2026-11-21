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
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
} from '@tanstack/react-router'
import { afterEach, expect, test } from 'vitest'

import { requireDashboardSectionAccess } from '@/features/dashboard/section-registry'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

afterEach(() => {
  useAuthStore.getState().auth.reset()
  localStorage.clear()
})

test.each([
  [ROLE.USER, '/403'],
  [ROLE.SITE_ADMIN, '/403'],
  [ROLE.ADMIN, '/dashboard/traffic'],
  [ROLE.SUPER_ADMIN, '/dashboard/traffic'],
])('role %i opening the traffic report lands on %s', async (role, expected) => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'tester', role })
  const root = createRootRoute()
  const dashboard = createRoute({
    getParentRoute: () => root,
    path: '/dashboard/$section',
    beforeLoad: requireDashboardSectionAccess,
  })
  const forbidden = createRoute({ getParentRoute: () => root, path: '/403' })
  const router = createRouter({
    routeTree: root.addChildren([dashboard, forbidden]),
    history: createMemoryHistory({ initialEntries: ['/dashboard/traffic'] }),
  })
  await router.load()
  expect(router.state.location.pathname).toBe(expected)
})
