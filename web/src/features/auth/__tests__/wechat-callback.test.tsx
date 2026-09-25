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
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { Route as WeChatCallbackRoute } from '@/routes/(auth)/oauth'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

test('a copied WeChat code URL returns to sign-in without creating or exchanging a flow', async () => {
  const get = vi.spyOn(api, 'get')
  const post = vi.spyOn(api, 'post')
  const root = createRootRoute()
  const auth = createRoute({ getParentRoute: () => root, id: '(auth)' })
  const callback = createRoute({
    getParentRoute: () => auth,
    path: 'oauth',
    component: WeChatCallbackRoute.options.component,
  })
  const signIn = createRoute({
    getParentRoute: () => root,
    path: 'sign-in',
    component: () => <h1>Sign in</h1>,
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([callback]), signIn]),
    history: createMemoryHistory({
      initialEntries: ['/oauth?provider=wechat&code=attacker-code'],
    }),
  })

  render(<RouterProvider router={router} />)

  expect(
    await screen.findByRole('heading', { name: 'Sign in' })
  ).toBeInTheDocument()
  expect(get).not.toHaveBeenCalled()
  expect(post).not.toHaveBeenCalled()
})
