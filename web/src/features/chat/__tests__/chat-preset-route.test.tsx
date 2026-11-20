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
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, test, vi } from 'vitest'

import { createLocaleI18n } from '@/i18n/__tests__/helpers'
import { api } from '@/lib/api'
import { Route } from '@/routes/_authenticated/chat/$chatId'
import { useAuthStore } from '@/stores/auth-store'

const initialAuth = useAuthStore.getState()

afterEach(() => {
  useAuthStore.setState(initialAuth, true)
  cleanup()
  localStorage.removeItem('status')
  vi.restoreAllMocks()
})

function renderPresetRoute(chatId: string, locale = i18next) {
  const root = createRootRoute()
  const auth = createRoute({ getParentRoute: () => root, id: '_authenticated' })
  const route = createRoute({
    getParentRoute: () => auth,
    path: 'chat/$chatId',
    component: Route.options.component,
  })
  const dashboard = createRoute({
    getParentRoute: () => root,
    path: '/dashboard',
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([route]), dashboard]),
    history: createMemoryHistory({ initialEntries: [`/chat/${chatId}`] }),
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={locale}>
        <RouterProvider router={router} />
      </I18nextProvider>
    </QueryClientProvider>
  )
}

test('shows loading until the configuration confirms that a preset is absent', async () => {
  localStorage.setItem('status', JSON.stringify({ chats: [] }))
  let release!: () => void
  const pending = new Promise<void>((resolve) => {
    release = resolve
  })
  vi.spyOn(api, 'get').mockImplementation(async () => {
    await pending
    return { data: { success: true, data: { chats: [] } } }
  })
  renderPresetRoute('3')
  expect(await screen.findByRole('status')).toHaveTextContent(
    'Loading configuration'
  )
  expect(screen.queryByText('Chat preset not found')).toBeNull()
  release()
  expect(
    await screen.findByRole('heading', { name: 'Chat preset not found' })
  ).toBeVisible()
  expect(
    screen.getByRole('link', { name: 'Return to dashboard' })
  ).toHaveAttribute('href', '/dashboard')
})

test('failed configuration can be retried and embeds the configured client by its source index', async () => {
  const get = vi
    .spyOn(api, 'get')
    .mockRejectedValueOnce(new Error('Unavailable'))
    .mockResolvedValueOnce({
      data: {
        success: true,
        data: { chats: [{}, { 'Support chat': 'https://chat.example.test/' }] },
      },
    })
  renderPresetRoute('1')
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Unable to open chat'
  )
  expect(screen.queryByText('Chat preset not found')).toBeNull()
  await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
  const frame = await screen.findByTitle('Support chat')
  expect(frame).toHaveAttribute('src', 'https://chat.example.test/')
  expect(frame).toHaveAttribute('allow', 'camera; microphone')
  expect(frame).not.toHaveAttribute('sandbox')
  expect(get.mock.calls.map(([url]) => url)).toEqual([
    '/api/status',
    '/api/status',
  ])
})

test('localizes known key errors, keeps the server message, and retries the active chat key', async () => {
  const locale = await createLocaleI18n('zhCN')
  useAuthStore.getState().auth.setUser({ id: 1, username: 'tester', role: 1 })
  vi.spyOn(api, 'get')
    .mockResolvedValueOnce({
      data: {
        success: true,
        data: {
          chats: [{ Support: 'https://chat.example.test/?token={key}' }],
        },
      },
    })
    .mockResolvedValueOnce({ data: { success: true, data: { items: [] } } })
    .mockResolvedValueOnce({
      data: { success: false, message: 'Account service stopped' },
    })
    .mockResolvedValueOnce({
      data: { success: true, data: { items: [{ id: 2, status: 1 }] } },
    })
  vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, data: { key: 'test-chat-token' } },
  })
  renderPresetRoute('0', locale)
  expect(await screen.findByRole('alert')).toHaveTextContent(
    '当前没有已启用的 API 密钥，请先创建或启用密钥'
  )
  expect(screen.getByRole('link', { name: 'API 密钥' })).toHaveAttribute(
    'href',
    '/keys'
  )
  await userEvent.click(screen.getByRole('button', { name: '重试' }))
  expect(await screen.findByText('Account service stopped')).toBeVisible()
  await userEvent.click(screen.getByRole('button', { name: '重试' }))
  expect(await screen.findByTitle('Support')).toHaveAttribute(
    'src',
    'https://chat.example.test/?token=sk-test-chat-token'
  )
})
