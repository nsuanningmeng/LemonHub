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
import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { PublicHeader } from '../public-header'

const originalConfig = useSystemConfigStore.getState()
const originalAuth = useAuthStore.getState()
const animationDescriptor = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'getAnimations'
)
let client: QueryClient

beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { enabled: false, retry: false } },
  })
  client.setQueryData(['status'], {
    system_name: 'LemonHub',
    HeaderNavModules: JSON.stringify({
      pricing: { enabled: true, requireAuth: true },
    }),
  })
  useAuthStore.getState().auth.setUser(null)
  useSystemConfigStore.getState().setConfig({ systemName: 'LemonHub' })
  useSystemConfigStore.getState().setLoading(false)
  Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  useSystemConfigStore.setState(originalConfig, true)
  useAuthStore.setState(originalAuth, true)
  localStorage.clear()
  if (animationDescriptor) {
    Object.defineProperty(
      HTMLElement.prototype,
      'getAnimations',
      animationDescriptor
    )
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, 'getAnimations')
  }
})

async function renderHeader() {
  const router = createRouter({
    routeTree: createRootRoute({
      component: () => (
        <PublicHeader
          showThemeSwitch={false}
          showLanguageSwitcher={false}
          showNotifications={false}
        />
      ),
    }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  await screen.findByRole('banner')
}

test('the mobile navigation exposes its open state, all links and keyboard dismissal', async () => {
  const user = userEvent.setup()
  await renderHeader()
  const trigger = screen.getByRole('button', { name: 'Toggle navigation menu' })
  expect(trigger).toHaveAttribute('aria-expanded', 'false')
  await user.click(trigger)
  const navigation = await screen.findByRole('navigation', {
    name: 'Mobile navigation',
  })
  expect(trigger).toHaveAttribute('aria-expanded', 'true')
  expect(
    within(navigation).getByRole('link', { name: 'About' })
  ).toHaveAttribute('href', '/about')
  await user.keyboard('{Escape}')
  expect(trigger).toHaveAttribute('aria-expanded', 'false')
})

test('a protected mobile destination closes navigation and keeps the sign-in prompt', async () => {
  const user = userEvent.setup()
  await renderHeader()
  await user.click(
    screen.getByRole('button', { name: 'Toggle navigation menu' })
  )
  const navigation = await screen.findByRole('navigation', {
    name: 'Mobile navigation',
  })
  await user.click(
    within(navigation).getByRole('link', { name: 'Model Square' })
  )
  expect(
    await screen.findByRole('dialog', { name: 'Sign in required' })
  ).toBeVisible()
  expect(
    screen.getByRole('button', { name: 'Toggle navigation menu', hidden: true })
  ).toHaveAttribute('aria-expanded', 'false')
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
})

test('desktop overflow keeps secondary destinations reachable', async () => {
  const user = userEvent.setup()
  await renderHeader()
  await user.click(screen.getByRole('button', { name: 'More' }))
  const menu = await screen.findByRole('menu')
  expect(within(menu).getByRole('menuitem', { name: 'About' })).toHaveAttribute(
    'href',
    '/about'
  )
  expect(
    within(menu).getByRole('menuitem', { name: 'Contact Us' })
  ).toHaveAttribute('href', '/contact')
})
