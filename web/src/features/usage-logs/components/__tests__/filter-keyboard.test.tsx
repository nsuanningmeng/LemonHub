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
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, test } from 'vitest'

import { api } from '@/lib/api'

import { CommonLogsFilterBar } from '../common-logs-filter-bar'
import { UsageLogsProvider } from '../usage-logs-provider'

let client: QueryClient
const originalAdapter = api.defaults.adapter

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
  api.defaults.adapter = async (config) => ({
    config,
    status: 200,
    statusText: 'OK',
    headers: {},
    data: { success: true, data: { quota: 0, rpm: 0, tpm: 0 } },
  })
})

afterEach(() => {
  client.clear()
  api.defaults.adapter = originalAdapter
})

test.each([
  { isComposing: true, keyCode: 13 },
  { isComposing: false, keyCode: 229 },
])(
  'confirming a log filter candidate ($isComposing/$keyCode) waits for a separate Enter to apply',
  async (event) => {
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
    const input = await screen.findByPlaceholderText('Group')
    fireEvent.change(input, { target: { value: 'zhong' } })
    await act(async () => {
      fireEvent.keyDown(input, { key: 'Enter', ...event })
    })
    expect(router.state.location.search).not.toHaveProperty('group')
    expect(input).toHaveValue('zhong')
    fireEvent.change(input, { target: { value: '中文' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    await waitFor(() =>
      expect(router.state.location.search).toMatchObject({
        group: '中文',
        page: 1,
      })
    )
  }
)
