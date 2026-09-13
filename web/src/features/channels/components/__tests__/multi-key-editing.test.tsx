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
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { channelSchema, type Channel } from '../../types'
import { ChannelsProvider } from '../channels-provider'
import { ChannelMutateDrawer } from '../drawers/channel-mutate-drawer'

const originalAuth = useAuthStore.getState().auth
let client: QueryClient
let channel: Channel

function EditingHarness() {
  const [open, setOpen] = useState(true)
  return (
    <QueryClientProvider client={client}>
      <ChannelsProvider>
        <ChannelMutateDrawer
          open={open}
          onOpenChange={setOpen}
          currentRow={channel}
        />
      </ChannelsProvider>
    </QueryClientProvider>
  )
}

beforeEach(() => {
  localStorage.clear()
  channel = channelSchema.parse({
    id: 42,
    name: 'Existing channel',
    type: 1,
    key: '',
    status: 1,
    created_time: 1,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
    models: 'custom-model',
    group: 'default',
    base_url: 'https://saved.example',
    channel_info: {
      is_multi_key: true,
      multi_key_size: 2,
      multi_key_mode: 'random',
    },
  })
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  useAuthStore.setState({
    auth: {
      ...originalAuth,
      user: { id: 1, username: 'root', role: ROLE.SUPER_ADMIN },
    },
  })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/channel/42') {
      return { data: { success: true, data: channel } }
    }
    if (url === '/api/channel/models') {
      return { data: { success: true, data: [{ id: 'custom-model' }] } }
    }
    if (url === '/api/group/') {
      return { data: { success: true, data: ['default'] } }
    }
    if (url === '/api/prefill_group') {
      return { data: { success: true, data: [] } }
    }
    if (url === '/api/user/2fa/status' || url === '/api/user/passkey') {
      return { data: { success: true, data: { enabled: false } } }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  localStorage.clear()
  useAuthStore.setState({ auth: originalAuth })
  vi.restoreAllMocks()
})

describe('multi-key channel editing', { timeout: 15000 }, () => {
  test.each([
    ['random', 'Random', 'polling', 'Polling'],
    ['polling', 'Polling', 'random', 'Random'],
  ] as const)(
    'editing %s strategy preserves keys while changing the selection strategy',
    async (initialMode, initialLabel, nextMode, nextLabel) => {
      channel.channel_info.multi_key_mode = initialMode
      const put = vi
        .spyOn(api, 'put')
        .mockResolvedValue({ data: { success: true } })
      const user = userEvent.setup()
      render(<EditingHarness />)
      await screen.findByDisplayValue('Existing channel')
      const strategy = screen.getByRole('combobox', {
        name: 'Multi-Key Strategy',
      })
      expect(strategy).toHaveTextContent(initialLabel)
      await user.click(strategy)
      await user.click(screen.getByRole('option', { name: nextLabel }))
      expect(strategy).toHaveTextContent(nextLabel)
      await user.click(screen.getByRole('button', { name: 'Update Channel' }))
      await waitFor(() => expect(put).toHaveBeenCalled())
      expect(put.mock.calls[0]?.[1]).toMatchObject({
        id: 42,
        multi_key_mode: nextMode,
      })
      expect(put.mock.calls[0]?.[1]).not.toHaveProperty('key')
      expect(put.mock.calls[0]?.[1]).not.toHaveProperty('key_mode')
    }
  )

  test('single-key editing omits the strategy control and update field', async () => {
    channel.channel_info.is_multi_key = false
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    render(<EditingHarness />)
    await screen.findByDisplayValue('Existing channel')
    expect(
      screen.queryByRole('combobox', { name: 'Multi-Key Strategy' })
    ).not.toBeInTheDocument()
    await userEvent.click(
      screen.getByRole('button', { name: 'Update Channel' })
    )
    await waitFor(() => expect(put).toHaveBeenCalled())
    expect(put.mock.calls[0]?.[1]).not.toHaveProperty('multi_key_mode')
  })

  test('an operator without sensitive write permission cannot change strategy and sends no credentials', async () => {
    useAuthStore.setState({
      auth: {
        ...originalAuth,
        user: {
          id: 2,
          username: 'operator',
          role: ROLE.ADMIN,
          permissions: {
            admin_permissions: {
              channel: { read: true, write: true, sensitive_write: false },
            },
          },
        },
      },
    })
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    render(<EditingHarness />)
    await screen.findByDisplayValue('Existing channel')
    const strategy = screen.getByRole('combobox', {
      name: 'Multi-Key Strategy',
    })
    expect(strategy).toBeDisabled()
    fireEvent.change(screen.getByRole('textbox', { name: /^Name/ }), {
      target: { value: 'Renamed channel' },
    })
    await userEvent.click(
      screen.getByRole('button', { name: 'Update Channel' })
    )
    await waitFor(() => expect(put).toHaveBeenCalled())
    expect(put.mock.calls[0]?.[1]).toMatchObject({ name: 'Renamed channel' })
    for (const field of [
      'multi_key_mode',
      'key_mode',
      'key',
      'base_url',
      'setting',
      'settings',
    ]) {
      expect(put.mock.calls[0]?.[1]).not.toHaveProperty(field)
    }
  })

  test.each([
    ['append', 'Append to existing keys'],
    ['replace', 'Replace all existing keys'],
  ] as const)(
    'editing with a new key preserves the selected %s mode',
    async (mode, label) => {
      const put = vi
        .spyOn(api, 'put')
        .mockResolvedValue({ data: { success: true } })
      const user = userEvent.setup()
      render(<EditingHarness />)
      await screen.findByDisplayValue('Existing channel')
      fireEvent.change(screen.getByLabelText('API Key *'), {
        target: { value: 'new-key' },
      })
      await user.click(
        screen.getByRole('combobox', { name: 'Key Update Mode' })
      )
      await user.click(screen.getByRole('option', { name: label }))
      await user.click(screen.getByRole('button', { name: 'Update Channel' }))
      await waitFor(() => expect(put).toHaveBeenCalled())
      expect(put.mock.calls[0]?.[1]).toMatchObject({
        id: 42,
        key: 'new-key',
        key_mode: mode,
        multi_key_mode: 'random',
      })
    }
  )
})
