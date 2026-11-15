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
import axios, {
  AxiosError,
  type AxiosResponse,
  type InternalAxiosRequestConfig,
} from 'axios'
import {
  afterAll,
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  test,
  vi,
} from 'vitest'

import { useAuthStore, type AuthBundle } from '@/stores/auth-store'

const network =
  vi.fn<(config: InternalAxiosRequestConfig) => Promise<AxiosResponse>>()
const originalAdapter = axios.defaults.adapter
let api: typeof import('../http-client').api
let applyAuthBundle: typeof import('../auth-session').applyAuthBundle
let clearAuthentication: typeof import('../auth-session').clearAuthentication
let getStatus: typeof import('../api').getStatus
let getSetupStatus: typeof import('@/features/setup/api').getSetupStatus

const accountA: AuthBundle = {
  access_token: 'token-a',
  token_type: 'Bearer',
  access_expires_at: 2_000_000_000,
  user: { id: 1, username: 'account-a', role: 1 },
  session: {
    sid: 'session-a',
    current: true,
    login_method: 'password',
    ip: '127.0.0.1',
    user_agent: 'test',
    created_at: 1,
    last_active_at: 1,
    expires_at: 2_000_000_000,
  },
}
const accountB: AuthBundle = {
  ...accountA,
  access_token: 'token-b',
  user: { id: 2, username: 'account-b', role: 1 },
  session: { ...accountA.session, sid: 'session-b' },
}

function deferred<T>(): {
  promise: Promise<T>
  resolve: (value: T) => void
} {
  let resolve: (value: T) => void = () => undefined
  const promise = new Promise<T>((fulfill) => {
    resolve = fulfill
  })
  return { promise, resolve }
}

function response(
  config: InternalAxiosRequestConfig,
  data: unknown
): AxiosResponse {
  return { config, data, headers: {}, status: 200, statusText: 'OK' }
}

function unauthorized(config: InternalAxiosRequestConfig): AxiosError {
  return new AxiosError('Unauthorized', 'ERR_BAD_REQUEST', config, undefined, {
    ...response(config, {}),
    status: 401,
    statusText: 'Unauthorized',
  })
}

beforeAll(async () => {
  axios.defaults.adapter = network
  ;({ api } = await import('../http-client'))
  ;({ applyAuthBundle, clearAuthentication } = await import('../auth-session'))
  ;({ getStatus } = await import('../api'))
  ;({ getSetupStatus } = await import('@/features/setup/api'))
})

beforeEach(() => {
  network.mockReset()
  applyAuthBundle(accountA, false)
})

afterEach(() => {
  clearAuthentication(false)
})

afterAll(() => {
  axios.defaults.adapter = originalAdapter
})

