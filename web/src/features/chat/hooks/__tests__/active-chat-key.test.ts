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
import { afterEach, expect, it } from 'vitest'

import { api } from '@/lib/api'

import { fetchActiveChatKey } from '../use-active-chat-key'

const originalAdapter = api.defaults.adapter

afterEach(() => {
  api.defaults.adapter = originalAdapter
})

it('requests currently usable keys and skips expired or exhausted stored-enabled rows', async () => {
  const requests: string[] = []
  api.defaults.adapter = async (config) => {
    requests.push(config.url ?? '')
    const data =
      config.method === 'get'
        ? {
            items: [
              { id: 9, status: 1, effective_status: 3 },
              { id: 8, status: 1, effective_status: 4 },
              { id: 7, status: 1, effective_status: 1 },
            ],
            total: 3,
          }
        : { key: `token-${config.url?.split('/')[3]}` }
    return {
      config,
      status: 200,
      statusText: 'OK',
      headers: {},
      data: { success: true, data },
    }
  }
  expect(await fetchActiveChatKey()).toBe('sk-token-7')
  expect(requests).toEqual([
    '/api/token/?p=1&size=50&status=1',
    '/api/token/7/key',
  ])
})

it('reports no usable key without revealing an expired token', async () => {
  const requests: string[] = []
  api.defaults.adapter = async (config) => {
    requests.push(config.url ?? '')
    const data =
      config.method === 'get'
        ? { items: [{ id: 9, status: 1, effective_status: 3 }], total: 1 }
        : { key: 'expired-token' }
    return {
      config,
      status: 200,
      statusText: 'OK',
      headers: {},
      data: { success: true, data },
    }
  }
  await expect(fetchActiveChatKey()).rejects.toThrow(
    'No enabled API keys found'
  )
  expect(requests).toEqual(['/api/token/?p=1&size=50&status=1'])
})
