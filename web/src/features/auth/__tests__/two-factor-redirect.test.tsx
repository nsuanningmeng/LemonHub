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
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore, type AuthBundle } from '@/stores/auth-store'

import { Otp } from '../otp'
import { UserAuthForm } from '../sign-in/components/user-auth-form'

const originalAuth = useAuthStore.getState()
const clients: QueryClient[] = []
afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  useAuthStore.setState(originalAuth, true)
  localStorage.clear()
})

test('two-factor authentication preserves the requested internal destination and passes its flow token with the backup code', async () => {
  const bundle: AuthBundle = {
    access_token: 'test-access',
    token_type: 'Bearer',
    access_expires_at: 9999999999,
    user: { id: 8, username: 'tester', role: 1 },
    session: {
      sid: 'test-session',
      current: true,
      login_method: 'password',
      ip: '',
      user_agent: '',
      created_at: 1,
      last_active_at: 1,
      expires_at: 9999999999,
    },
  }
  const post = vi.spyOn(api, 'post').mockImplementation(async (url) => ({
    data:
      url === '/api/user/login'
        ? {
            success: true,
            data: { require_2fa: true, flow_token: 'challenge-token' },
          }
        : { success: true, data: bundle },
  }))
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  clients.push(client)
  client.setQueryData(['status'], { password_login_enabled: true })
  const root = createRootRoute()
  const auth = createRoute({ getParentRoute: () => root, id: '(auth)' })
  const signIn = createRoute({
    getParentRoute: () => auth,
    path: 'sign-in',
    component: () => <UserAuthForm redirectTo='/pricing?view=table' />,
  })
  const otp = createRoute({
    getParentRoute: () => auth,
    path: 'otp',
    component: Otp,
    validateSearch: (search) => ({
      redirect:
        typeof search.redirect === 'string' ? search.redirect : undefined,
    }),
  })
  const pricing = createRoute({
    getParentRoute: () => root,
    path: 'pricing',
    component: () => <p>Protected catalog</p>,
  })
  const dashboard = createRoute({
    getParentRoute: () => root,
    path: 'dashboard',
    component: () => <p>Dashboard</p>,
  })
  const router = createRouter({
    routeTree: root.addChildren([
      auth.addChildren([signIn, otp]),
      pricing,
      dashboard,
    ]),
    history: createMemoryHistory({ initialEntries: ['/sign-in'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  const user = userEvent.setup()
  await user.type(await screen.findByLabelText('Username or Email'), 'tester')
  await user.type(screen.getByLabelText('Password'), 'password123')
  await user.click(screen.getByRole('button', { name: 'Sign in' }))
  await screen.findByRole('heading', { name: 'Two-factor Authentication' })
  expect(router.state.location.search).toEqual({
    redirect: '/pricing?view=table',
  })
  expect(screen.getByRole('link', { name: 'Re-login' })).toHaveAttribute(
    'href',
    '/sign-in?redirect=%2Fpricing%3Fview%3Dtable'
  )
  await user.click(screen.getByRole('button', { name: 'Use backup code' }))
  await user.type(screen.getByLabelText('Backup Code'), 'CAWD-OQDV')
  await user.click(screen.getByRole('button', { name: 'Verify and Sign In' }))
  await screen.findByText('Protected catalog')
  await waitFor(() =>
    expect(router.state.location.href).toBe('/pricing?view=table')
  )
  expect(post).toHaveBeenLastCalledWith(
    '/api/user/login/2fa',
    { code: 'CAWDOQDV', flow_token: 'challenge-token' },
    { skipAuthRefresh: true }
  )
})