describe('HTTP requests stay within their original authentication session', () => {
  test.each(['/api/status', '/api/setup'])(
    'public bootstrap data at %s survives authentication completing',
    async (url) => {
      clearAuthentication(false)
      const sent = deferred<InternalAxiosRequestConfig>()
      const release = deferred<void>()
      network.mockImplementation(async (config) => {
        sent.resolve(config)
        await release.promise
        return response(config, { success: true, data: { status: true } })
      })

      const request = url === '/api/status' ? getStatus() : getSetupStatus()
      const config = await sent.promise
      applyAuthBundle(accountB, false)
      release.resolve()

      await expect(request).resolves.toEqual(
        url === '/api/status'
          ? { status: true }
          : { success: true, data: { status: true } }
      )
      expect(config.signal?.aborted).not.toBe(true)
      expect(config.headers.Authorization).toBeUndefined()
      expect(network).toHaveBeenCalledTimes(1)
    }
  )

  test('an authenticated public-status request sends no token and cannot refresh or clear the session on 401', async () => {
    network.mockImplementation(async (config) => {
      throw unauthorized(config)
    })

    await expect(getStatus()).rejects.toMatchObject({
      response: { status: 401 },
    })

    expect(network).toHaveBeenCalledTimes(1)
    expect(network.mock.calls[0][0].headers.Authorization).toBeUndefined()
    expect(useAuthStore.getState().auth.accessToken).toBe('token-a')
  })

  test('a late 401 after switching accounts neither refreshes nor replays the old write', async () => {
    const sent = deferred<InternalAxiosRequestConfig>()
    const release = deferred<void>()
    network.mockImplementation(async (config) => {
      if (config.url === '/api/user/auth/refresh') {
        return response(config, { success: true, data: accountB })
      }
      if (!config.authRetry) {
        sent.resolve(config)
        await release.promise
        throw unauthorized(config)
      }
      return response(config, { success: true })
    })

    const request = api.put('/api/user/setting', { setting: 'belongs-to-a' })
    const originalRequest = await sent.promise
    clearAuthentication(false)
    applyAuthBundle(accountB, false)
    release.resolve()

    await expect(request).rejects.toMatchObject({ code: 'ERR_CANCELED' })
    expect(originalRequest.headers.Authorization).toBe('Bearer token-a')
    expect(network).toHaveBeenCalledTimes(1)
    expect(useAuthStore.getState().auth.session?.sid).toBe('session-b')
  })

  test('switching accounts while a refresh is pending does not replay the old write', async () => {
    const refreshing = deferred<void>()
    const release = deferred<void>()
    network.mockImplementation(async (config) => {
      if (config.url === '/api/user/auth/refresh') {
        refreshing.resolve()
        await release.promise
        return response(config, { success: true, data: accountA })
      }
      throw unauthorized(config)
    })

    const request = api.put('/api/user/setting', { setting: 'belongs-to-a' })
    await refreshing.promise
    applyAuthBundle(accountB, false)
    release.resolve()

    await expect(request).rejects.toMatchObject({ code: 'ERR_CANCELED' })
    expect(network).toHaveBeenCalledTimes(2)
    expect(useAuthStore.getState().auth.accessToken).toBe('token-b')
  })

  test('a retried request returning 401 after an account switch cannot clear the new session', async () => {
    const retried = deferred<InternalAxiosRequestConfig>()
    const release = deferred<void>()
    network.mockImplementation(async (config) => {
      if (config.url === '/api/user/auth/refresh') {
        return response(config, {
          success: true,
          data: { ...accountA, access_token: 'refreshed-a' },
        })
      }
      if (config.authRetry) {
        retried.resolve(config)
        await release.promise
      }
      throw unauthorized(config)
    })

    const request = api.put('/api/user/setting', { setting: 'belongs-to-a' })
    const retry = await retried.promise
    expect(retry.headers.Authorization).toBe('Bearer refreshed-a')
    applyAuthBundle(accountB, false)
    release.resolve()

    await expect(request).rejects.toMatchObject({ code: 'ERR_CANCELED' })
    expect(useAuthStore.getState().auth.accessToken).toBe('token-b')
    expect(network).toHaveBeenCalledTimes(3)
  })

  test('signing out aborts requests even when the caller supplies its own cancellation signal', async () => {
    const sent = deferred<InternalAxiosRequestConfig>()
    const release = deferred<void>()
    const controller = new AbortController()
    network.mockImplementation(async (config) => {
      sent.resolve(config)
      await release.promise
      return response(config, { success: true })
    })

    const request = api.put(
      '/api/user/setting',
      {},
      { signal: controller.signal }
    )
    const originalRequest = await sent.promise
    clearAuthentication(false)
    const abortedOnSignOut = originalRequest.signal?.aborted
    release.resolve()

    await expect(request).rejects.toMatchObject({ code: 'ERR_CANCELED' })
    expect(abortedOnSignOut).toBe(true)
    expect(controller.signal.aborted).toBe(false)
  })

  test('switching accounts immediately after starting a write cannot send it with the new credentials', async () => {
    network.mockImplementation(async (config) =>
      response(config, { success: true })
    )

    const request = api.put('/api/user/setting', { setting: 'belongs-to-a' })
    applyAuthBundle(accountB, false)

    await expect(request).rejects.toMatchObject({ code: 'ERR_CANCELED' })
    expect(network).toHaveBeenCalledTimes(1)
    expect(network.mock.calls[0][0].headers.Authorization).toBe(
      'Bearer token-a'
    )
  })

  test('a caller can still cancel its retried request after authentication refresh', async () => {
    const retried = deferred<InternalAxiosRequestConfig>()
    const release = deferred<void>()
    const controller = new AbortController()
    network.mockImplementation(async (config) => {
      if (config.url === '/api/user/auth/refresh') {
        return response(config, { success: true, data: accountA })
      }
      if (!config.authRetry) throw unauthorized(config)
      retried.resolve(config)
      await release.promise
      return response(config, { success: true })
    })

    const request = api.put(
      '/api/user/setting',
      {},
      { signal: controller.signal }
    )
    const retry = await retried.promise
    controller.abort()
    const retryAborted = retry.signal?.aborted
    release.resolve()

    await expect(request).rejects.toMatchObject({ code: 'ERR_CANCELED' })
    expect(retryAborted).toBe(true)
    expect(useAuthStore.getState().auth.accessToken).toBe('token-a')
  })

  test('signing out and restoring the same SID does not reuse an earlier pending GET', async () => {
    const sent = deferred<void>()
    const release = deferred<void>()
    network.mockImplementationOnce(async (config) => {
      sent.resolve()
      await release.promise
      return response(config, { value: 'old' })
    })
    network.mockImplementation(async (config) =>
      response(config, { value: 'new' })
    )

    const previousRequest = api.get('/api/user/self')
    await sent.promise
    clearAuthentication(false)
    applyAuthBundle(accountA, false)
    const currentRequest = api.get('/api/user/self')
    release.resolve()

    await expect(previousRequest).rejects.toMatchObject({
      code: 'ERR_CANCELED',
    })
    await expect(currentRequest).resolves.toMatchObject({
      data: { value: 'new' },
    })
    expect(network).toHaveBeenCalledTimes(2)
  })

  test('a same-session token refresh retries the original body once with the new token', async () => {
    network.mockImplementation(async (config) => {
      if (config.url === '/api/user/auth/refresh') {
        return response(config, {
          success: true,
          data: { ...accountA, access_token: 'refreshed-a' },
        })
      }
      if (!config.authRetry) throw unauthorized(config)
      return response(config, { success: true })
    })

    await expect(
      api.put('/api/user/setting', { setting: 'belongs-to-a' })
    ).resolves.toMatchObject({
      data: { success: true },
    })

    const writes = network.mock.calls
      .map(([config]) => config)
      .filter((config) => config.method === 'put')
    expect(writes).toHaveLength(2)
    expect(writes[1].headers.Authorization).toBe('Bearer refreshed-a')
    expect(writes[1].data).toBe(JSON.stringify({ setting: 'belongs-to-a' }))
  })

  test('a refresh that discovers a different browser session cannot replay the old write', async () => {
    network.mockImplementation(async (config) => {
      if (config.url === '/api/user/auth/refresh') {
        if (config.headers['X-Auth-Session']) {
          throw new AxiosError(
            'Session mismatch',
            'ERR_BAD_REQUEST',
            config,
            undefined,
            {
              ...response(config, { code: 'AUTH_SESSION_MISMATCH' }),
              status: 409,
            }
          )
        }
        return response(config, { success: true, data: accountB })
      }
      if (!config.authRetry) throw unauthorized(config)
      return response(config, { success: true })
    })

    await expect(
      api.put('/api/user/setting', { setting: 'belongs-to-a' })
    ).rejects.toMatchObject({ code: 'ERR_CANCELED' })

    expect(network).toHaveBeenCalledTimes(3)
    expect(useAuthStore.getState().auth.accessToken).toBe('token-b')
  })

  test('an expired refresh still clears the original session and rejects the write', async () => {
    const previousURL = window.location.href
    window.history.replaceState(null, '', '/sign-in')
    network.mockImplementation(async (config) => {
      throw unauthorized(config)
    })

    try {
      await expect(
        api.put('/api/user/setting', {}, { skipErrorHandler: true })
      ).rejects.toMatchObject({ response: { status: 401 } })
      expect(useAuthStore.getState().auth.session).toBeNull()
      expect(network).toHaveBeenCalledTimes(2)
    } finally {
      window.history.replaceState(null, '', previousURL)
    }
  })
})
