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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, test } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { useSidebarView } from '../use-sidebar-view'

function SidebarLinks() {
  const view = useSidebarView()
  return (
    <nav>
      {view.navGroups.flatMap((group) =>
        group.items.map((item) => (
          <a key={item.url} href={item.url}>
            {item.title}
          </a>
        ))
      )}
    </nav>
  )
}

let client: QueryClient

afterEach(() => {
  cleanup()
  client?.clear()
  useAuthStore.getState().auth.reset()
  localStorage.clear()
})

test.each([
  { role: ROLE.SUPER_ADMIN, showsSettings: true, showsAgent: false },
  { role: ROLE.ADMIN, showsSettings: false, showsAgent: false },
  { role: ROLE.SITE_ADMIN, showsSettings: false, showsAgent: true },
])(
  'role $role sees only its authorized settings and agent console navigation',
  async ({ role, showsSettings, showsAgent }) => {
    useAuthStore.getState().auth.setUser({ id: 1, username: 'operator', role })
    client = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    })
    client.setQueryData(['status'], {})
    const router = createRouter({
      routeTree: createRootRoute({ component: SidebarLinks }),
      history: createMemoryHistory({ initialEntries: ['/'] }),
    })
    render(
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    )
    await screen.findByRole('navigation')
    expect(
      Boolean(screen.queryByRole('link', { name: 'System Settings' }))
    ).toBe(showsSettings)
    expect(Boolean(screen.queryByRole('link', { name: 'Agent Console' }))).toBe(
      showsAgent
    )
  }
)
