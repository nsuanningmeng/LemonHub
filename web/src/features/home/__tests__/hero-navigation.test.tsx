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
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, test } from 'vitest'

import { Hero } from '../components/sections/hero'

let client: QueryClient

beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { enabled: false, retry: false } },
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  localStorage.clear()
})

async function renderHero(isAuthenticated: boolean) {
  const router = createRouter({
    routeTree: createRootRoute({
      component: () => <Hero isAuthenticated={isAuthenticated} />,
    }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

test('signed-out visitors can create a workspace or explore models from the hero', async () => {
  await renderHero(false)
  expect(
    await screen.findByRole('link', { name: 'Start building' })
  ).toHaveAttribute('href', '/sign-up')
  expect(
    screen.getByRole('link', { name: 'Explore the models' })
  ).toHaveAttribute('href', '/pricing')
})

test('signed-in visitors return to their workspace instead of registration', async () => {
  await renderHero(true)
  expect(
    await screen.findByRole('link', { name: 'Open your workspace' })
  ).toHaveAttribute('href', '/dashboard')
  expect(
    screen.queryByRole('link', { name: 'Start building' })
  ).not.toBeInTheDocument()
})

test('configured hero copy and documentation survive the new layout', async () => {
  client.setQueryData(['status'], {
    home_badge: 'Custom workspace',
    home_title_line1: 'Build together',
    home_title_line2: 'On your terms',
    docs_link: 'https://example.test/docs',
  })
  await renderHero(false)
  expect(await screen.findByRole('heading', { level: 1 })).toHaveTextContent(
    'Build togetherOn your terms'
  )
  expect(screen.getByText('Custom workspace')).toBeVisible()
  expect(screen.getByRole('link', { name: 'Docs' })).toHaveAttribute(
    'href',
    'https://example.test/docs'
  )
  expect(screen.getByRole('link', { name: 'Docs' })).toHaveAttribute(
    'rel',
    'noopener noreferrer'
  )
})
