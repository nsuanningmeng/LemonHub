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
import { AxiosError, CanceledError } from 'axios'
import { toast } from 'sonner'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/http-client'
import { useAuthStore } from '@/stores/auth-store'

afterEach(() => {
  useAuthStore.getState().auth.reset('idle')
})

describe('HTTP error notifications', () => {
  test('aborting an in-flight query rejects without showing an error notification', async () => {
    const notify = vi.spyOn(toast, 'error')
    const controller = new AbortController()
    let requestStarted = () => {}
    const started = new Promise<void>((resolve) => {
      requestStarted = resolve
    })
    const request = api.get('/api/rewards/summary', {
      signal: controller.signal,
      adapter: (config) =>
        new Promise((_, reject) => {
          controller.signal.addEventListener('abort', () => {
            reject(new CanceledError('canceled', config))
          })
          requestStarted()
        }),
    })
    const rejected = expect(request).rejects.toBeInstanceOf(CanceledError)

    await started
    controller.abort()

    await rejected
    expect(notify).not.toHaveBeenCalled()
  })

  test('a transport failure still rejects and shows the error notification', async () => {
    const notify = vi.spyOn(toast, 'error')
    const failure = new AxiosError('Network Error', 'ERR_NETWORK')

    await expect(
      api.get('/api/rewards/summary', {
        adapter: () => Promise.reject(failure),
      })
    ).rejects.toBe(failure)

    expect(notify).toHaveBeenCalledWith('Network Error')
  })
})
