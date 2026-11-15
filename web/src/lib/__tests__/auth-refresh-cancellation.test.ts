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
  type AxiosResponse,
  type InternalAxiosRequestConfig,
} from 'axios'
import {
  afterAll,
  afterEach,
  beforeAll,
  beforeEach,
  expect,
  test,
  vi,
} from 'vitest'

import { useAuthStore, type AuthBundle } from '@/stores/auth-store'

const network =
  vi.fn<(config: InternalAxiosRequestConfig) => Promise<AxiosResponse>>()
const originalAdapter = axios.defaults.adapter
const locksDescriptor = Object.getOwnPropertyDescriptor(navigator, 'locks')
let auth: typeof import('../auth-session')
const accountA: AuthBundle = {
  access_token: 'access-a',
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
  access_token: 'access-b',
  user: { id: 2, username: 'account-b', role: 1 },
  session: { ...accountA.session, sid: 'session-b' },
}

function deferred<T>() {
  let resolve: (value: T) => void = () => undefined
  const promise = new Promise<T>((fulfill) => {
    resolve = fulfill
  })
  return { promise, resolve }
}

function response(
  config: InternalAxiosRequestConfig,
  bundle: AuthBundle
): AxiosResponse {
  return {
    config,
    data: { success: true, data: bundle },
    headers: {},
    status: 200,
    statusText: 'OK',
  }
}

beforeAll(async () => {
  axios.defaults.adapter = network
  auth = await import('../auth-session')
})
beforeEach(() => {
  network.mockReset()
  auth.applyAuthBundle(accountA, false)
  Object.defineProperty(navigator, 'locks', {
    configurable: true,
    value: undefined,
  })
})
afterEach(() => {
  auth.clearAuthentication(false)
  if (locksDescriptor) {
    Object.defineProperty(navigator, 'locks', locksDescriptor)
  } else {
    Reflect.deleteProperty(navigator, 'locks')
  }
})
afterAll(() => {
  axios.defaults.adapter = originalAdapter
})

test('signing out aborts the pending refresh transport and leaves authentication cleared', async () => {
  const sent = deferred<InternalAxiosRequestConfig>()
  const release = deferred<void>()
  network.mockImplementation(async (config) => {
    sent.resolve(config)
    await release.promise
    return response(config, accountA)
  })
  const refreshing = auth.refreshAuthentication()
  const request = await sent.promise

  auth.clearAuthentication(false)
  const wasAborted = request.signal?.aborted
  release.resolve()

  expect(await refreshing).toMatchObject({ kind: 'transient_error' })
  expect(wasAborted).toBe(true)
  expect(useAuthStore.getState().auth.session).toBeNull()
})

test('switching accounts cancels the old refresh and keeps new-session refresh callers coalesced when the old task settles', async () => {
  const sentA = deferred<InternalAxiosRequestConfig>()
  const releaseA = deferred<void>()
  const releaseB = deferred<void>()
  network.mockImplementation(async (config) => {
    if (config.headers['X-Auth-Session'] === accountA.session.sid) {
      sentA.resolve(config)
      await releaseA.promise
      return response(config, accountA)
    }
    await releaseB.promise
    return response(config, accountB)
  })
  const oldRefresh = auth.refreshAuthentication()
  const oldRequest = await sentA.promise

  auth.applyAuthBundle(accountB, false)
  const newRefresh = auth.refreshAuthentication()
  const wasAborted = oldRequest.signal?.aborted
  releaseA.resolve()
  await oldRefresh
  const coalescedRefresh = auth.refreshAuthentication()
  releaseB.resolve()
  const results = await Promise.all([newRefresh, coalescedRefresh])

  expect(wasAborted).toBe(true)
  expect(network).toHaveBeenCalledTimes(2)
  expect(results).toEqual([
    { kind: 'authenticated', bundle: accountB },
    { kind: 'authenticated', bundle: accountB },
  ])
  expect(useAuthStore.getState().auth.accessToken).toBe('access-b')
})

test('same-session simultaneous refresh callers share one transport and receive the refreshed token', async () => {
  const sent = deferred<void>()
  const release = deferred<void>()
  const refreshed = { ...accountA, access_token: 'refreshed-a' }
  network.mockImplementation(async (config) => {
    sent.resolve()
    await release.promise
    return response(config, refreshed)
  })
  const first = auth.refreshAuthentication()
  await sent.promise
  const second = auth.refreshAuthentication()
  release.resolve()

  expect(await Promise.all([first, second])).toEqual([
    { kind: 'authenticated', bundle: refreshed },
    { kind: 'authenticated', bundle: refreshed },
  ])
  expect(network).toHaveBeenCalledOnce()
  expect(useAuthStore.getState().auth.accessToken).toBe('refreshed-a')
})

test('cancelling a queued browser refresh lock does not reset the new account bootstrap state', async () => {
  let rejectLock: (reason: unknown) => void = () => undefined
  let lockSignal: AbortSignal | undefined
  const lock = new Promise<never>((_resolve, reject) => {
    rejectLock = reject
  })
  Object.defineProperty(navigator, 'locks', {
    configurable: true,
    value: {
      request: (_name: string, options: LockOptions) => {
        lockSignal = options.signal
        return lock
      },
    },
  })
  const refreshing = auth.refreshAuthentication()
  auth.applyAuthBundle(accountB, false)
  const wasAborted = lockSignal?.aborted
  rejectLock(new DOMException('The lock request was aborted', 'AbortError'))

  expect(await refreshing).toMatchObject({ kind: 'transient_error' })
  expect(wasAborted).toBe(true)
  expect(network).not.toHaveBeenCalled()
  expect(useAuthStore.getState().auth.bootstrapState).toBe('complete')
  expect(useAuthStore.getState().auth.accessToken).toBe('access-b')
})
