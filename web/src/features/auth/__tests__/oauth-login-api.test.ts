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
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { telegramLogin, wechatLoginByCode } from '../api'

afterEach(() => vi.restoreAllMocks())

describe('browser-bound provider login requests', () => {
  test('WeChat uses the supplied flow without creating one from a copied code', async () => {
    const get = vi
      .spyOn(api, 'get')
      .mockResolvedValue({ data: { success: true } })
    const post = vi.spyOn(api, 'post')

    await wechatLoginByCode('user-entered-code', 'existing-browser-flow')

    expect(get).toHaveBeenCalledWith('/api/oauth/wechat', {
      params: { code: 'user-entered-code' },
      headers: { 'X-OAuth-State': 'existing-browser-flow' },
    })
    expect(post).not.toHaveBeenCalled()
  })

  test('Telegram keeps browser state outside the provider-signed parameters', async () => {
    const get = vi
      .spyOn(api, 'get')
      .mockResolvedValue({ data: { success: true } })
    const authorization = Object.freeze({
      id: '123456',
      auth_date: '1900000000',
      hash: 'provider-signature',
    })

    await telegramLogin(authorization, 'telegram-browser-flow')

    expect(get).toHaveBeenCalledWith(
      '/api/oauth/telegram/login',
      expect.objectContaining({
        params: {
          id: '123456',
          auth_date: '1900000000',
          hash: 'provider-signature',
        },
        headers: { 'X-OAuth-State': 'telegram-browser-flow' },
      })
    )
  })
})
