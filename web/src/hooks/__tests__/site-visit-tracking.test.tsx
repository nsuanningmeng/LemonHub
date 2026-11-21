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
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  redirect,
  RouterProvider,
} from '@tanstack/react-router'
import { act, render, screen } from '@testing-library/react'
import { StrictMode } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { useSiteVisitTracking } from '../use-site-visit-tracking'

function Site() {
  useSiteVisitTracking()
  return <Outlet />
}

function openSite() {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  const root = createRootRoute({ component: Site })
  const home = createRoute({
    getParentRoute: () => root,
    path: '/',
    component: () => <h1>Home</h1>,
  })
  const about = createRoute({
    getParentRoute: () => root,
    path: '/about',
    component: () => <h1>About</h1>,
  })
  const legacy = createRoute({
    getParentRoute: () => root,
    path: '/legacy',
    beforeLoad: () => {
      throw redirect({ to: '/about' })
    },
  })
  const router = createRouter({
    routeTree: root.addChildren([home, about, legacy]),
    history: createMemoryHistory({ initialEntries: ['/?code=private'] }),
  })
  render(
    <StrictMode>
      <RouterProvider router={router} />
    </StrictMode>
  )
  return router
}

afterEach(() => {
  vi.unstubAllGlobals()
})

test('counts a page open and completed path changes, including revisits, without sending the URL', async () => {
  const send = vi
    .fn<typeof fetch>()
    .mockResolvedValue(new Response(null, { status: 204 }))
  vi.stubGlobal('fetch', send)
  const router = openSite()
  await screen.findByRole('heading', { name: 'Home' })

  expect(send).toHaveBeenCalledTimes(1)
  await act(() => router.navigate({ to: '/', search: {}, hash: 'section' }))
  expect(send).toHaveBeenCalledTimes(1)
  expect(send).toHaveBeenCalledWith('/api/site-analytics/visit', {
    method: 'POST',
    headers: { 'X-Site-Visit': '1' },
    mode: 'same-origin',
    credentials: 'same-origin',
    referrerPolicy: 'no-referrer',
    keepalive: true,
  })

  await act(() => router.navigate({ to: '/about' }))
  expect(send).toHaveBeenCalledTimes(2)
  await act(() => router.navigate({ to: '/' }))
  expect(send).toHaveBeenCalledTimes(3)
})

test('counts only the displayed destination when preloading and following a redirect', async () => {
  const send = vi.fn<typeof fetch>().mockResolvedValue(new Response())
  vi.stubGlobal('fetch', send)
  const router = openSite()
  await screen.findByRole('heading', { name: 'Home' })

  await act(async () => {
    await router.preloadRoute({ to: '/about' })
  })
  expect(send).toHaveBeenCalledTimes(1)
  await act(() => router.navigate({ href: '/legacy' }))

  expect(screen.getByRole('heading', { name: 'About' })).toBeVisible()
  expect(send).toHaveBeenCalledTimes(2)
})

test('waits for the initial visitor cookie before counting a quick second navigation', async () => {
  let finishFirstVisit!: (response: Response) => void
  const firstVisit = new Promise<Response>((resolve) => {
    finishFirstVisit = resolve
  })
  const send = vi
    .fn<typeof fetch>()
    .mockReturnValueOnce(firstVisit)
    .mockResolvedValue(new Response())
  vi.stubGlobal('fetch', send)
  const router = openSite()
  await screen.findByRole('heading', { name: 'Home' })

  await act(() => router.navigate({ to: '/about' }))
  expect(screen.getByRole('heading', { name: 'About' })).toBeVisible()
  expect(send).toHaveBeenCalledTimes(1)

  await act(async () => {
    finishFirstVisit(new Response(null, { status: 204 }))
  })
  expect(send).toHaveBeenCalledTimes(2)
})

test('keeps rendering and records later visits after a network failure', async () => {
  const send = vi
    .fn<typeof fetch>()
    .mockRejectedValueOnce(new TypeError('Offline'))
    .mockResolvedValue(new Response())
  vi.stubGlobal('fetch', send)
  const router = openSite()
  await screen.findByRole('heading', { name: 'Home' })

  await act(() => router.navigate({ to: '/about' }))

  expect(screen.getByRole('heading', { name: 'About' })).toBeVisible()
  expect(send).toHaveBeenCalledTimes(2)
})

test.each([
  ['cookieEnabled', false],
  ['doNotTrack', '1'],
  ['globalPrivacyControl', true],
] as const)(
  'skips collection when %s expresses a privacy preference',
  async (key, value) => {
    const send = vi.fn<typeof fetch>()
    vi.stubGlobal('fetch', send)
    Object.defineProperty(navigator, key, { configurable: true, value })
    try {
      const router = openSite()
      await screen.findByRole('heading', { name: 'Home' })
      await act(() => router.navigate({ to: '/about' }))

      expect(send).not.toHaveBeenCalled()
    } finally {
      Reflect.deleteProperty(navigator, key)
    }
  }
)
