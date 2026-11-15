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
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test } from 'vitest'

import { CommandMenu } from '@/components/command-menu'
import { SidebarProvider } from '@/components/ui/sidebar'
import { LayoutProvider } from '@/context/layout-provider'
import { SearchProvider } from '@/context/search-provider'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { AppSidebar } from '../app-sidebar'
import { Header } from '../header'

const originalWidth = window.innerWidth
const originalConfig = useSystemConfigStore.getState()
const animationsDescriptor = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'getAnimations'
)
let client: QueryClient

function WorkspaceFixture() {
  return (
    <LayoutProvider>
      <SearchProvider>
        <SidebarProvider>
          <AppSidebar />
          <Header />
        </SidebarProvider>
        <CommandMenu />
      </SearchProvider>
    </LayoutProvider>
  )
}

beforeEach(() => {
  Object.defineProperty(window, 'innerWidth', {
    configurable: true,
    value: 1280,
  })
  Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
  document.cookie = 'layout_variant=sidebar; path=/'
  document.cookie = 'layout_collapsible=icon; path=/'
  localStorage.clear()
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  client.setQueryData(['status'], { system_name: 'LemonHub', chats: [] })
  client.setQueryData(['rewards', 1, 'summary'], { pending_count: 0 })
})

afterEach(() => {
  client.clear()
  useAuthStore.getState().auth.reset()
  useSystemConfigStore.setState(originalConfig, true)
  localStorage.clear()
  document.cookie = 'layout_variant=; max-age=0; path=/'
  document.cookie = 'layout_collapsible=; max-age=0; path=/'
  document.cookie = 'sidebar_state=; max-age=0; path=/'
  Object.defineProperty(window, 'innerWidth', {
    configurable: true,
    value: originalWidth,
  })
  if (animationsDescriptor) {
    Object.defineProperty(
      HTMLElement.prototype,
      'getAnimations',
      animationsDescriptor
    )
  } else Reflect.deleteProperty(HTMLElement.prototype, 'getAnimations')
})

function renderWorkspace(path = '/keys', role: number = ROLE.USER) {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'maker', role })
  const root = createRootRoute({ component: WorkspaceFixture })
  const routeTree = root.addChildren([
    createRoute({ getParentRoute: () => root, path: '/keys' }),
    createRoute({ getParentRoute: () => root, path: '/dashboard/overview' }),
    createRoute({
      getParentRoute: () => root,
      path: '/system-settings/site/system-info',
    }),
  ])
  const router = createRouter({
    routeTree,
    history: createMemoryHistory({ initialEntries: [path] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return router
}

test('the workspace rail starts with account activity and exposes the current authorized page', async () => {
  renderWorkspace()
  const nav = await screen.findByRole('navigation', {
    name: 'Workspace navigation',
  })
  expect(within(nav).getAllByRole('link')[0]).toHaveAttribute(
    'href',
    '/dashboard/overview'
  )
  expect(within(nav).getByRole('link', { name: 'API Keys' })).toHaveAttribute(
    'aria-current',
    'page'
  )
  expect(
    within(nav).getByRole('link', { name: 'Usage analytics' })
  ).toHaveAttribute('href', '/dashboard/models')
  expect(
    within(nav).queryByRole('link', { name: 'Users' })
  ).not.toBeInTheDocument()
})

test('the command palette action in the workspace rail opens searchable commands', async () => {
  renderWorkspace()
  await userEvent
    .setup()
    .click(await screen.findByRole('button', { name: /Command palette/ }))
  expect(
    await screen.findByPlaceholderText('Type a command or search...')
  ).toBeVisible()
})

test('collapsing the workspace rail preserves the current page and persists the preference', async () => {
  renderWorkspace()
  const nav = await screen.findByRole('navigation', {
    name: 'Workspace navigation',
  })
  const trigger = within(screen.getByRole('banner')).getByRole('button', {
    name: 'Toggle Sidebar',
  })
  await userEvent.setup().click(trigger)
  expect(document.cookie).toContain('sidebar_state=false')
  expect(within(nav).getByRole('link', { name: 'API Keys' })).toHaveAttribute(
    'aria-current',
    'page'
  )
})

test('selecting a page from the mobile rail navigates and closes the drawer', async () => {
  Object.defineProperty(window, 'innerWidth', {
    configurable: true,
    value: 375,
  })
  const router = renderWorkspace()
  const trigger = await screen.findByRole('button', { name: 'Toggle Sidebar' })
  await waitFor(() =>
    expect(
      screen.queryByRole('navigation', { name: 'Workspace navigation' })
    ).not.toBeInTheDocument()
  )
  const user = userEvent.setup()
  await user.click(trigger)
  const nav = await screen.findByRole('navigation', {
    name: 'Workspace navigation',
  })
  await user.click(within(nav).getByRole('link', { name: 'Overview' }))
  await waitFor(() =>
    expect(router.state.location.pathname).toBe('/dashboard/overview')
  )
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
})

test('a settings workspace keeps its back route and selected settings section', async () => {
  renderWorkspace('/system-settings/site/system-info', ROLE.SUPER_ADMIN)
  const back = await screen.findByRole('link', { name: 'Back to Dashboard' })
  expect(back).toHaveAttribute('href', '/dashboard/overview')
  expect(
    screen.getByRole('button', { name: 'Site & Branding' })
  ).toHaveAttribute('aria-expanded', 'true')
})
