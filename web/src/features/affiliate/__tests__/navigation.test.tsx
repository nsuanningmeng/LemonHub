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
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test } from 'vitest'

import { Affiliate } from '..'

afterEach(cleanup)

test('the affiliate actions retain their destinations and the FAQ is available by keyboard', async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { enabled: false } },
  })
  const router = createRouter({
    routeTree: createRootRoute({ component: Affiliate }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  const user = userEvent.setup()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  const main = within(screen.getByRole('main'))
  expect(main.getByRole('link', { name: 'Learn More' })).toHaveAttribute(
    'href',
    '#affiliate-advantages'
  )
  for (const link of main.getAllByRole('link', { name: 'Apply Now' })) {
    expect(link).toHaveAttribute('href', '/contact')
  }
  expect(
    main.getByRole('link', { name: 'Get my referral link' })
  ).toHaveAttribute('href', '/referral')
  const question = main.getByRole('button', {
    name: 'Can I customize the brand and pricing?',
  })
  question.focus()
  await user.keyboard('{Enter}')
  expect(question).toHaveAttribute('aria-expanded', 'true')
  client.clear()
})
