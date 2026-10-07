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
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import type { Channel } from '../../types'
import { ChannelsPrimaryButtons } from '../channels-primary-buttons'
import { ChannelsProvider } from '../channels-provider'
import { ChannelsTable } from '../channels-table'

function channel(id: number, name: string): Channel {
  return {
    id,
    name,
    tag: 'production',
    type: 1,
    key: '',
    status: 1,
    created_time: 0,
    test_time: 0,
    response_time: 0,
    other: '',
    balance: 0,
    balance_updated_time: 0,
    models: 'gpt-4o',
    group: 'default',
    used_quota: 0,
    other_info: '',
    remark: '',
    max_input_tokens: 0,
    settings: '{}',
    channel_info: {
      is_multi_key: false,
      multi_key_size: 0,
      multi_key_polling_index: 0,
      multi_key_mode: 'random',
    },
  }
}
const channels = [
  channel(101, 'selected-one'),
  channel(202, 'unselected-two'),
  channel(303, 'selected-three'),
]
const clients: QueryClient[] = []
const initialAuth = useAuthStore.getState().auth
beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('channels:view-mode', 'table')
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  useAuthStore.getState().auth.setUser({ id: 1, username: 'admin', role: 100 })
})
afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  localStorage.clear()
  useAuthStore.setState({ auth: initialAuth })
})
function ChannelPage() {
  return (
    <ChannelsProvider>
      <ChannelsPrimaryButtons />
      <ChannelsTable />
    </ChannelsProvider>
  )
}
async function renderChannelsPage() {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/channel') {
      return {
        data: {
          success: true,
          data: { items: channels, total: 3, type_counts: { 1: 3 } },
        },
      }
    }
    if (url === '/api/group/') {
      return { data: { success: true, data: ['default'] } }
    }
    return { data: { success: true, data: {} } }
  })
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true, data: 2 } })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  const root = createRootRoute()
  const auth = createRoute({ getParentRoute: () => root, id: '_authenticated' })
  const route = createRoute({
    getParentRoute: () => auth,
    path: 'channels/',
    component: ChannelPage,
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([route])]),
    history: createMemoryHistory({ initialEntries: ['/channels/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  await screen.findByText('selected-one')
  return { user: userEvent.setup(), post }
}
async function selectGroupedChildren(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('switch', { name: 'Batch Operations' }))
  await user.click(screen.getByRole('switch', { name: 'Tag Mode' }))
  const group = await screen.findByRole('row', { name: /Tag：production/ })
  expect(within(group).queryByRole('checkbox')).not.toBeInTheDocument()
  await user.click(within(group).getAllByRole('button')[0])
  for (const name of ['selected-one', 'selected-three']) {
    const row = await screen.findByRole('row', { name: new RegExp(name) })
    await user.click(within(row).getByRole('checkbox', { name: 'Select row' }))
  }
  expect(
    within(screen.getByRole('row', { name: /unselected-two/ })).getByRole(
      'checkbox',
      { name: 'Select row' }
    )
  ).not.toBeChecked()
  await screen.findByRole('toolbar', {
    name: 'Bulk actions for 2 selected channels',
  })
}

test.each([
  {
    action: 'Enable selected channels',
    url: '/api/channel/status/batch',
    payload: { ids: [101, 303], status: 1 },
  },
  {
    action: 'Disable selected channels',
    url: '/api/channel/status/batch',
    payload: { ids: [101, 303], status: 2 },
  },
  {
    action: 'Set tag for selected channels',
    url: '/api/channel/batch/tag',
    payload: { ids: [101, 303], tag: 'reviewed' },
  },
  {
    action: 'Delete selected channels',
    url: '/api/channel/batch',
    payload: { ids: [101, 303] },
  },
])(
  'tag mode $action submits only checked channel IDs and clears selection after success',
  async ({ action, url, payload }) => {
    const { user, post } = await renderChannelsPage()
    await selectGroupedChildren(user)
    await user.click(screen.getByRole('button', { name: action }))
    if (action === 'Set tag for selected channels') {
      const dialog = await screen.findByRole('dialog', { name: 'Set Tag' })
      await user.type(
        within(dialog).getByRole('textbox', { name: 'Tag' }),
        'reviewed'
      )
      await user.click(within(dialog).getByRole('button', { name: 'Set Tag' }))
    }
    if (action === 'Delete selected channels') {
      const dialog = await screen.findByRole('dialog', {
        name: 'Delete Channels?',
      })
      await user.click(within(dialog).getByRole('button', { name: 'Delete' }))
    }
    await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
    expect(post.mock.calls[0][0]).toBe(url)
    expect(post.mock.calls[0][1]).toEqual(payload)
    await waitFor(() =>
      expect(
        screen.queryByRole('toolbar', { name: /Bulk actions/ })
      ).not.toBeInTheDocument()
    )
  }
)

test('Escape on the toolbar clears grouped checkboxes and hides the toolbar without any mutation', async () => {
  const { user, post } = await renderChannelsPage()
  await selectGroupedChildren(user)
  screen.getByRole('toolbar', { name: /Bulk actions/ }).focus()
  await user.keyboard('{Escape}')
  expect(
    screen.queryByRole('toolbar', { name: /Bulk actions/ })
  ).not.toBeInTheDocument()
  expect(
    screen
      .getAllByRole('checkbox', { name: 'Select row' })
      .every((checkbox) => checkbox.getAttribute('aria-checked') === 'false')
  ).toBe(true)
  expect(post).not.toHaveBeenCalled()
})

test('an administrator without sensitive-write cannot delete selected grouped channels', async () => {
  useAuthStore.getState().auth.setUser({
    id: 2,
    username: 'operator',
    role: 10,
    permissions: {
      admin_permissions: {
        channel: { operate: true, write: true, sensitive_write: false },
      },
    },
  })
  const { user, post } = await renderChannelsPage()
  await selectGroupedChildren(user)
  const button = screen.getByRole('button', {
    name: 'Delete selected channels',
  })
  expect(button).toHaveAttribute('aria-disabled', 'true')
  await user.click(button)
  expect(
    screen.queryByRole('dialog', { name: 'Delete Channels?' })
  ).not.toBeInTheDocument()
  expect(post).not.toHaveBeenCalled()
})

test('Escape dismisses an open toolbar tooltip before a second Escape clears selection', async () => {
  const { user, post } = await renderChannelsPage()
  await selectGroupedChildren(user)
  screen.getByRole('toolbar', { name: /Bulk actions/ }).focus()
  await user.tab()
  expect(screen.getByRole('button', { name: 'Clear selection' })).toHaveFocus()
  await user.hover(screen.getByRole('button', { name: 'Clear selection' }))
  await screen.findByText('Clear selection (Escape)')
  await user.keyboard('{Escape}')
  await waitFor(() =>
    expect(
      screen.queryByText('Clear selection (Escape)')
    ).not.toBeInTheDocument()
  )
  expect(
    screen.getByRole('toolbar', { name: /Bulk actions/ })
  ).toBeInTheDocument()
  await user.keyboard('{Escape}')
  expect(
    screen.queryByRole('toolbar', { name: /Bulk actions/ })
  ).not.toBeInTheDocument()
  expect(post).not.toHaveBeenCalled()
})
