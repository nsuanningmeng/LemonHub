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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { SidebarMenu, SidebarProvider } from '@/components/ui/sidebar'
import { api } from '@/lib/api'

import { ChatPresetsItem } from '../chat-presets-item'

let client: QueryClient

function ChatPresetsFixture() {
  return (
    <SidebarProvider>
      <SidebarMenu>
        <ChatPresetsItem
          item={{ type: 'chat-presets', title: 'Chat presets' }}
        />
      </SidebarMenu>
    </SidebarProvider>
  )
}

beforeEach(() => {
  localStorage.clear()
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  client.setQueryData(['status'], {
    server_address: 'https://gateway.example',
    chats: [
      { Chatbox: 'chatbox://api/providers/import?config={chatboxConfig}' },
    ],
  })
})

afterEach(() => {
  client.clear()
  localStorage.clear()
})

it('fetches the enabled key before opening a selected Chatbox preset', async () => {
  const user = userEvent.setup()
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  const open = vi.spyOn(window, 'open').mockReturnValue(null)
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        items: [
          { id: 3, status: 2 },
          { id: 8, status: 1 },
        ],
      },
    },
  })
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true, data: { key: 'chosen-key' } } })
  const router = createRouter({
    routeTree: createRootRoute({ component: ChatPresetsFixture }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  await user.click(await screen.findByRole('button', { name: 'Chat presets' }))
  await user.click(await screen.findByText('Chatbox'))

  await waitFor(() => expect(post).toHaveBeenCalledWith('/api/token/8/key'))
  expect(get).toHaveBeenCalledWith('/api/token/?p=1&size=50')
  expect(open).toHaveBeenCalledTimes(1)
  const url = new URL(String(open.mock.calls[0]?.[0]))
  expect(JSON.parse(atob(url.searchParams.get('config') ?? ''))).toMatchObject({
    settings: { apiKey: 'sk-chosen-key', apiHost: 'https://gateway.example' },
  })
})
