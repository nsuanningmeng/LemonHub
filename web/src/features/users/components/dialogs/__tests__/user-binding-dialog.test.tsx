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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterAll, beforeAll, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { UserBindingDialog } from '../user-binding-dialog'

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

function mockBindingRequests(field: string, custom = false) {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    switch (url) {
      case '/api/user/7':
        return {
          data: {
            success: true,
            data: { id: 7, username: 'bound-user', [field]: 'bound-identity' },
          },
        }
      case '/api/user/7/oauth/bindings':
        return {
          data: {
            success: true,
            data: custom
              ? [{ provider_id: 42, provider_user_id: 'custom-identity' }]
              : [],
          },
        }
      case '/api/status':
        return {
          data: {
            success: true,
            data: {
              github_oauth: true,
              discord_oauth: true,
              wechat_login: true,
              oidc_enabled: true,
              telegram_oauth: true,
              linuxdo_oauth: true,
              custom_oauth_providers: custom
                ? [{ id: 42, name: 'Company Login' }]
                : [],
            },
          },
        }
      default:
        throw new Error(`Unexpected GET ${url}`)
    }
  })
  return vi.spyOn(api, 'delete').mockResolvedValue({ data: { success: true } })
}

test.each([
  ['Email', 'email', 'email'],
  ['GitHub', 'github_id', 'github'],
  ['Discord', 'discord_id', 'discord'],
  ['WeChat', 'wechat_id', 'wechat'],
  ['OIDC', 'oidc_id', 'oidc'],
  ['Telegram', 'telegram_id', 'telegram'],
  ['LinuxDO', 'linux_do_id', 'linuxdo'],
])(
  'confirmed %s unbinding sends the backend provider type',
  async (provider, field, type) => {
    const remove = mockBindingRequests(field)
    render(<UserBindingDialog open userId={7} onOpenChange={() => undefined} />)
    await screen.findByText(provider)
    fireEvent.click(screen.getByRole('button', { name: '' }))
    expect(remove).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Confirm Unbind' }))
    await waitFor(() =>
      expect(remove).toHaveBeenCalledWith(`/api/user/7/bindings/${type}`)
    )
  }
)

test('custom OAuth unbinding keeps the provider-specific endpoint', async () => {
  const remove = mockBindingRequests('unused', true)
  render(<UserBindingDialog open userId={7} onOpenChange={() => undefined} />)
  await screen.findByText('Company Login')
  fireEvent.click(screen.getByRole('button', { name: '' }))
  fireEvent.click(screen.getByRole('button', { name: 'Confirm Unbind' }))
  await waitFor(() =>
    expect(remove).toHaveBeenCalledWith('/api/user/7/oauth/bindings/42')
  )
})

test('cancelling built-in unbinding does not send a deletion request', async () => {
  const remove = mockBindingRequests('github_id')
  render(<UserBindingDialog open userId={7} onOpenChange={() => undefined} />)
  await screen.findByText('GitHub')
  fireEvent.click(screen.getByRole('button', { name: '' }))
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  await waitFor(() =>
    expect(
      screen.queryByRole('button', { name: 'Confirm Unbind' })
    ).not.toBeInTheDocument()
  )
  expect(remove).not.toHaveBeenCalled()
  expect(screen.getByText('bound-identity')).toBeInTheDocument()
})
