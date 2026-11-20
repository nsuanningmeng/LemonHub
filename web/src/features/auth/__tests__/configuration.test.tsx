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
} from '@tanstack/react-router'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { AuthLayout } from '../auth-layout'
import { TermsFooter } from '../components/terms-footer'
import { UserAuthForm } from '../sign-in/components/user-auth-form'
import { SignUpForm } from '../sign-up/components/sign-up-form'
import type { SystemStatus } from '../types'

const clients: QueryClient[] = []
afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  localStorage.clear()
  vi.unstubAllGlobals()
})

async function renderConfiguration(
  status: SystemStatus,
  mode: 'sign-in' | 'sign-up' = 'sign-in'
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  clients.push(client)
  client.setQueryData(['status'], status)
  const root = createRootRoute({
    component: () => (
      <AuthLayout>
        {mode === 'sign-up' ? <SignUpForm /> : <UserAuthForm />}
        <TermsFooter status={status} variant={mode} />
      </AuthLayout>
    ),
  })
  const router = createRouter({
    routeTree: root,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  await screen.findByRole('region', { name: 'Authentication' })
  return userEvent.setup()
}

test.each([{ enabled: false }, { enabled: true, requireAuth: true }])(
  'restricted pricing %j removes the catalog link from the authentication header',
  async (pricing) => {
    await renderConfiguration({
      HeaderNavModules: JSON.stringify({ pricing }),
    })
    expect(
      screen.queryByRole('link', { name: 'Explore models' })
    ).not.toBeInTheDocument()
  }
)

test('OAuth-only login hides password and recovery controls while consent gates the enabled provider', async () => {
  const user = await renderConfiguration({
    password_login_enabled: false,
    github_oauth: true,
    user_agreement_enabled: true,
  })
  expect(screen.queryByLabelText('Username or Email')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('link', { name: 'Forgot password?' })
  ).not.toBeInTheDocument()
  expect(
    screen.getByRole('button', { name: /Continue with GitHub/ })
  ).toBeDisabled()
  await user.click(screen.getByRole('checkbox'))
  expect(
    screen.getByRole('button', { name: /Continue with GitHub/ })
  ).toBeEnabled()
})

test('disabled legal policies and login providers do not leave inactive controls on the password form', async () => {
  await renderConfiguration({
    password_login_enabled: true,
    passkey_login: false,
    github_oauth: false,
    user_agreement_enabled: false,
    privacy_policy_enabled: false,
  })
  expect(screen.getByLabelText('Username or Email')).toBeVisible()
  expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('link', { name: 'Privacy Policy' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Sign in with Passkey' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: /Continue with GitHub/ })
  ).not.toBeInTheDocument()
})

test('enabled passkey on an unsupported device explains why its action is unavailable', async () => {
  vi.stubGlobal('PublicKeyCredential', undefined)
  await renderConfiguration({
    password_login_enabled: false,
    passkey_login: true,
  })
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Sign in with Passkey' })
    ).toBeDisabled()
  )
  expect(
    screen.getByText('Passkey is not supported on this device.')
  ).toBeVisible()
})

test.each([{ register_enabled: false }, { self_use_mode_enabled: true }])(
  'unavailable public registration %j removes credential and OAuth registration actions',
  async (configuration) => {
    await renderConfiguration(
      { ...configuration, github_oauth: true },
      'sign-up'
    )
    expect(screen.queryByLabelText('Username')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Create account' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /Continue with GitHub/ })
    ).not.toBeInTheDocument()
    expect(
      screen.getByText('Registration is currently unavailable.')
    ).toBeVisible()
  }
)

test('OAuth-only registration hides password controls while retaining consent and the enabled provider', async () => {
  const user = await renderConfiguration(
    {
      register_enabled: true,
      password_register_enabled: false,
      github_oauth: true,
      privacy_policy_enabled: true,
    },
    'sign-up'
  )
  expect(screen.queryByLabelText('Username')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Create account' })
  ).not.toBeInTheDocument()
  expect(
    screen.getByRole('button', { name: /Continue with GitHub/ })
  ).toBeDisabled()
  await user.click(screen.getByRole('checkbox'))
  expect(
    screen.getByRole('button', { name: /Continue with GitHub/ })
  ).toBeEnabled()
})

test('disabling OAuth registration leaves password registration available without provider actions', async () => {
  await renderConfiguration(
    {
      register_enabled: true,
      password_register_enabled: true,
      oauth_register_enabled: false,
      github_oauth: true,
    },
    'sign-up'
  )
  expect(screen.getByLabelText('Username')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Create account' })).toBeEnabled()
  expect(
    screen.queryByRole('button', { name: /Continue with GitHub/ })
  ).not.toBeInTheDocument()
})

test('enabling passkey on a supported browser keeps legal consent required before the action becomes available', async () => {
  vi.stubGlobal('PublicKeyCredential', {
    isConditionalMediationAvailable: async () => true,
  })
  const user = await renderConfiguration({
    password_login_enabled: false,
    passkey_login: true,
    user_agreement_enabled: true,
  })
  const passkey = screen.getByRole('button', { name: 'Sign in with Passkey' })
  expect(passkey).toBeDisabled()
  await user.click(screen.getByRole('checkbox'))
  await waitFor(() => expect(passkey).toBeEnabled())
  expect(
    screen.queryByText('Passkey is not supported on this device.')
  ).not.toBeInTheDocument()
})

test('disabling every sign-in method displays an explanation instead of an empty authentication form', async () => {
  await renderConfiguration({
    password_login_enabled: false,
    passkey_login: false,
  })
  expect(screen.getByRole('status')).toHaveTextContent(
    'Sign-in is currently unavailable. Please contact support.'
  )
  expect(
    within(screen.getByRole('region', { name: 'Authentication' })).queryByRole(
      'button'
    )
  ).not.toBeInTheDocument()
})
