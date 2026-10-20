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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { channelSchema, type Channel } from '../../types'
import { ChannelsProvider } from '../channels-provider'
import { ChannelMutateDrawer } from '../drawers/channel-mutate-drawer'

const originalAuth = useAuthStore.getState().auth
let client: QueryClient
let channel: Channel

beforeEach(() => {
  localStorage.clear()
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
      return { data: { success: true, data: [{ id: 'claude-opus-4-7' }] } }
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
  client.clear()
  localStorage.clear()
  useAuthStore.setState({ auth: originalAuth })
  vi.restoreAllMocks()
})

it.each([true, false])(
  'keeps legacy AWS default and gates interface edits on sensitive_write=%s',
  async (canEditSensitive) => {
    useAuthStore.setState({
      auth: {
        ...originalAuth,
        user: {
          id: 2,
          username: 'writer',
          role: ROLE.ADMIN,
          permissions: {
            admin_permissions: {
              channel: {
                read: true,
                write: true,
                sensitive_write: canEditSensitive,
                secret_view: false,
              },
            },
          },
        },
      },
    })
    channel = channelSchema.parse({
      id: 42,
      key: '',
      type: 33,
      name: 'Existing AWS',
      status: 1,
      created_time: 1,
      test_time: 0,
      response_time: 0,
      balance_updated_time: 0,
      models: 'claude-opus-4-7',
      group: 'default',
      settings: '{}',
    })
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    render(
      <QueryClientProvider client={client}>
        <ChannelsProvider>
          <ChannelMutateDrawer
            open
            onOpenChange={vi.fn()}
            currentRow={channel}
          />
        </ChannelsProvider>
      </QueryClientProvider>
    )
    await screen.findByDisplayValue('Existing AWS')
    const selector = screen.getByRole('combobox', { name: 'AWS API' })
    expect(selector).toHaveTextContent('InvokeModel (legacy)')
    if (!canEditSensitive) {
      expect(selector).toBeDisabled()
      return
    }
    expect(selector).toBeEnabled()
    await userEvent.click(selector)
    await userEvent.click(
      screen.getByRole('option', { name: 'Mantle (Messages API)' })
    )
    await userEvent.click(
      screen.getByRole('button', { name: 'Update Channel' })
    )
    await waitFor(() => expect(put).toHaveBeenCalled())
    const payload = put.mock.calls[0]?.[1] as Pick<Channel, 'settings'>
    expect(JSON.parse(payload.settings)).toMatchObject({
      aws_api_type: 'mantle',
    })
  },
  15000
)
