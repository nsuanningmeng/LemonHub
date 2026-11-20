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
  type RouteComponent,
} from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'

import { GeneralError } from '../general-error'
import { NotFoundError } from '../not-found-error'
import { UnauthorisedError } from '../unauthorized-error'

function renderError(component: RouteComponent) {
  const root = createRootRoute()
  const home = createRoute({
    getParentRoute: () => root,
    path: '/',
    component: () => <h1>Workspace home</h1>,
  })
  const signIn = createRoute({
    getParentRoute: () => root,
    path: '/sign-in',
    component: () => <h1>Account sign-in</h1>,
  })
  const error = createRoute({
    getParentRoute: () => root,
    path: '/problem',
    component,
  })
  const router = createRouter({
    routeTree: root.addChildren([home, signIn, error]),
    history: createMemoryHistory({ initialEntries: ['/', '/problem'] }),
  })
  render(<RouterProvider router={router} />)
}

test('an unavailable page presents a readable recovery message and returns home', async () => {
  const user = userEvent.setup()
  renderError(NotFoundError)
  expect(
    await screen.findByRole('heading', {
      name: 'Page not found',
    })
  ).toBeVisible()
  expect(screen.getByText('404')).toBeVisible()
  expect(screen.getByRole('main').querySelector(':scope > .border')).toBeNull()
  expect(screen.queryByText('Connection notice')).not.toBeInTheDocument()
  expect(screen.getByRole('main').parentElement).toHaveClass('min-h-svh')
  await user.click(screen.getByRole('button', { name: 'Back to Home' }))
  expect(
    await screen.findByRole('heading', { name: 'Workspace home' })
  ).toBeVisible()
})

test('an unauthenticated visitor can go directly to sign-in from the recovery surface', async () => {
  const user = userEvent.setup()
  renderError(UnauthorisedError)
  await user.click(await screen.findByRole('link', { name: 'Sign in' }))
  expect(
    await screen.findByRole('heading', { name: 'Account sign-in' })
  ).toBeVisible()
})

test('a rate-limited request explains when to retry without sending users to an unrelated support site', async () => {
  renderError(() => <GeneralError error={{ response: { status: 429 } }} />)
  expect(
    await screen.findByRole('heading', { name: 'Too many requests' })
  ).toBeVisible()
  expect(
    screen.getByText('Please wait a moment before trying again.')
  ).toBeVisible()
  expect(
    screen.getByText(
      'If the problem continues, contact your workspace administrator.'
    )
  ).toBeVisible()
  expect(
    screen.queryByRole('link', { name: 'Report an issue' })
  ).not.toBeInTheDocument()
})

test('back from a recovery surface restores the previous route', async () => {
  const user = userEvent.setup()
  renderError(NotFoundError)
  await user.click(await screen.findByRole('button', { name: 'Go Back' }))
  expect(
    await screen.findByRole('heading', { name: 'Workspace home' })
  ).toBeVisible()
})

test('a minimal recovery surface keeps the error message without full-page branding or navigation', async () => {
  renderError(() => (
    <GeneralError minimal error={{ response: { status: 500 } }} />
  ))
  expect(
    await screen.findByRole('heading', {
      name: 'This page could not be loaded.',
    })
  ).toBeVisible()
  expect(screen.queryByText('500')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Back to Home' })
  ).not.toBeInTheDocument()
  expect(screen.queryByRole('banner')).not.toBeInTheDocument()
})
