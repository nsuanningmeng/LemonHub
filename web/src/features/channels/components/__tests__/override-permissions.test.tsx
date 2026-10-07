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
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import {
  act,
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
import { useChannelsColumns } from '../channels-columns'
import { ChannelsProvider } from '../channels-provider'
import { ChannelMutateDrawer } from '../drawers/channel-mutate-drawer'

const animationsDescriptor = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'getAnimations'
)
const originalAuth = useAuthStore.getState().auth
let client: QueryClient
let channel: Channel & {
  param_override_configured: boolean
  header_override_configured: boolean
}

function EditingHarness() {
  const [open, setOpen] = useState(true)
  return (
    <QueryClientProvider client={client}>
      <ChannelsProvider>
        <button type='button' onClick={() => setOpen(true)}>
          Reopen editor
        </button>
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
  Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
  localStorage.clear()
  channel = {
    ...channelSchema.parse({
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
      param_override: null,
      header_override: null,
    }),
    param_override_configured: true,
    header_override_configured: true,
  }
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
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
              sensitive_write: true,
              secret_view: false,
            },
          },
        },
      },
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
  if (animationsDescriptor) {
    Object.defineProperty(
      HTMLElement.prototype,
      'getAnimations',
      animationsDescriptor
    )
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, 'getAnimations')
  }
  client.clear()
  localStorage.clear()
  useAuthStore.setState({ auth: originalAuth })
  vi.restoreAllMocks()
})

async function openOverrides() {
  await screen.findByDisplayValue('Existing channel')
  const input = screen.queryByRole('textbox', {
    name: 'Request Header Override',
  })
  if (!input) {
    await userEvent.click(
      screen.getByRole('button', {
        name: /^Advanced Settings Request overrides/,
      })
    )
  }
  return screen.findByRole('textbox', { name: 'Request Header Override' })
}

describe('restricted channel override editing', { timeout: 20000 }, () => {
  test('routing-only save by a sensitive writer preserves both hidden overrides', async () => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    render(<EditingHarness />)
    await screen.findByDisplayValue('Existing channel')
    fireEvent.change(screen.getByRole('textbox', { name: /^Name/ }), {
      target: { value: 'Renamed channel' },
    })
    await userEvent.click(
      screen.getByRole('button', { name: 'Update Channel' })
    )
    await waitFor(() => expect(put).toHaveBeenCalled())
    expect(put.mock.calls[0]?.[1]).toMatchObject({ name: 'Renamed channel' })
    expect(put.mock.calls[0]?.[1]).not.toHaveProperty('param_override')
    expect(put.mock.calls[0]?.[1]).not.toHaveProperty('header_override')
  })

  test('explicit Clear on a hidden empty field submits only that override', async () => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    render(<EditingHarness />)
    const input = await openOverrides()
    expect(input).toHaveValue('')
    const clearButtons = screen.getAllByRole('button', { name: 'Clear' })
    const clear = clearButtons.at(-1)
    expect(clear).toBeDefined()
    if (!clear) {
      throw new Error('Missing header clear action')
    }
    await userEvent.click(clear)
    await userEvent.click(
      screen.getByRole('button', { name: 'Update Channel' })
    )
    await waitFor(() => expect(put).toHaveBeenCalled())
    expect(put.mock.calls[0]?.[1]).toMatchObject({ header_override: '' })
    expect(put.mock.calls[0]?.[1]).not.toHaveProperty('param_override')
  })

  test('header template replaces one hidden configuration and preserves the other', async () => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    render(<EditingHarness />)
    await openOverrides()
    await userEvent.click(
      screen.getByRole('button', { name: 'Passthrough Template' })
    )
    await userEvent.click(
      screen.getByRole('button', { name: 'Update Channel' })
    )
    await waitFor(() => expect(put).toHaveBeenCalled())
    expect(put.mock.calls[0]?.[1]).toMatchObject({
      header_override: JSON.stringify({ '*': true }, null, 2),
    })
    expect(put.mock.calls[0]?.[1]).not.toHaveProperty('param_override')
  })
})

test('typing JSON into a hidden parameter override replaces only that field', async () => {
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true } })
  render(<EditingHarness />)
  await openOverrides()
  const input = screen.getByRole('textbox', { name: 'Parameter Override' })
  fireEvent.input(input, { target: { value: '{"temperature":0}' } })
  await userEvent.click(screen.getByRole('button', { name: 'Update Channel' }))
  await waitFor(() => expect(put).toHaveBeenCalled())
  expect(put.mock.calls[0]?.[1]).toMatchObject({
    param_override: '{"temperature":0}',
  })
  expect(put.mock.calls[0]?.[1]).not.toHaveProperty('header_override')
}, 20000)

