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
import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import {
  loadConfig,
  loadMessages,
  loadParameterEnabled,
  saveMessages,
} from '../../lib/storage/storage'
import type { Message } from '../../types'
import { usePlaygroundState } from '../use-playground-state'

const accountAMessages: Message[] = [
  {
    key: 'a',
    from: 'user',
    versions: [{ id: 'a-v', content: 'Account A private chat' }],
  },
]
const accountBMessages: Message[] = [
  {
    key: 'b',
    from: 'user',
    versions: [{ id: 'b-v', content: 'Account B private chat' }],
  },
]

beforeEach(() => {
  vi.useFakeTimers()
  localStorage.clear()
  useAuthStore.getState().auth.setUser({ id: 1, username: 'a', role: 1 })
})
afterEach(() => {
  vi.useRealTimers()
  useAuthStore.getState().auth.reset()
  localStorage.clear()
})

test('switching accounts before the debounced save flushes keeps both accounts messages separate', () => {
  const oldAccount = renderHook(() => usePlaygroundState())
  act(() => vi.advanceTimersByTime(0))
  act(() => oldAccount.result.current.updateMessages(accountAMessages))
  act(() =>
    useAuthStore.getState().auth.setUser({ id: 2, username: 'b', role: 1 })
  )
  saveMessages(accountBMessages)
  oldAccount.unmount()
  expect(loadMessages()).toEqual(accountBMessages)
  useAuthStore.getState().auth.setUser({ id: 1, username: 'a', role: 1 })
  expect(loadMessages()).toEqual(accountAMessages)
})

test('late config and parameter callbacks from an old account cannot overwrite the new account', () => {
  const oldAccount = renderHook(() => usePlaygroundState())
  act(() => vi.advanceTimersByTime(0))
  act(() =>
    useAuthStore.getState().auth.setUser({ id: 2, username: 'b', role: 1 })
  )
  act(() => {
    oldAccount.result.current.updateConfig('model', 'account-a-model')
    oldAccount.result.current.updateParameterEnabled('temperature', true)
  })
  expect(loadConfig()).toEqual({})
  expect(loadParameterEnabled()).toEqual({})
  oldAccount.unmount()
})
