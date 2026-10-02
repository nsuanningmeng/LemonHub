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
import { QueryClient } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRoute,
  createRouter,
} from '@tanstack/react-router'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

vi.setConfig({ testTimeout: 15000 })

const clients: QueryClient[] = []

beforeEach(() => {
  localStorage.clear()
  vi.resetModules()
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
})

afterEach(async () => {
  for (const client of clients.splice(0)) client.clear()
  const { useAuthStore } = await import('@/stores/auth-store')
  useAuthStore.getState().auth.reset()
  localStorage.clear()
})

async function loadSetupRoute(setupStatus: boolean, pathname = '/') {
  const { Route } = await import('../__root')
  const { api } = await import('@/lib/api')
  const { useAuthStore } = await import('@/stores/auth-store')
  useAuthStore.getState().auth.reset()
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { status: setupStatus } },
  })
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(queryClient)
  const children = ['/', '/about', '/setup'].map((path) =>
    createRoute({ getParentRoute: () => Route, path })
  )
  const router = createRouter({
    routeTree: Route.addChildren(children),
    history: createMemoryHistory({ initialEntries: [pathname] }),
    context: { queryClient },
  })
  return { router, get }
}

test('a stale setup flag from a previous instance cannot bypass setup after reload', async () => {
  localStorage.setItem('setup_status_checked', 'true')
  const { router, get } = await loadSetupRoute(false)
  await router.load()
  expect(get).toHaveBeenCalledWith('/api/setup', expect.anything())
  expect(router.state.location.pathname).toBe('/setup')
})

test('completed setup is reused during navigation but checked again in a new page session', async () => {
  const { router, get } = await loadSetupRoute(true)
  await router.load()
  await router.navigate({ to: '/about' })
  expect(get).toHaveBeenCalledTimes(1)
  expect(localStorage.getItem('setup_status_checked')).toBeNull()

  vi.resetModules()
  const reloaded = await loadSetupRoute(false)
  await reloaded.router.load()
  expect(reloaded.get).toHaveBeenCalledWith('/api/setup', expect.anything())
  expect(reloaded.router.state.location.pathname).toBe('/setup')
})

test('an unavailable setup endpoint is retried on the next navigation', async () => {
  const { router, get } = await loadSetupRoute(false)
  get.mockRejectedValueOnce(new Error('temporarily unavailable'))
  vi.spyOn(console, 'warn').mockImplementation(() => undefined)
  await router.load()
  expect(router.state.location.pathname).toBe('/')
  await router.navigate({ to: '/about' })
  expect(get).toHaveBeenCalledTimes(2)
  expect(router.state.location.pathname).toBe('/setup')
})

test('the setup screen does not query or redirect to itself', async () => {
  const { router, get } = await loadSetupRoute(false, '/setup')
  await router.load()
  expect(get).not.toHaveBeenCalled()
  expect(router.state.location.pathname).toBe('/setup')
})
