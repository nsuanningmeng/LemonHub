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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { ApiKeysProvider } from '../api-keys-provider'
import { ApiKeysTable } from '../api-keys-table'

const clients: QueryClient[] = []
afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  vi.restoreAllMocks()
  localStorage.clear()
})

it('requests server status on page one and refreshes after disabling a derived expired key', async () => {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  let disabled = false
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url.startsWith('/api/token/?')) {
      const params = new URL(url, 'http://localhost').searchParams
      const filtered = params.get('status') === '3'
      return {
        data: {
          success: true,
          data: {
            total: !filtered ? 60 : Number(!disabled),
            items:
              disabled && filtered
                ? []
                : [
                    {
                      id: 1,
                      name: filtered
                        ? 'expired-server-row'
                        : 'unfiltered-page-row',
                      key: 'masked',
                      status: 1,
                      effective_status: filtered ? 3 : 1,
                      remain_quota: 100,
                      used_quota: 0,
                      unlimited_quota: false,
                      expired_time: 1,
                      created_time: 1,
                      accessed_time: 1,
                      model_limits_enabled: false,
                      group: '',
                      auto_groups: null,
                      cross_group_retry: false,
                      model_limits: '',
                      allow_ips: '',
                    },
                  ],
          },
        },
      }
    }
    return { data: { success: true, data: {} } }
  })
  vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, data: { key: 'local-key' } },
  })
  const put = vi.spyOn(api, 'put').mockImplementation(async () => {
    disabled = true
    return { data: { success: true } }
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  const root = createRootRoute()
  const auth = createRoute({
    getParentRoute: () => root,
    id: '_authenticated',
  })
  const keys = createRoute({
    getParentRoute: () => auth,
    path: 'keys/',
    component: () => (
      <ApiKeysProvider>
        <ApiKeysTable />
      </ApiKeysProvider>
    ),
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([keys])]),
    history: createMemoryHistory({
      initialEntries: ['/keys/?page=3&pageSize=20'],
    }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  await screen.findByText('unfiltered-page-row')
  expect(
    get.mock.calls.some(([url]) => url === '/api/token/?p=3&size=20')
  ).toBe(true)
  await userEvent.click(screen.getByRole('button', { name: 'Status' }))
  await userEvent.click(await screen.findByText('Expired'))
  await screen.findByText('expired-server-row')
  await waitFor(() =>
    expect(
      get.mock.calls.some(([url]) => url === '/api/token/?p=1&size=20&status=3')
    ).toBe(true)
  )
  expect(router.state.location.search).toMatchObject({ status: ['3'] })
  expect(
    new URLSearchParams(router.state.location.searchStr).get('page') ?? '1'
  ).toBe('1')
  await userEvent.click(screen.getByRole('button', { name: 'Disable' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith('/api/token/?status_only=true', {
      id: 1,
      status: 2,
    })
  )
  await waitFor(() =>
    expect(screen.queryByText('expired-server-row')).not.toBeInTheDocument()
  )
  expect(
    get.mock.calls.filter(([url]) => url === '/api/token/?p=1&size=20&status=3')
      .length
  ).toBeGreaterThanOrEqual(2)
})
