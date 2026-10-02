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
import { getCoreRowModel, useReactTable } from '@tanstack/react-table'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { CommonLogsFilterBar } from '../common-logs-filter-bar'
import { UsageLogsProvider } from '../usage-logs-provider'

let client: QueryClient

function FilterBar() {
  const table = useReactTable({
    data: [],
    columns: [],
    getCoreRowModel: getCoreRowModel(),
  })
  return (
    <UsageLogsProvider>
      <CommonLogsFilterBar table={table} />
    </UsageLogsProvider>
  )
}

beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 1, username: 'admin', role: 100 })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { quota: 0, rpm: 0, tpm: 0 } },
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  useAuthStore.getState().auth.reset()
})

test('hidden filter values avoid password autofill and remain unchanged when shown and applied', async () => {
  const user = userEvent.setup()
  const root = createRootRoute()
  const authenticated = createRoute({
    getParentRoute: () => root,
    id: '_authenticated',
  })
  const logs = createRoute({
    getParentRoute: () => authenticated,
    path: 'usage-logs/$section',
    component: FilterBar,
  })
  const router = createRouter({
    routeTree: root.addChildren([authenticated.addChildren([logs])]),
    history: createMemoryHistory({ initialEntries: ['/usage-logs/common'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  await user.click(
    await screen.findByRole('button', { name: i18next.t('Expand') })
  )
  const group = screen.getByPlaceholderText(i18next.t('Group'))
  const token = screen.getByPlaceholderText(i18next.t('Token Name'))
  const username = screen.getByPlaceholderText(i18next.t('Username'))
  await user.type(group, 'team-one')
  await user.type(token, 'deployment')
  await user.type(username, 'alice')
  await user.click(screen.getByRole('button', { name: i18next.t('Hide') }))
  for (const input of [group, token, username]) {
    expect(input).not.toHaveAttribute('type', 'password')
    expect(input).toHaveAttribute('autocomplete', 'off')
    expect(input).toHaveClass('[-webkit-text-security:disc]')
  }
  await user.click(screen.getByRole('button', { name: i18next.t('Show') }))
  expect(group).toHaveValue('team-one')
  expect(token).toHaveValue('deployment')
  expect(username).toHaveValue('alice')
  for (const input of [group, token, username]) {
    expect(input).not.toHaveClass('[-webkit-text-security:disc]')
  }
  await user.click(token)
  await user.keyboard('{Enter}')
  await waitFor(() =>
    expect(router.state.location.search).toMatchObject({
      group: 'team-one',
      token: 'deployment',
      username: 'alice',
      page: 1,
    })
  )
})
