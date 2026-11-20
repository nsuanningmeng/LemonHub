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
  RouterProvider,
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
} from '@tanstack/react-router'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { Playground } from '..'
import { loadMessages, saveMessages } from '../lib/storage/storage'
import type { Message } from '../types'

const originalAdapter = api.defaults.adapter

afterEach(() => {
  api.defaults.adapter = originalAdapter
  vi.restoreAllMocks()
  localStorage.clear()
  useAuthStore.getState().auth.reset()
})

function Destination() {
  return <p>Destination page</p>
}

function renderPlayground() {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  api.defaults.adapter = async (config) => ({
    config,
    status: 200,
    statusText: 'OK',
    headers: {},
    data: {
      success: true,
      data:
        config.url === '/api/user/models'
          ? ['gpt-4o']
          : { default: { desc: 'Default', ratio: 1 } },
    },
  })
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const rootRoute = createRootRoute()
  const chatRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/',
    component: Playground,
  })
  const destinationRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/about',
    component: Destination,
  })
  const router = createRouter({
    history: createMemoryHistory({ initialEntries: ['/'] }),
    routeTree: rootRoute.addChildren([chatRoute, destinationRoute]),
  })
  const view = render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return { view, queryClient, router }
}

test('the playground shows a recoverable save failure, clears it after retry, and removes it when accounts change', async () => {
  const user = userEvent.setup()
  const accountA: Message[] = [
    {
      key: 'a-first',
      from: 'user',
      versions: [{ id: 'a-first-v', content: 'Account A first prompt' }],
    },
    {
      key: 'a-second',
      from: 'user',
      versions: [{ id: 'a-second-v', content: 'Account A second prompt' }],
    },
  ]
  const accountB: Message[] = [
    {
      key: 'b',
      from: 'user',
      versions: [{ id: 'b-v', content: 'Account B private prompt' }],
    },
  ]
  localStorage.clear()
  useAuthStore.getState().auth.setUser({ id: 1, username: 'a', role: 1 })
  saveMessages(accountA, 1)
  saveMessages(accountB, 2)
  const { view, queryClient } = renderPlayground()
  await screen.findByText('Account A first prompt')
  vi.spyOn(console, 'error').mockImplementation(() => undefined)
  const write = vi
    .spyOn(Storage.prototype, 'setItem')
    .mockImplementation(() => {
      throw new DOMException('Storage is full', 'QuotaExceededError')
    })

  await user.click(screen.getAllByLabelText('Delete')[0])

  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Conversation could not be saved in this browser.'
  )
  expect(
    screen.getByRole('button', { name: 'Download conversation' })
  ).toBeEnabled()
  expect(screen.getByText('Account A second prompt')).toBeVisible()
  expect(loadMessages(1)).toEqual(accountA)
  write.mockRestore()
  await user.click(screen.getByRole('button', { name: 'Retry saving' }))
  await waitFor(() =>
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  )
  expect(loadMessages(1)).toEqual(accountA.slice(1))

  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
    throw new DOMException('Storage is full', 'QuotaExceededError')
  })
  await user.click(screen.getAllByLabelText('Delete')[0])
  await screen.findByRole('alert')
  act(() =>
    useAuthStore.getState().auth.setUser({ id: 2, username: 'b', role: 1 })
  )

  await screen.findByText('Account B private prompt')
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Download conversation' })
  ).not.toBeInTheDocument()
  expect(screen.queryByText('Account A second prompt')).not.toBeInTheDocument()
  expect(loadMessages(2)).toEqual(accountB)
  view.unmount()
  queryClient.clear()
}, 15_000)

const navigationHistory: Message[] = [
  {
    key: 'first',
    from: 'user',
    versions: [{ id: 'first-v', content: 'First prompt' }],
  },
  {
    key: 'remaining',
    from: 'user',
    versions: [{ id: 'remaining-v', content: 'Remaining prompt' }],
  },
]

test('navigation during the save debounce retains the conversation when persistence fails until retry succeeds', async () => {
  const user = userEvent.setup()
  useAuthStore.getState().auth.setUser({ id: 1, username: 'a', role: 1 })
  saveMessages(navigationHistory, 1)
  const { view, queryClient, router } = renderPlayground()
  await screen.findByText('First prompt')
  vi.spyOn(console, 'error').mockImplementation(() => undefined)
  const write = vi
    .spyOn(Storage.prototype, 'setItem')
    .mockImplementation(() => {
      throw new DOMException('Storage is full', 'QuotaExceededError')
    })
  fireEvent.click(screen.getAllByLabelText('Delete')[0])
  void router.navigate({ to: '/about' })

  await screen.findByRole('alertdialog')
  expect(router.history.location.pathname).toBe('/')
  expect(screen.getByText('Remaining prompt')).toBeInTheDocument()
  expect(loadMessages(1)).toEqual(navigationHistory)
  await user.click(screen.getByRole('button', { name: 'Stay' }))
  await waitFor(() =>
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  )
  expect(router.history.location.pathname).toBe('/')
  expect(
    screen.getByRole('button', { name: 'Download conversation' })
  ).toBeEnabled()

  write.mockRestore()
  await user.click(screen.getByRole('button', { name: 'Retry saving' }))
  await act(async () => {
    await router.navigate({ to: '/about' })
  })

  expect(await screen.findByText('Destination page')).toBeVisible()
  expect(screen.queryByText('Remaining prompt')).not.toBeInTheDocument()
  expect(loadMessages(1)).toEqual(navigationHistory.slice(1))
  view.unmount()
  queryClient.clear()
}, 15_000)

test('navigation during the save debounce flushes successfully before leaving without a confirmation', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'a', role: 1 })
  saveMessages(navigationHistory, 1)
  const { view, queryClient, router } = renderPlayground()
  await screen.findByText('First prompt')
  fireEvent.click(screen.getAllByLabelText('Delete')[0])

  await act(async () => {
    await router.navigate({ to: '/about' })
  })

  expect(await screen.findByText('Destination page')).toBeVisible()
  expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  expect(loadMessages(1)).toEqual(navigationHistory.slice(1))
  view.unmount()
  queryClient.clear()
}, 15_000)

test('navigation with a failed save leaves only after explicitly discarding the unsaved changes', async () => {
  const user = userEvent.setup()
  useAuthStore.getState().auth.setUser({ id: 1, username: 'a', role: 1 })
  saveMessages(navigationHistory, 1)
  const { view, queryClient, router } = renderPlayground()
  await screen.findByText('First prompt')
  vi.spyOn(console, 'error').mockImplementation(() => undefined)
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
    throw new DOMException('Storage is full', 'QuotaExceededError')
  })
  fireEvent.click(screen.getAllByLabelText('Delete')[0])
  await screen.findByRole('alert')
  const navigation = router.navigate({ to: '/about' })

  await screen.findByRole('alertdialog')
  expect(router.history.location.pathname).toBe('/')
  await user.click(screen.getByRole('button', { name: 'Discard and leave' }))
  await navigation

  expect(await screen.findByText('Destination page')).toBeVisible()
  expect(loadMessages(1)).toEqual(navigationHistory)
  view.unmount()
  queryClient.clear()
}, 15_000)