test('saving the visual editor marks a hidden parameter override as explicitly edited', async () => {
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true } })
  render(<EditingHarness />)
  await openOverrides()
  await userEvent.click(screen.getByRole('button', { name: 'Visual edit' }))
  await userEvent.click(await screen.findByRole('button', { name: 'Save' }))
  await userEvent.click(screen.getByRole('button', { name: 'Update Channel' }))
  await waitFor(() => expect(put).toHaveBeenCalled())
  expect(put.mock.calls[0]?.[1]).toHaveProperty('param_override')
  expect(put.mock.calls[0]?.[1]).not.toHaveProperty('header_override')
}, 20000)

test('closing and reopening discards explicit edit intent and preserves hidden values', async () => {
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true } })
  render(<EditingHarness />)
  await openOverrides()
  await userEvent.click(
    screen.getByRole('button', { name: 'Passthrough Template' })
  )
  await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  await userEvent.click(
    await screen.findByRole('button', { name: 'Reopen editor' })
  )
  const input = await screen.findByRole('textbox', {
    name: 'Request Header Override',
  })
  expect(input).toHaveValue('')
  await userEvent.click(screen.getByRole('button', { name: 'Update Channel' }))
  await waitFor(() => expect(put).toHaveBeenCalled())
  expect(put.mock.calls[0]?.[1]).not.toHaveProperty('param_override')
  expect(put.mock.calls[0]?.[1]).not.toHaveProperty('header_override')
}, 20000)

test.each([true, false])(
  'secret-view permission %s controls read access without granting sensitive writes',
  async (secretView) => {
    channel.param_override = '{"private":"PARAM_SENTINEL"}'
    channel.header_override = '{"X-Private":"HEADER_SENTINEL"}'
    useAuthStore.setState({
      auth: {
        ...originalAuth,
        user: {
          id: 2,
          username: 'reader',
          role: ROLE.ADMIN,
          permissions: {
            admin_permissions: {
              channel: {
                read: true,
                write: true,
                sensitive_write: false,
                secret_view: secretView,
              },
            },
          },
        },
      },
    })
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    render(<EditingHarness />)
    const input = await openOverrides()
    expect(input).toBeDisabled()
    expect(input).toHaveValue(secretView ? channel.header_override : '')
    expect(
      screen.getByRole('textbox', { name: 'Parameter Override' })
    ).toHaveValue(secretView ? channel.param_override : '')
    if (!secretView) {
      expect(
        screen.getAllByText(
          'Existing configuration is hidden because you do not have permission to view channel secrets.'
        )
      ).toHaveLength(2)
    }
    await userEvent.click(
      screen.getByRole('button', { name: 'Update Channel' })
    )
    await waitFor(() => expect(put).toHaveBeenCalled())
    expect(put.mock.calls[0]?.[1]).not.toHaveProperty('param_override')
    expect(put.mock.calls[0]?.[1]).not.toHaveProperty('header_override')
  },
  20000
)

function NameCell() {
  const table = useReactTable({
    data: [channel],
    columns: useChannelsColumns({ enableSelection: false }),
    getCoreRowModel: getCoreRowModel(),
  })
  const cell = table
    .getRowModel()
    .rows[0]?.getAllCells()
    .find((item) => item.column.id === 'name')
  return cell ? flexRender(cell.column.columnDef.cell, cell.getContext()) : null
}

test('a configured but hidden parameter override keeps the list indicator', async () => {
  render(
    <QueryClientProvider client={client}>
      <ChannelsProvider>
        <NameCell />
      </ChannelsProvider>
    </QueryClientProvider>
  )
  expect(
    screen.getByRole('img', { name: 'Override request parameters' })
  ).toBeInTheDocument()
})

test.each([false, true])(
  'changing authorization scope (secret view %s) removes prior values when the new detail request fails',
  async (secretView) => {
    channel.param_override = '{"private":"PARAM_SENTINEL"}'
    channel.header_override = '{"X-Private":"HEADER_SENTINEL"}'
    useAuthStore.setState({
      auth: {
        ...originalAuth,
        user: { id: 1, username: 'root', role: ROLE.SUPER_ADMIN },
      },
    })
    render(<EditingHarness />)
    const input = await openOverrides()
    expect(input).toHaveValue(channel.header_override)
    vi.mocked(api.get).mockRejectedValueOnce(new Error('Access denied'))
    await act(async () => {
      useAuthStore.setState({
        auth: {
          ...originalAuth,
          user: {
            id: secretView ? 2 : 1,
            username: 'operator',
            role: ROLE.ADMIN,
            permissions: {
              admin_permissions: {
                channel: {
                  read: true,
                  write: true,
                  sensitive_write: true,
                  secret_view: secretView,
                },
              },
            },
          },
        },
      })
    })
    await waitFor(() => {
      expect(
        screen.getByRole('textbox', { name: 'Request Header Override' })
      ).toHaveValue('')
    })
    expect(
      screen.queryByDisplayValue(channel.param_override)
    ).not.toBeInTheDocument()
    expect(
      screen.queryByDisplayValue(channel.header_override)
    ).not.toBeInTheDocument()
  },
  20000
)
