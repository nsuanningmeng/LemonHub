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

import { loadMessages, saveMessages } from '../../lib/storage/storage'
import type { Message } from '../../types'
import { usePlaygroundState } from '../use-playground-state'

const messages: Message[] = [
  {
    key: 'a',
    from: 'user',
    versions: [{ id: 'a-v', content: 'Account A unsaved instruction' }],
  },
]

beforeEach(() => {
  vi.useFakeTimers()
  localStorage.clear()
  useAuthStore.getState().auth.setUser({ id: 1, username: 'a', role: 1 })
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.useRealTimers()
  localStorage.clear()
  useAuthStore.getState().auth.reset()
})

test('a quota failure keeps the latest messages in memory and retry clears the failure after saving', () => {
  const view = renderHook(() => usePlaygroundState())
  act(() => vi.advanceTimersByTime(0))
  vi.spyOn(console, 'error').mockImplementation(() => undefined)
  const write = vi
    .spyOn(Storage.prototype, 'setItem')
    .mockImplementation(() => {
      throw new DOMException('Storage is full', 'QuotaExceededError')
    })
  act(() => view.result.current.updateMessages(messages))
  act(() => vi.advanceTimersByTime(500))

  expect(view.result.current.hasMessageSaveError).toBe(true)
  expect(view.result.current.messages).toEqual(messages)
  const updated = [
    ...messages,
    {
      key: 'latest',
      from: 'assistant' as const,
      versions: [{ id: 'latest-v', content: 'Latest response' }],
    },
  ]
  act(() => view.result.current.updateMessages(updated))
  act(() => vi.advanceTimersByTime(500))
  expect(view.result.current.hasMessageSaveError).toBe(true)
  expect(view.result.current.messages).toEqual(updated)

  write.mockRestore()
  act(() => view.result.current.retryMessageSave())

  expect(view.result.current.hasMessageSaveError).toBe(false)
  expect(loadMessages(1)).toEqual(updated)
  view.unmount()
})

test('a new account session clears the old save failure and loads only its own conversation', () => {
  const accountB: Message[] = [
    {
      key: 'b',
      from: 'user',
      versions: [{ id: 'b-v', content: 'Account B message' }],
    },
  ]
  saveMessages(accountB, 2)
  const oldSession = renderHook(() => usePlaygroundState())
  act(() => vi.advanceTimersByTime(0))
  vi.spyOn(console, 'error').mockImplementation(() => undefined)
  const write = vi
    .spyOn(Storage.prototype, 'setItem')
    .mockImplementation(() => {
      throw new DOMException('Storage is full', 'QuotaExceededError')
    })
  act(() => oldSession.result.current.updateMessages(messages))
  act(() => vi.advanceTimersByTime(500))
  expect(oldSession.result.current.hasMessageSaveError).toBe(true)
  const oldRetry = oldSession.result.current.retryMessageSave
  oldSession.unmount()
  write.mockRestore()

  useAuthStore.getState().auth.setUser({ id: 2, username: 'b', role: 1 })
  const currentSession = renderHook(() => usePlaygroundState())
  act(() => vi.advanceTimersByTime(0))
  act(() => oldRetry())

  expect(currentSession.result.current.hasMessageSaveError).toBe(false)
  expect(currentSession.result.current.messages).toEqual(accountB)
  expect(loadMessages(2)).toEqual(accountB)
  currentSession.unmount()
})

test('reloading with an unwritten update flushes it and warns only if persistence fails', () => {
  const view = renderHook(() => usePlaygroundState())
  act(() => vi.advanceTimersByTime(0))
  vi.spyOn(console, 'error').mockImplementation(() => undefined)
  const write = vi
    .spyOn(Storage.prototype, 'setItem')
    .mockImplementation(() => {
      throw new DOMException('Storage is full', 'QuotaExceededError')
    })
  act(() => view.result.current.updateMessages(messages))
  const failedUnload = new Event('beforeunload', { cancelable: true })

  act(() => {
    window.dispatchEvent(failedUnload)
  })

  expect(failedUnload.defaultPrevented).toBe(true)
  expect(view.result.current.hasMessageSaveError).toBe(true)
  expect(view.result.current.messages).toEqual(messages)
  write.mockRestore()
  const savedUnload = new Event('beforeunload', { cancelable: true })

  act(() => {
    window.dispatchEvent(savedUnload)
  })

  expect(savedUnload.defaultPrevented).toBe(false)
  expect(loadMessages(1)).toEqual(messages)
  expect(view.result.current.hasMessageSaveError).toBe(false)
  view.unmount()
})

test('flushing before the initial load completes never overwrites saved messages with an empty array', () => {
  saveMessages(messages, 1)
  const view = renderHook(() => usePlaygroundState())

  act(() => {
    expect(view.result.current.flushMessageSave()).toBe(true)
  })

  expect(loadMessages(1)).toEqual(messages)
  view.unmount()
})

test('consecutive flushes before React renders a failed save both report failure', () => {
  const view = renderHook(() => usePlaygroundState())
  act(() => vi.advanceTimersByTime(0))
  vi.spyOn(console, 'error').mockImplementation(() => undefined)
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
    throw new DOMException('Storage is full', 'QuotaExceededError')
  })
  act(() => view.result.current.updateMessages(messages))

  act(() => {
    expect(view.result.current.flushMessageSave()).toBe(false)
    expect(view.result.current.flushMessageSave()).toBe(false)
  })

  expect(view.result.current.hasMessageSaveError).toBe(true)
  expect(view.result.current.messages).toEqual(messages)
  view.unmount()
})
