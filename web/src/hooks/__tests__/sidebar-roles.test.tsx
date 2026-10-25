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
import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
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
          <a key={item.title} href={item.url}>
            {item.title}
            {item.badge && <span>{item.badge}</span>}
          </a>
        ))
      )}
    </nav>
  )
}

let client: QueryClient

function renderSidebar(role: number, adminConfig = '', userConfig = '') {
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'operator',
    role,
    sidebar_modules: userConfig,
  })
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  client.setQueryData(['status'], { SidebarModulesAdmin: adminConfig })
  client.setQueryData(['rewards', 1, 'summary'], {
    pending_count: 0,
    fixed_count: 0,
    random_count: 0,
    fixed_quota: 0,
  })
  const router = createRouter({
    routeTree: createRootRoute({ component: SidebarLinks }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  return render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

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
  { role: ROLE.USER, showsSettings: false, showsAgent: false },
])(
  'role $role sees only its authorized settings and agent console navigation',
  async ({ role, showsSettings, showsAgent }) => {
    renderSidebar(role)
    await screen.findByRole('navigation')
    expect(
      Boolean(screen.queryByRole('link', { name: 'System Settings' }))
    ).toBe(showsSettings)
    expect(Boolean(screen.queryByRole('link', { name: 'Agent Console' }))).toBe(
      showsAgent
    )
    expect(screen.getByRole('link', { name: 'My Rewards' })).toHaveAttribute(
      'href',
      '/rewards'
    )
    expect(Boolean(screen.queryByRole('link', { name: 'Reward Center' }))).toBe(
      role >= ROLE.ADMIN
    )
  }
)

test('pending rewards appear in navigation and disappear after all rewards are claimed', async () => {
  renderSidebar(ROLE.USER)
  const link = await screen.findByRole('link', { name: 'My Rewards' })
  act(() => {
    client.setQueryData(['rewards', 1, 'summary'], {
      pending_count: 2,
      fixed_count: 1,
      random_count: 1,
      fixed_quota: 500000,
    })
  })
  await waitFor(() => expect(link).toHaveTextContent('My Rewards2'))

  act(() => {
    client.setQueryData(['rewards', 1, 'summary'], {
      pending_count: 0,
      fixed_count: 0,
      random_count: 0,
      fixed_quota: 0,
    })
  })
  await waitFor(() => expect(link).toHaveTextContent(/^My Rewards$/))
})

test('legacy sidebar configurations show new reward entries by default', async () => {
  const legacyConfig = JSON.stringify({
    personal: { enabled: true, topup: true },
    admin: { enabled: true, redemption: true },
  })
  renderSidebar(ROLE.ADMIN, legacyConfig, legacyConfig)
  expect(await screen.findByRole('link', { name: 'My Rewards' })).toBeVisible()
  expect(screen.getByRole('link', { name: 'Reward Center' })).toHaveAttribute(
    'href',
    '/reward-campaigns'
  )
})

test.each(['admin', 'user'])(
  '%s configuration can hide reward entries without hiding the wallet',
  async (layer) => {
    const hiddenConfig = JSON.stringify({
      personal: { enabled: true, rewards: false },
      admin: { enabled: true, reward_campaigns: false },
    })
    const visibleConfig = JSON.stringify({
      personal: { enabled: true, rewards: true },
      admin: { enabled: true, reward_campaigns: true },
    })
    renderSidebar(
      ROLE.ADMIN,
      layer === 'admin' ? hiddenConfig : visibleConfig,
      layer === 'user' ? hiddenConfig : visibleConfig
    )
    await screen.findByRole('navigation')
    expect(screen.queryByRole('link', { name: 'My Rewards' })).toBeNull()
    expect(screen.queryByRole('link', { name: 'Reward Center' })).toBeNull()
    expect(screen.getByRole('link', { name: 'Wallet' })).toBeVisible()
  }
)
