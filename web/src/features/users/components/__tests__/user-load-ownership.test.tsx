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
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { userSchema, type User } from '../../types'
import { UsersMutateDrawer } from '../users-mutate-drawer'
import { UsersProvider } from '../users-provider'

const originalAdapter = api.defaults.adapter
const originalAnimations = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'getAnimations'
)
const userA = userSchema.parse({
  id: 7,
  username: 'member-a',
  display_name: 'Original user',
  role: 1,
  status: 1,
  quota: 5000000,
  used_quota: 0,
  request_count: 0,
  group: 'default',
})
let client: QueryClient
let reads: Array<{ url: string; respond: (user: User) => void }>
let completeQuota: () => void
let quotaRequested: boolean

function Editor(props: { row?: User }) {
  return (
    <QueryClientProvider client={client}>
      <UsersProvider>
        <UsersMutateDrawer
          open={!!props.row}
          currentRow={props.row}
          onOpenChange={() => undefined}
        />
      </UsersProvider>
    </QueryClientProvider>
  )
}

beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  reads = []
  completeQuota = () => undefined
  quotaRequested = false
  localStorage.clear()
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
  api.defaults.adapter = (config) =>
    new Promise((resolve) => {
      const respond = (data: unknown) =>
        resolve({
          config,
          status: 200,
          statusText: 'OK',
          headers: {},
          data: { success: true, data },
        })
      if (
        config.method === 'get' &&
        /^\/api\/user\/\d+$/.test(config.url ?? '')
      ) {
        reads.push({ url: config.url ?? '', respond })
      } else if (config.url === '/api/user/manage') {
        quotaRequested = true
        completeQuota = () => respond(undefined)
      } else {
        respond(
          config.url === '/api/group/'
            ? ['default']
            : { resources: [], roles: [] }
        )
      }
    })
})

afterEach(async () => {
  cleanup()
  await act(async () => {
    for (const read of reads) read.respond(userA)
    completeQuota()
  })
  client.clear()
  api.defaults.adapter = originalAdapter
  useAuthStore.getState().auth.reset()
  localStorage.clear()
  if (originalAnimations) {
    Object.defineProperty(
      HTMLElement.prototype,
      'getAnimations',
      originalAnimations
    )
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, 'getAnimations')
  }
})

async function completeUserRead(index: number, user: User) {
  await waitFor(() => expect(reads[index]).toBeDefined())
  await act(async () => {
    reads[index]?.respond(user)
  })
}

it.each([7, 8])(
  'reopening user %s ignores the previous editor initial response',
  async (nextId) => {
    const nextUser = {
      ...userA,
      id: nextId,
      username: `member-${nextId}`,
      display_name: 'Current editor',
    }
    const view = render(<Editor row={userA} />)
    await waitFor(() => expect(reads).toHaveLength(1))
    view.rerender(<Editor />)
    view.rerender(<Editor row={nextUser} />)
    await completeUserRead(1, nextUser)
    expect(screen.getByDisplayValue('Current editor')).toBeInTheDocument()
    await completeUserRead(0, userA)
    expect(screen.getByDisplayValue('Current editor')).toBeInTheDocument()
    expect(screen.queryByDisplayValue('Original user')).not.toBeInTheDocument()
  }
)

it.each([7, 8])(
  'a previous quota operation cannot refresh the reopened user %s editor',
  async (nextId) => {
    const user = userEvent.setup()
    const nextUser = {
      ...userA,
      id: nextId,
      username: `member-${nextId}`,
      display_name: 'Current editor',
    }
    const view = render(<Editor row={userA} />)
    await completeUserRead(0, userA)
    await user.click(screen.getByRole('button', { name: 'Adjust Quota' }))
    fireEvent.change(screen.getByRole('spinbutton'), { target: { value: '1' } })
    await user.click(screen.getByRole('button', { name: 'Confirm' }))
    await waitFor(() => expect(quotaRequested).toBe(true))
    view.rerender(<Editor />)
    view.rerender(<Editor row={nextUser} />)
    await completeUserRead(1, nextUser)
    await act(async () => {
      completeQuota()
    })
    // The stale completion must not even request data for the reopened editor.
    expect(reads).toHaveLength(2)
    expect(screen.getByDisplayValue('Current editor')).toBeInTheDocument()
  }
)

it.each([7, 8])(
  'a previous quota refresh response cannot overwrite reopened user %s',
  async (nextId) => {
    const user = userEvent.setup()
    const nextUser = {
      ...userA,
      id: nextId,
      username: `member-${nextId}`,
      display_name: 'Current editor',
    }
    const view = render(<Editor row={userA} />)
    await completeUserRead(0, userA)
    await user.click(screen.getByRole('button', { name: 'Adjust Quota' }))
    fireEvent.change(screen.getByRole('spinbutton'), { target: { value: '1' } })
    await user.click(screen.getByRole('button', { name: 'Confirm' }))
    await waitFor(() => expect(quotaRequested).toBe(true))
    await act(async () => {
      completeQuota()
    })
    await waitFor(() => expect(reads).toHaveLength(2))
    view.rerender(<Editor />)
    view.rerender(<Editor row={nextUser} />)
    await completeUserRead(2, nextUser)
    await completeUserRead(1, { ...userA, display_name: 'Stale quota refresh' })
    expect(screen.getByDisplayValue('Current editor')).toBeInTheDocument()
    expect(
      screen.queryByDisplayValue('Stale quota refresh')
    ).not.toBeInTheDocument()
  }
)

it('applies the quota refresh while the same editor remains open', async () => {
  const user = userEvent.setup()
  render(<Editor row={userA} />)
  await completeUserRead(0, userA)
  await user.click(screen.getByRole('button', { name: 'Adjust Quota' }))
  fireEvent.change(screen.getByRole('spinbutton'), { target: { value: '1' } })
  await user.click(screen.getByRole('button', { name: 'Confirm' }))
  await waitFor(() => expect(quotaRequested).toBe(true))
  await act(async () => {
    completeQuota()
  })
  await completeUserRead(1, {
    ...userA,
    display_name: 'Refreshed user',
    quota: 5500000,
  })
  expect(screen.getByDisplayValue('Refreshed user')).toBeInTheDocument()
})
