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
  createRouter,
  RouterProvider,
  type RouteComponent,
} from '@tanstack/react-router'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, test } from 'vitest'

import { useSystemConfigStore } from '@/stores/system-config-store'

import { AuthLayout } from '../auth-layout'
import { OAuthProviders } from '../components/oauth-providers'
import { ForgotPasswordForm } from '../forgot-password/components/forgot-password-form'
import { UserAuthForm } from '../sign-in/components/user-auth-form'

const systemName =
  'LemonHub Enterprise International Research and Engineering Platform'
const providerName =
  'ExtremelyLongOrganizationIdentityProviderNameWithoutAnySpaces'
const originalConfig = useSystemConfigStore.getState()
let queryClient: QueryClient

function AuthLayoutFixture() {
  return (
    <AuthLayout>
      <section aria-label='Account access'>
        <h2>Sign in</h2>
        <OAuthProviders
          status={{ oidc_enabled: true, oidc_display_name: providerName }}
        />
      </section>
    </AuthLayout>
  )
}

function PasswordFormFixture() {
  return (
    <AuthLayout>
      <UserAuthForm />
    </AuthLayout>
  )
}

function ForgotPasswordFixture() {
  return (
    <AuthLayout>
      <ForgotPasswordForm />
    </AuthLayout>
  )
}

beforeEach(() => {
  localStorage.clear()
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  queryClient.setQueryData(['status'], { password_login_enabled: true })
  useSystemConfigStore.getState().setConfig({ systemName })
  useSystemConfigStore.getState().setLoading(false)
})

afterEach(() => {
  cleanup()
  queryClient.clear()
  useSystemConfigStore.setState(originalConfig, true)
  localStorage.clear()
})

function renderAuthLayout(component: RouteComponent = AuthLayoutFixture) {
  const root = createRootRoute({ component })
  const router = createRouter({
    routeTree: root,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

test('long provider names wrap inside an expanding button without widening the authentication form', async () => {
  renderAuthLayout()

  const label = await screen.findByText(`Continue with ${providerName}`)
  const provider = screen.getByRole('button', {
    name: `Continue with ${providerName}`,
  })
  expect(provider).toHaveClass(
    'h-auto',
    'min-h-11',
    'min-w-0',
    'whitespace-normal'
  )
  expect(label).toHaveClass('min-w-0', '[overflow-wrap:anywhere]')
})

test('long branding stays bounded and authentication content reserves header space on short screens', async () => {
  renderAuthLayout()

  const branding = await screen.findByRole('heading', { name: systemName })
  const home = screen.getByRole('link', { name: `Logo ${systemName}` })
  expect(branding).toHaveClass('min-w-0', 'truncate')
  expect(branding).toHaveAttribute('title', systemName)
  expect(home).toHaveClass('right-4', 'sm:right-8', 'min-w-0')
  expect(home.parentElement).toHaveClass('min-h-svh', 'min-w-0')
  expect(home.parentElement).not.toHaveClass('h-svh')
  expect(
    screen.getByRole('region', { name: 'Account access' }).closest('.container')
  ).toHaveClass('pt-16', 'sm:pt-24', 'min-w-0')
})

test('password recovery links can wrap beneath translated password labels without overlap', async () => {
  renderAuthLayout(PasswordFormFixture)

  const recovery = await screen.findByRole('link', { name: 'Forgot password?' })
  const label = screen.getByText('Password', { selector: 'label' })
  expect(recovery.parentElement).toContainElement(label)
  expect(recovery.parentElement).toHaveClass(
    'flex',
    'flex-wrap',
    'gap-x-3',
    'gap-y-1'
  )
  expect(recovery).not.toHaveClass('absolute')
})

test('translated password reset actions wrap and grow instead of forcing a wider form', async () => {
  renderAuthLayout(ForgotPasswordFixture)

  const submit = await screen.findByRole('button', { name: 'Send reset email' })
  expect(submit).toHaveClass(
    'h-auto',
    'min-h-8',
    'whitespace-normal',
    '[overflow-wrap:anywhere]'
  )
})
