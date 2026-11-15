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
import {
  act,
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test } from 'vitest'

import { DirectionProvider } from '@/context/direction-provider'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { AuthenticatedLayout } from '../authenticated-layout'

const originalConfig = useSystemConfigStore.getState()
let client: QueryClient

beforeEach(() => {
  localStorage.clear()
  document.cookie = 'sidebar_state=; max-age=0; path=/'
  document.cookie = 'layout_collapsible=icon; path=/'
  client = new QueryClient({
    defaultOptions: { queries: { enabled: false, retry: false } },
  })
  client.setQueryData(['rewards', 1, 'summary'], { pending_count: 0 })
  client.setQueryData(['rewards', 1, 'notifications'], { items: [] })
  useAuthStore.getState().auth.reset()
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'member',
    role: ROLE.USER,
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  localStorage.clear()
  document.cookie = 'sidebar_state=; max-age=0; path=/'
  document.cookie = 'layout_collapsible=; max-age=0; path=/'
  useAuthStore.getState().auth.reset()
  useSystemConfigStore.setState(originalConfig, true)
})

async function renderWorkspace(defaultCollapsed?: boolean) {
  if (defaultCollapsed !== undefined) {
    client.setQueryData(['status'], {
      default_collapse_sidebar: defaultCollapsed,
    })
  }
  const router = createRouter({
    routeTree: createRootRoute({
      component: () => (
        <AuthenticatedLayout>Workspace content</AuthenticatedLayout>
      ),
    }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <DirectionProvider>
        <RouterProvider router={router} />
      </DirectionProvider>
    </QueryClientProvider>
  )
  const navigation = await screen.findByRole('navigation', {
    name: 'Workspace navigation',
  })
  return navigation.closest('[data-slot="sidebar"]')
}

test('a user without a saved preference receives the configured collapsed sidebar', async () => {
  const sidebar = await renderWorkspace(true)
  expect(sidebar).toHaveAttribute('data-state', 'collapsed')
  expect(document.cookie).not.toContain('sidebar_state=')
})

test('the initial sidebar follows the site default when status arrives after the layout', async () => {
  const sidebar = await renderWorkspace()
  expect(sidebar).toHaveAttribute('data-state', 'expanded')

  act(() => {
    client.setQueryData(['status'], { default_collapse_sidebar: true })
  })

  await waitFor(() =>
    expect(sidebar).toHaveAttribute('data-state', 'collapsed')
  )
})

test.each([
  ['true', true, 'expanded'],
  ['false', false, 'collapsed'],
] as const)(
  'saved preference %s overrides site collapsed=%s',
  async (saved, defaultCollapsed, state) => {
    document.cookie = `sidebar_state=${saved}; path=/`
    const sidebar = await renderWorkspace(defaultCollapsed)
    expect(sidebar).toHaveAttribute('data-state', state)
  }
)

test('a manual collapse stays selected when a later status refresh requests an expanded default', async () => {
  const sidebar = await renderWorkspace()
  await userEvent.setup().click(
    within(screen.getByRole('banner')).getByRole('button', {
      name: 'Toggle Sidebar',
    })
  )
  expect(sidebar).toHaveAttribute('data-state', 'collapsed')

  act(() => {
    client.setQueryData(['status'], { default_collapse_sidebar: false })
  })

  await waitFor(() =>
    expect(sidebar).toHaveAttribute('data-state', 'collapsed')
  )
  expect(document.cookie).toContain('sidebar_state=false')
})
