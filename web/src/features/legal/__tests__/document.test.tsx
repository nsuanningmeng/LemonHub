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
import { cleanup, render, screen, within } from '@testing-library/react'
import { afterEach, expect, test } from 'vitest'

import { PrivacyPolicy } from '../privacy-policy'

afterEach(cleanup)

async function renderPolicy(content: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { enabled: false } },
  })
  client.setQueryData(['privacy-policy'], { data: content, success: true })
  const router = createRouter({
    routeTree: createRootRoute({ component: PrivacyPolicy }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return within(screen.getByRole('main'))
}

test('an empty policy identifies the document and explains its availability', async () => {
  const page = await renderPolicy('')
  expect(
    page.getByRole('heading', { level: 1, name: 'Privacy Policy' })
  ).toBeVisible()
  expect(
    page.getByText('The administrator has not configured a privacy policy yet.')
  ).toBeVisible()
  expect(
    page.queryByRole('link', { name: 'View document' })
  ).not.toBeInTheDocument()
})

test('an external policy keeps its configured link in a separate tab', async () => {
  const page = await renderPolicy('https://example.test/privacy')
  const link = page.getByRole('link', { name: 'View document' })
  expect(link).toHaveAttribute('href', 'https://example.test/privacy')
  expect(link).toHaveAttribute('target', '_blank')
})
