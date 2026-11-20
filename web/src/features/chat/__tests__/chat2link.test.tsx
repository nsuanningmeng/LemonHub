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
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { Route } from '@/routes/_authenticated/chat2link'

afterEach(() => {
  cleanup()
  localStorage.removeItem('status')
  vi.restoreAllMocks()
})

function renderChatLinkRoute() {
  const root = createRootRoute()
  const route = createRoute({
    getParentRoute: () => root,
    path: '/chat2link',
    component: Route.options.component,
  })
  const playground = createRoute({
    getParentRoute: () => root,
    path: '/playground',
  })
  const dashboard = createRoute({
    getParentRoute: () => root,
    path: '/dashboard',
  })
  const router = createRouter({
    routeTree: root.addChildren([route, playground, dashboard]),
    history: createMemoryHistory({ initialEntries: ['/chat2link'] }),
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

test('waits for current configuration and provides an exit for an empty list or native-only links', async () => {
  localStorage.setItem('status', JSON.stringify({ chats: [] }))
  let releaseStatus!: () => void
  const pendingStatus = new Promise<void>((resolve) => {
    releaseStatus = resolve
  })
  const get = vi.spyOn(api, 'get').mockImplementation(async () => {
    await pendingStatus
    return {
      data: {
        success: true,
        data: { chats: [{ Native: 'customchat://open' }] },
      },
    }
  })
  renderChatLinkRoute()
  expect(await screen.findByRole('status')).toHaveTextContent(
    'Loading configuration'
  )
  expect(screen.queryByText('No available Web chat links')).toBeNull()
  releaseStatus()
  expect(
    await screen.findByRole('heading', { name: 'No available Web chat links' })
  ).toBeVisible()
  expect(screen.getByRole('link', { name: 'Playground' })).toHaveAttribute(
    'href',
    '/playground'
  )
  expect(get.mock.calls.map(([url]) => url)).toEqual(['/api/status'])
})

test('a failed status request can be retried and uses the dashboard when Playground is disabled', async () => {
  const get = vi
    .spyOn(api, 'get')
    .mockRejectedValueOnce(new Error('Status unavailable'))
    .mockResolvedValueOnce({
      data: {
        success: true,
        data: {
          chats: [],
          SidebarModulesAdmin: JSON.stringify({ chat: { playground: false } }),
        },
      },
    })
  renderChatLinkRoute()
  expect(
    await screen.findByRole('heading', { name: 'Unable to open chat' })
  ).toBeVisible()
  await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
  expect(
    await screen.findByRole('heading', { name: 'No available Web chat links' })
  ).toBeVisible()
  expect(
    screen.getByRole('link', { name: 'Return to dashboard' })
  ).toHaveAttribute('href', '/dashboard')
  expect(get).toHaveBeenCalledTimes(2)
})
