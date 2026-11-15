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

async function renderKeys() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  const root = createRootRoute()
  const authenticated = createRoute({
    getParentRoute: () => root,
    id: '_authenticated',
  })
  const keys = createRoute({
    getParentRoute: () => authenticated,
    path: 'keys/',
    component: () => (
      <ApiKeysProvider>
        <ApiKeysTable />
      </ApiKeysProvider>
    ),
  })
  const router = createRouter({
    routeTree: root.addChildren([authenticated.addChildren([keys])]),
    history: createMemoryHistory({ initialEntries: ['/keys/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

it.each(['response', 'network'])(
  'keeps a %s failure distinct from an empty account and lets Retry recover',
  async (failure) => {
    vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
    let recovered = false
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (url.startsWith('/api/token/?') && !recovered) {
        if (failure === 'network') throw new Error('Connection interrupted')
        return {
          data: { success: false, message: 'Key service unavailable' },
        }
      }
      return { data: { success: true, data: { items: [], total: 0 } } }
    })
    await renderKeys()

    expect(await screen.findByRole('alert')).toHaveTextContent(
      failure === 'network'
        ? 'Connection interrupted'
        : 'Key service unavailable'
    )
    expect(screen.queryByText('No API Keys Found')).not.toBeInTheDocument()
    expect(screen.queryByText('Total keys')).not.toBeInTheDocument()
    expect(screen.getByRole('search', { name: 'Filter records' })).toBeVisible()
    expect(
      screen.queryByRole('navigation', { name: 'Table pagination' })
    ).not.toBeInTheDocument()

    recovered = true
    await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('No API Keys Found')).toBeVisible()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.getByText('Total keys').nextElementSibling).toHaveTextContent(
      '0'
    )
    expect(screen.getByRole('button', { name: 'Create API Key' })).toBeVisible()
  }
)
