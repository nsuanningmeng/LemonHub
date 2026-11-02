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
import {
  QueryClient,
  QueryClientProvider,
  useQuery,
} from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterContextProvider,
} from '@tanstack/react-router'
import { act, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { DirectionProvider } from '@/context/direction-provider'
import { FontProvider } from '@/context/font-provider'
import { ThemeProvider } from '@/context/theme-provider'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { AuthenticatedLayout } from '../authenticated-layout'

afterEach(() => {
  useAuthStore.getState().auth.reset()
  localStorage.clear()
})

test('sign-out unmounts protected pages and stops their invalidated queries', async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  client.setQueryData(['status'], {})
  useAuthStore.getState().auth.setUser({ id: 1, username: 'member', role: 1 })
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/notice') {
      return { data: { success: true, data: '' } } as never
    }
    return { data: { success: true, data: { items: [] } } } as never
  })
  function ProtectedPage() {
    useQuery({
      queryKey: ['protected-page'],
      queryFn: () => api.get('/api/protected-page'),
    })
    return <h1>Protected account</h1>
  }
  const router = createRouter({
    routeTree: createRootRoute(),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  const rendered = render(
    <QueryClientProvider client={client}>
      <ThemeProvider>
        <FontProvider>
          <DirectionProvider>
            <RouterContextProvider router={router}>
              <AuthenticatedLayout>
                <ProtectedPage />
              </AuthenticatedLayout>
            </RouterContextProvider>
          </DirectionProvider>
        </FontProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
  expect(
    await screen.findByRole('heading', { name: 'Protected account' })
  ).toBeVisible()
  expect(get).toHaveBeenCalledWith('/api/protected-page')
  await act(async () => {
    useAuthStore.getState().auth.reset()
  })
  expect(
    screen.queryByRole('heading', { name: 'Protected account' })
  ).toBeNull()
  get.mockClear()
  await act(async () => {
    await client.invalidateQueries({ queryKey: ['protected-page'] })
  })
  expect(get).not.toHaveBeenCalledWith('/api/protected-page')
  rendered.unmount()
  client.clear()
})
