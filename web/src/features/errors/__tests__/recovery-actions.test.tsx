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
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, test } from 'vitest'

import { UnauthorisedError } from '../unauthorized-error'

afterEach(cleanup)

test('direct access to a protected page offers a sign-in destination and a readable status', async () => {
  const router = createRouter({
    routeTree: createRootRoute({ component: UnauthorisedError }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(<RouterProvider router={router} />)
  expect(
    screen.getByRole('heading', { name: 'Sign in required', level: 1 })
  ).toBeVisible()
  expect(screen.getByText('HTTP 401')).toBeVisible()
  expect(screen.getByRole('link', { name: 'Sign in' })).toHaveAttribute(
    'href',
    '/sign-in'
  )
  expect(screen.getByRole('button', { name: 'Back to Home' })).toBeVisible()
  expect(screen.queryByRole('button', { name: 'Go Back' })).toBeNull()
})
