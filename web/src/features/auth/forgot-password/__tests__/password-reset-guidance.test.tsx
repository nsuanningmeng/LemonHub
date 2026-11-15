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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Toaster, toast } from 'sonner'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { ForgotPassword } from '../index'

const clients: QueryClient[] = []
afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  toast.dismiss()
  localStorage.clear()
})

test('password reset explains the original site and bound email, then gives a non-enumerating confirmation and prevents immediate resending', async () => {
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValue({ data: { success: true } })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  clients.push(client)
  client.setQueryData(['status'], {})
  const root = createRootRoute({
    component: () => (
      <>
        <ForgotPassword />
        <Toaster />
      </>
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
  expect(
    await screen.findByText(
      'Use the site where you registered and enter the email bound to your account to request a password reset link.'
    )
  ).toBeVisible()
  const user = userEvent.setup()
  const email = screen.getByRole('textbox', { name: 'Email' })
  await user.click(email)
  await user.paste('member@example.com')
  await user.click(screen.getByRole('button', { name: 'Send reset email' }))
  expect(
    await screen.findByText(
      'If this email is registered on this site, check your inbox for a password reset link. If no email arrives, contact the administrator.'
    )
  ).toBeVisible()
  expect(get).toHaveBeenCalledWith('/api/reset_password', {
    params: { email: 'member@example.com', turnstile: '' },
  })
  expect(email).toHaveValue('')
  expect(screen.getByRole('button', { name: /^Resend/ })).toBeDisabled()
})
