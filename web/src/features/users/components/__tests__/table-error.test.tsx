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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { api } from '@/lib/api'
import { Route as PageRoute } from '@/routes/_authenticated/users/index'
import { useAuthStore } from '@/stores/auth-store'

import { UsersProvider } from '../users-provider'
import { UsersTable } from '../users-table'

const clients: QueryClient[] = []
const record = {
  id: 2,
  username: 'member',
  display_name: '',
  role: 1,
  status: 1,
  quota: 0,
  used_quota: 0,
  request_count: 0,
  group: 'default',
}

beforeEach(() => {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  localStorage.clear()
  useAuthStore.getState().auth.setUser({ id: 1, username: 'admin', role: 100 })
})

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  localStorage.clear()
  useAuthStore.getState().auth.reset()
})

async function renderPage(initialEntry = '/users/') {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  const root = createRootRoute()
  const auth = createRoute({ getParentRoute: () => root, id: '_authenticated' })
  const page = createRoute({
    getParentRoute: () => auth,
    path: 'users/',
    validateSearch: PageRoute.options.validateSearch,
    component: () => (
      <TooltipProvider>
        <UsersProvider>
          <UsersTable />
        </UsersProvider>
      </TooltipProvider>
    ),
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([page])]),
    history: createMemoryHistory({ initialEntries: [initialEntry] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

it.each([
  ['business failure', '/users/'],
  ['network failure', '/users/'],
  ['business failure', '/users/?filter=needle'],
  ['network failure', '/users/?filter=needle'],
])(
  'shows an error for %s at %s and retries the same request',
  async (failure, entry) => {
    let unavailable = true
    const get = vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (url === '/api/group/') {
        return { data: { success: true, data: ['default'] } }
      }
      if (unavailable) {
        if (failure === 'network failure') {
          throw new Error('Service unavailable')
        }
        return { data: { success: false, message: 'Service unavailable' } }
      }
      return { data: { success: true, data: { items: [record], total: 1 } } }
    })
    await renderPage(entry)
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Service unavailable'
    )
    expect(screen.queryByText('No Users Found')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('navigation', { name: 'Pagination' })
    ).not.toBeInTheDocument()
    const failedRequest = get.mock.calls.find(([url]) => url !== '/api/group/')
    unavailable = false
    await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
    await screen.findByText('member')
    await waitFor(() =>
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    )
    expect(get.mock.calls.at(-1)).toEqual(failedRequest)
  }
)

it('keeps a successful empty response distinct from a loading failure', async () => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data: {
      success: true,
      data: url === '/api/group/' ? ['default'] : { items: [], total: 0 },
    },
  }))
  await renderPage()
  await screen.findByText('No Users Found')
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Retry' })
  ).not.toBeInTheDocument()
})
