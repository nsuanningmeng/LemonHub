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
import { I18nextProvider } from 'react-i18next'
import {
  afterAll,
  afterEach,
  beforeAll,
  beforeEach,
  expect,
  test,
  vi,
} from 'vitest'

import { createLocaleI18n } from '@/i18n/__tests__/helpers'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { userSchema } from '../../types'
import { UsersMutateDrawer } from '../users-mutate-drawer'
import { UsersProvider } from '../users-provider'

const clients: QueryClient[] = []
const originalGetAnimations = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'getAnimations'
)

beforeAll(() => {
  Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
})

afterAll(() => {
  if (originalGetAnimations) {
    Object.defineProperty(
      HTMLElement.prototype,
      'getAnimations',
      originalGetAnimations
    )
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, 'getAnimations')
  }
})

beforeEach(() => {
  localStorage.clear()
  useAuthStore.getState().auth.setUser({ id: 1, username: 'admin', role: 100 })
})

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  useAuthStore.getState().auth.reset()
  localStorage.clear()
})

async function renderUserForm(isUpdate = true, language: 'en' | 'zhCN' = 'en') {
  const existingUser = userSchema.parse({
    id: 7,
    username: 'member',
    display_name: 'Member',
    role: 1,
    status: 1,
    quota: 0,
    used_quota: 0,
    request_count: 0,
    group: 'default',
  })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    switch (url) {
      case '/api/group/':
        return { data: { success: true, data: ['default'] } }
      case '/api/authz/catalog':
        return { data: { success: true, data: { resources: [], roles: [] } } }
      case '/api/user/7':
        return { data: { success: true, data: existingUser } }
      default:
        throw new Error(`Unexpected GET ${url}`)
    }
  })
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true } })
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true } })
  const close = vi.fn()
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  const i18n = await createLocaleI18n(language)
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <UsersProvider>
          <UsersMutateDrawer
            open
            currentRow={isUpdate ? existingUser : undefined}
            onOpenChange={close}
          />
        </UsersProvider>
      </QueryClientProvider>
    </I18nextProvider>
  )
  if (isUpdate) {
    await screen.findByDisplayValue('member')
  }
  return { put, post, close, i18n }
}

test.each([
  ['7 characters', 'short12'],
  ['21 characters', '123456789012345678901'],
  ['7 Unicode characters', '🔐🔐🔐🔐🔐🔐🔐'],
])(
  'editing a password with %s shows a field error without saving',
  async (_name, password) => {
    const user = userEvent.setup()
    const { put, close, i18n } = await renderUserForm()
    const input = screen.getByLabelText(i18n.t('Password'))
    await user.type(input, password)
    await user.click(
      screen.getByRole('button', { name: i18n.t('Save changes') })
    )

    await waitFor(() => expect(input).toHaveAttribute('aria-invalid', 'true'))
    expect(input).toHaveAccessibleDescription(
      expect.stringContaining(
        i18n.t('Password must be between 8 and 20 characters')
      )
    )
    expect(put).not.toHaveBeenCalled()
    expect(close).not.toHaveBeenCalled()
  }
)

test.each(['12345678', '12345678901234567890', '🔐🔐🔐🔐🔐🔐🔐🔐'])(
  'editing a valid password submits its exact value: %s',
  async (password) => {
    const user = userEvent.setup()
    const { put, close, i18n } = await renderUserForm()
    await user.type(screen.getByLabelText(i18n.t('Password')), password)
    await user.click(
      screen.getByRole('button', { name: i18n.t('Save changes') })
    )

    await waitFor(() => expect(close).toHaveBeenCalledWith(false))
    expect(put).toHaveBeenCalledWith(
      '/api/user/',
      expect.objectContaining({ id: 7, password })
    )
  }
)

test('editing with an empty password saves without replacing the password', async () => {
  const user = userEvent.setup()
  const { put, close, i18n } = await renderUserForm()
  await user.click(screen.getByRole('button', { name: i18n.t('Save changes') }))

  await waitFor(() => expect(close).toHaveBeenCalledWith(false))
  expect(put).toHaveBeenCalledWith(
    '/api/user/',
    expect.objectContaining({ id: 7, password: undefined })
  )
})

test('creating a user with an empty password shows a field error without saving', async () => {
  const user = userEvent.setup()
  const { post, i18n } = await renderUserForm(false)
  await user.type(screen.getByLabelText(i18n.t('Username')), 'new-member')
  await user.click(screen.getByRole('button', { name: i18n.t('Save changes') }))

  const input = screen.getByLabelText(i18n.t('Password'))
  await waitFor(() => expect(input).toHaveAttribute('aria-invalid', 'true'))
  expect(input).toHaveAccessibleDescription(
    expect.stringContaining(
      i18n.t('Password must be between 8 and 20 characters')
    )
  )
  expect(post).not.toHaveBeenCalled()
})

test('editing an invalid password shows the existing Chinese length message', async () => {
  const user = userEvent.setup()
  const { put, i18n } = await renderUserForm(true, 'zhCN')
  const input = screen.getByLabelText(i18n.t('Password'))
  await user.type(input, 'short12')
  await user.click(screen.getByRole('button', { name: i18n.t('Save changes') }))

  await waitFor(() => expect(input).toHaveAttribute('aria-invalid', 'true'))
  expect(input).toHaveAccessibleDescription(
    expect.stringContaining('密码长度必须在 8 到 20 个字符之间')
  )
  expect(put).not.toHaveBeenCalled()
})
