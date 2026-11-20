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
import type { InternalAxiosRequestConfig } from 'axios'
import { toast } from 'sonner'
import { expect, test, vi } from 'vitest'

import { api } from '../http-client'

vi.mock('sonner', () => ({ toast: { error: vi.fn() } }))

function successfulResponse(config: InternalAxiosRequestConfig) {
  return {
    config,
    data: { success: true },
    status: 200,
    statusText: 'OK',
    headers: {},
  }
}

test('canceling one subscriber keeps a replacement GET alive and silent', async () => {
  const first = new AbortController()
  const adapter = vi.fn(async (config: InternalAxiosRequestConfig) =>
    successfulResponse(config)
  )
  const original = api.get('/test/cancel', { signal: first.signal, adapter })
  first.abort()
  const replacement = api.get('/test/cancel', {
    signal: new AbortController().signal,
    adapter,
  })

  await expect(original).rejects.toMatchObject({ code: 'ERR_CANCELED' })
  await expect(replacement).resolves.toMatchObject({ status: 200 })
  expect(toast.error).not.toHaveBeenCalled()
})

test('unscoped duplicate GETs still share the response and real errors are reported', async () => {
  const adapter = vi.fn(async (config: InternalAxiosRequestConfig) =>
    successfulResponse(config)
  )
  const a = api.get('/test/deduplicate', { adapter })
  const b = api.get('/test/deduplicate', { adapter })
  await Promise.all([a, b])
  expect(adapter).toHaveBeenCalledTimes(1)

  await expect(
    api.get('/test/network-failure', {
      adapter: async () => {
        throw new Error('Network Error')
      },
    })
  ).rejects.toThrow('Network Error')
  expect(toast.error).toHaveBeenCalledWith('Network Error')
})
