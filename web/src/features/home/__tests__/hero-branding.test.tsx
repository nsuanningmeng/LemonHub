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
import { expect, test } from 'vitest'

import { Hero } from '../components/sections/hero'

test('a configured tenant sees its own hero copy, endpoint and documentation alongside the signed-in action', async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { enabled: false } },
  })
  client.setQueryData(['status'], {
    system_name: 'Tenant Gateway',
    home_badge: 'Research tenant',
    home_title_line1: 'Research models',
    home_title_line2: 'for the lab',
    docs_link: '/guide',
    server_address: 'https://models.example.test/',
  })
  const router = createRouter({
    routeTree: createRootRoute({ component: () => <Hero isAuthenticated /> }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  expect(await screen.findByRole('heading', { level: 1 })).toHaveAccessibleName(
    'Research models for the lab'
  )
  expect(screen.getByText('Research tenant')).toBeVisible()
  expect(screen.getByText('https://models.example.test/v1')).toBeVisible()
  expect(screen.getByRole('link', { name: 'Go to Dashboard' })).toHaveAttribute(
    'href',
    '/dashboard'
  )
  expect(screen.getByRole('link', { name: 'Docs' })).toHaveAttribute(
    'href',
    '/guide'
  )
  client.clear()
})
