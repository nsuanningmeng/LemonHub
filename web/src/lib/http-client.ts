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
import axios, { CanceledError, type AxiosRequestConfig } from 'axios'
import { t } from 'i18next'
import { toast } from 'sonner'

import {
  applyAuthRotation,
  clearAuthentication,
  refreshAuthentication,
} from '@/lib/auth-session'
import { getServerErrorMessageKey } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

declare module 'axios' {
  export interface AxiosRequestConfig {
    skipBusinessError?: boolean
    skipErrorHandler?: boolean
    disableDuplicate?: boolean
    skipAuthRefresh?: boolean
    authRetry?: boolean
    acceptAuthRotation?: boolean
    // Only for public endpoints whose responses do not depend on an account.
    publicRequest?: boolean
    authRequestContext?: {
      identityVersion: number
      callerSignal: AxiosRequestConfig['signal']
    }
    releaseAuthRequest?: () => void
  }
}

export type ApiRequestConfig = AxiosRequestConfig

export const api = axios.create({
  baseURL: '',
  withCredentials: true,
  headers: {
    'Cache-Control': 'no-store',
  },
})

const inFlightGet = new Map<string, Promise<unknown>>()
const originalGet = api.get.bind(api)
let authIdentityVersion = 0
let authRequests = new AbortController()

useAuthStore.subscribe((state, previous) => {
  if (
    state.auth.session?.sid === previous.auth.session?.sid &&
    state.auth.user?.id === previous.auth.user?.id
  ) {
    return
  }

  authIdentityVersion += 1
  const previousRequests = authRequests
  authRequests = new AbortController()
  previousRequests.abort()
})

api.get = ((url: string, config: ApiRequestConfig = {}) => {
  // Callers with their own cancellation lifecycle need independent requests.
  if (config.disableDuplicate || config.signal || config.cancelToken) {
    return originalGet(url, config)
  }

  const params = config.params ? JSON.stringify(config.params) : '{}'
  const scope = config.publicRequest ? 'public' : authIdentityVersion
  const key = `${scope}:${url}?${params}`
  const existingRequest = inFlightGet.get(key)
  if (existingRequest) return existingRequest

  const request = originalGet(url, config).finally(() => {
    inFlightGet.delete(key)
  })
  inFlightGet.set(key, request)
  return request
}) as typeof api.get

function redirectToSignIn(): void {
  if (
    typeof window !== 'undefined' &&
    window.location.pathname !== '/sign-in'
  ) {
    window.location.replace('/sign-in')
  }
}

api.interceptors.response.use(
  (response) => {
    response.config.releaseAuthRequest?.()
    if (
      response.config.authRequestContext &&
      response.config.authRequestContext.identityVersion !== authIdentityVersion
    ) {
      throw new CanceledError('Authentication session changed', response.config)
    }
    if (response.config.acceptAuthRotation && response.data?.success === true) {
      applyAuthRotation(response.data.data)
    }

    if (
      !response.config.skipBusinessError &&
      typeof response.data?.success === 'boolean' &&
      !response.data.success
    ) {
      const messageKey = getServerErrorMessageKey(response.data)
      toast.error(
        messageKey
          ? t(messageKey)
          : response.data.message || t('Request failed')
      )
    }
    return response
  },
  async (error) => {
    const config = error?.config as ApiRequestConfig | undefined
    config?.releaseAuthRequest?.()
    if (axios.isCancel(error)) throw error
    if (
      config?.authRequestContext &&
      config.authRequestContext.identityVersion !== authIdentityVersion
    ) {
      throw new CanceledError('Authentication session changed')
    }
    const skipErrorHandler = config?.skipErrorHandler
    const status = error?.response?.status

    if (status === 401 && !config?.publicRequest) {
      if (config && !config.skipAuthRefresh && !config.authRetry) {
        config.authRetry = true
        const outcome = await refreshAuthentication()
        const currentAuth = useAuthStore.getState().auth
        const sessionExpired =
          (outcome.kind === 'anonymous' || outcome.kind === 'out_of_sync') &&
          !currentAuth.session &&
          !currentAuth.user
        if (
          config.authRequestContext?.identityVersion !== authIdentityVersion &&
          !sessionExpired
        ) {
          throw new CanceledError('Authentication session changed')
        }
        if (outcome.kind === 'authenticated') {
          const token = useAuthStore.getState().auth.accessToken
          if (token) {
            config.headers = {
              ...config.headers,
              Authorization: `Bearer ${token}`,
            }
          }
          return api.request(config)
        }

        if (outcome.kind === 'anonymous' || outcome.kind === 'out_of_sync') {
          if (!skipErrorHandler) toast.error(t('Session expired!'))
          redirectToSignIn()
        }
      } else if (config?.authRetry) {
        clearAuthentication(false)
        if (!skipErrorHandler) toast.error(t('Session expired!'))
        redirectToSignIn()
      } else if (!skipErrorHandler) {
        toast.error(t('Session expired!'))
      }
    } else if (!skipErrorHandler) {
      const messageKey = getServerErrorMessageKey(error)
      const message = messageKey
        ? t(messageKey)
        : error?.response?.data?.message ||
          error?.message ||
          t('Request failed')
      toast.error(message)
    }
    throw error
  }
)

api.interceptors.request.use(
  (config) => {
    if (config.publicRequest) {
      config.headers.delete('Authorization')
      config.skipAuthRefresh = true
      return config
    }

    // Keep the initial identity and caller cancellation signal across retries.
    config.authRequestContext ??= {
      identityVersion: authIdentityVersion,
      callerSignal: config.signal,
    }
    if (config.authRequestContext.identityVersion !== authIdentityVersion) {
      throw new CanceledError('Authentication session changed', config)
    }

    const controller = new AbortController()
    const sessionSignal = authRequests.signal
    const callerSignal = config.authRequestContext.callerSignal
    const abort = () => controller.abort()
    sessionSignal.addEventListener('abort', abort, { once: true })
    callerSignal?.addEventListener?.('abort', abort, { once: true })
    if (callerSignal?.aborted) controller.abort()
    config.signal = controller.signal
    config.releaseAuthRequest = () => {
      sessionSignal.removeEventListener('abort', abort)
      callerSignal?.removeEventListener?.('abort', abort)
    }

    const accessToken = useAuthStore.getState().auth.accessToken
    if (accessToken) {
      config.headers.Authorization = `Bearer ${accessToken}`
    }
    return config
  },
  (error: unknown) => {
    throw error
  },
  { synchronous: true }
)
