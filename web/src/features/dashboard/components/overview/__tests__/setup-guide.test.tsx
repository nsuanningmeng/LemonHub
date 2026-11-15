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
import { act, cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { OverviewDashboard } from '../overview-dashboard'

let client: QueryClient
const getAnimationsDescriptor = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'getAnimations'
)

beforeEach(() => {
  Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
    matches: query === '(prefers-reduced-motion)',
    media: query,
    onchange: null,
    addListener: () => undefined,
    removeListener: () => undefined,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    dispatchEvent: () => false,
  }))
  localStorage.clear()
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'member',
    role: 1,
    quota: 500000,
    request_count: 1,
  })
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  client.setQueryData(['status'], {
    api_info_enabled: false,
    announcements_enabled: true,
    announcements: [{ id: 1, content: 'Local tenant announcement' }],
    faq_enabled: false,
    uptime_kuma_enabled: false,
  })
  client.setQueryData(
    ['dashboard', 'overview', 'api-keys'],
    [{ id: 1, name: 'App', key: 'masked', status: 1 }]
  )
  client.setQueryData(['dashboard', 'overview', 'user-models'], ['test-model'])
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: [] } })
})

afterEach(() => {
  cleanup()
  if (getAnimationsDescriptor) {
    Object.defineProperty(
      HTMLElement.prototype,
      'getAnimations',
      getAnimationsDescriptor
    )
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, 'getAnimations')
  }
  client.clear()
  useAuthStore.getState().auth.reset()
  localStorage.clear()
})

function renderOverview() {
  const router = createRouter({
    routeTree: createRootRoute({ component: OverviewDashboard }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  return render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

test('completed setup uses a compact toggle, restores focus after hiding, and preserves announcements', async () => {
  const user = userEvent.setup()
  renderOverview()
  const toggle = await screen.findByRole('button', { name: 'Setup guide' })
  expect(toggle).toHaveAttribute('aria-expanded', 'false')
  expect(screen.queryByText('Setup guide complete')).not.toBeInTheDocument()
  expect(
    screen.getByRole('button', { name: /Local tenant announcement/ })
  ).toBeVisible()
  toggle.focus()
  await user.keyboard('{Enter}')
  expect(toggle).toHaveAttribute('aria-expanded', 'true')
  expect(
    screen.getByRole('heading', {
      name: 'Build on your API gateway in minutes',
    })
  ).toBeVisible()
  screen.getByRole('button', { name: 'Hide setup guide' }).focus()
  await user.keyboard(' ')
  expect(toggle).toHaveFocus()
  expect(toggle).toHaveAttribute('aria-expanded', 'false')
  expect(
    screen.queryByRole('heading', {
      name: 'Build on your API gateway in minutes',
    })
  ).not.toBeInTheDocument()
  expect(localStorage.getItem('dashboard_overview_setup_guide_expanded')).toBe(
    'collapsed'
  )
  await user.click(
    screen.getByRole('button', { name: /Local tenant announcement/ })
  )
  expect(await screen.findByRole('dialog')).toBeVisible()
})

test('an unfinished manually collapsed setup retains its progress and expand action', async () => {
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'member',
    role: 1,
    quota: 0,
    request_count: 0,
  })
  client.setQueryData(['dashboard', 'overview', 'api-keys'], [])
  localStorage.setItem('dashboard_overview_setup_guide_expanded', 'collapsed')
  const user = userEvent.setup()
  renderOverview()
  const showGuide = await screen.findByRole('button', {
    name: 'Show setup guide',
  })
  expect(screen.getByText('Setup progress: 0/3')).toBeVisible()
  await user.click(showGuide)
  expect(
    await screen.findByRole('button', { name: 'Hide setup guide' })
  ).toHaveAttribute('aria-expanded', 'true')
})

test('completed onboarding keeps workspace shortcuts reachable and hides administrator routes for members', async () => {
  renderOverview()
  const shortcuts = await screen.findByRole('region', {
    name: 'Workspace shortcuts',
  })
  expect(
    within(shortcuts).getByRole('link', { name: 'API Keys' })
  ).toHaveAttribute('href', '/keys')
  expect(
    within(shortcuts).getByRole('link', { name: /Usage Logs/ })
  ).toHaveAttribute('href', '/usage-logs')
  expect(
    within(shortcuts).getByRole('link', { name: /Pricing/ })
  ).toHaveAttribute('href', '/pricing')
  expect(
    within(shortcuts).queryByRole('link', { name: /Channels/ })
  ).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Setup guide' })).toHaveAttribute(
    'aria-expanded',
    'false'
  )
  expect(screen.getByRole('link', { name: 'Open playground' })).toHaveAttribute(
    'href',
    '/playground'
  )
  expect(
    screen.getByRole('region', { name: 'Usage at a glance' })
  ).toBeVisible()
})

test('an administrator can reach channel management from persistent workspace shortcuts', async () => {
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'admin',
    role: 10,
    quota: 500000,
    request_count: 1,
  })
  client.setQueryData(['perf-metrics-summary', 24], { data: { models: [] } })
  renderOverview()
  const shortcuts = await screen.findByRole('region', {
    name: 'Workspace shortcuts',
  })
  expect(
    within(shortcuts).getByRole('link', { name: /Channels/ })
  ).toHaveAttribute('href', '/channels')
})

test('disabled navigation modules are removed from overview links and onboarding while account data stays visible', async () => {
  localStorage.setItem('dashboard_overview_setup_guide_expanded', 'expanded')
  client.setQueryData(['status'], {
    api_info_enabled: false,
    announcements_enabled: false,
    faq_enabled: false,
    uptime_kuma_enabled: false,
    SidebarModulesAdmin: JSON.stringify({
      console: { enabled: true, token: false, log: false },
      personal: { enabled: true, topup: false },
      chat: { enabled: true, playground: false },
    }),
  })
  renderOverview()
  await screen.findByRole('region', { name: 'Usage at a glance' })
  expect(
    screen
      .queryAllByRole('link')
      .filter((link) =>
        ['/keys', '/wallet', '/usage-logs', '/playground'].includes(
          link.getAttribute('href') ?? ''
        )
      )
  ).toHaveLength(0)
  expect(
    screen.queryByRole('button', { name: 'Setup guide' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('region', { name: 'Setup guide' })
  ).not.toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Pricing' })).toBeVisible()
})

test('status content changes replace visible dashboard panels without reloading the page', async () => {
  renderOverview()
  expect(
    await screen.findByRole('region', { name: 'Announcements' })
  ).toBeVisible()
  await act(async () => {
    client.setQueryData(['status'], {
      announcements_enabled: false,
      api_info_enabled: true,
      api_info: [
        {
          route: 'Regional endpoint',
          url: 'https://example.test/v1',
          description: 'Tenant route',
          color: 'blue',
        },
      ],
      faq_enabled: true,
      faq: [
        {
          id: 1,
          question: 'How do I connect?',
          answer: 'Use your regional endpoint.',
        },
      ],
      uptime_kuma_enabled: false,
    })
  })
  expect(
    screen.queryByRole('region', { name: 'Announcements' })
  ).not.toBeInTheDocument()
  expect(screen.getByRole('region', { name: 'API Info' })).toBeVisible()
  expect(screen.getByText('Regional endpoint')).toBeVisible()
  expect(
    screen.getByRole('button', { name: 'How do I connect?' })
  ).toBeVisible()
})
