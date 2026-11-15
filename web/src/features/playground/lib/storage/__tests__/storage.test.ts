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
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import { STORAGE_KEYS } from '../../../constants'
import type { Message } from '../../../types'
import {
  clearPlaygroundData,
  loadConfig,
  loadMessages,
  loadParameterEnabled,
  saveConfig,
  saveMessages,
  saveParameterEnabled,
} from '../storage'

const messages: Message[] = [
  {
    key: 'a-chat',
    from: 'user',
    versions: [{ id: 'a-version', content: 'Account A private chat' }],
  },
]

beforeEach(() => {
  localStorage.clear()
  useAuthStore.getState().auth.reset()
})

afterEach(() => {
  localStorage.clear()
  useAuthStore.getState().auth.reset()
})

test('switching accounts isolates config, messages and parameter toggles and preserves returning account data', () => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'A', role: 1 })
  saveConfig({ model: 'account-a-model' })
  saveMessages(messages)
  saveParameterEnabled({ temperature: true })

  useAuthStore.getState().auth.reset()
  useAuthStore.getState().auth.setUser({ id: 2, username: 'B', role: 1 })
  expect(loadConfig()).toEqual({})
  expect(loadMessages()).toBeNull()
  expect(loadParameterEnabled()).toEqual({})
  saveConfig({ model: 'account-b-model' })
  saveMessages([])
  saveParameterEnabled({ temperature: false })

  useAuthStore.getState().auth.setUser({ id: 1, username: 'A', role: 1 })
  expect(loadConfig()).toEqual({ model: 'account-a-model' })
  expect(loadMessages()).toEqual(messages)
  expect(loadParameterEnabled()).toEqual({ temperature: true })

  clearPlaygroundData()
  expect(loadMessages()).toBeNull()
  useAuthStore.getState().auth.setUser({ id: 2, username: 'B', role: 1 })
  expect(loadConfig()).toEqual({ model: 'account-b-model' })
  expect(loadMessages()).toEqual([])
  expect(loadParameterEnabled()).toEqual({ temperature: false })
})

test('legacy global data stays unassigned and preserved instead of being shown to the next account', () => {
  localStorage.setItem(
    STORAGE_KEYS.CONFIG,
    JSON.stringify({ model: 'legacy-model' })
  )
  localStorage.setItem(STORAGE_KEYS.MESSAGES, JSON.stringify(messages))
  localStorage.setItem(
    STORAGE_KEYS.PARAMETER_ENABLED,
    JSON.stringify({ temperature: true })
  )
  useAuthStore.getState().auth.setUser({ id: 2, username: 'B', role: 1 })

  expect(loadConfig()).toEqual({})
  expect(loadMessages()).toBeNull()
  expect(loadParameterEnabled()).toEqual({})
  expect(
    JSON.parse(localStorage.getItem(STORAGE_KEYS.CONFIG) ?? 'null')
  ).toEqual({
    model: 'legacy-model',
  })
  expect(
    JSON.parse(localStorage.getItem(STORAGE_KEYS.MESSAGES) ?? 'null')
  ).toEqual(messages)
  expect(
    JSON.parse(localStorage.getItem(STORAGE_KEYS.PARAMETER_ENABLED) ?? 'null')
  ).toEqual({ temperature: true })
})

test('storage calls without a signed-in user do not persist shared data', () => {
  saveConfig({ model: 'signed-out-model' })
  saveMessages(messages)
  saveParameterEnabled({ temperature: true })
  expect(loadConfig()).toEqual({})
  expect(loadMessages()).toBeNull()
  expect(loadParameterEnabled()).toEqual({})
  expect(localStorage.length).toBe(0)
})

test('reloading long messages preserves every version and reasoning without rewriting storage', () => {
  const content = `${'long prompt '.repeat(4100)}end of prompt`
  const longMessages: Message[] = [
    {
      key: 'long-message',
      from: 'assistant',
      status: 'complete',
      versions: [
        { id: 'original', content },
        { id: 'revised', content: `${content} revised ending` },
      ],
      reasoning: { content: `${content} reasoning ending`, duration: 2 },
    },
    ...messages,
  ]
  saveMessages(longMessages, 1)
  const saved = localStorage.getItem(`${STORAGE_KEYS.MESSAGES}:1`)

  expect(loadMessages(1)).toEqual(longMessages)
  expect(loadMessages(1)).toEqual(longMessages)
  expect(localStorage.getItem(`${STORAGE_KEYS.MESSAGES}:1`)).toBe(saved)
})

test('reloading a stored conversation larger than one million characters preserves the complete record', () => {
  const largeMessages: Message[] = [
    {
      key: 'large-message',
      from: 'user',
      versions: [
        {
          id: 'large-version',
          content: `${'x'.repeat(1_048_576)}complete ending`,
        },
      ],
    },
  ]
  saveMessages(largeMessages, 1)
  const saved = localStorage.getItem(`${STORAGE_KEYS.MESSAGES}:1`)

  expect(loadMessages(1)).toEqual(largeMessages)
  expect(localStorage.getItem(`${STORAGE_KEYS.MESSAGES}:1`)).toBe(saved)
})

test('saving more than one hundred messages keeps the earliest message after reload', () => {
  const history: Message[] = Array.from({ length: 101 }, (_, index) => ({
    key: `message-${index}`,
    from: 'user',
    versions: [{ id: `version-${index}`, content: `Message ${index}` }],
  }))

  saveMessages(history, 1)

  expect(loadMessages(1)).toEqual(history)
})

test('repeated numbered headings in a valid message are not treated as duplicate streaming snapshots', () => {
  const section = `## 1. Daily observations\n${'Observation '.repeat(100)}\n`
  const repeatedMessages: Message[] = [
    {
      key: 'repeated-section',
      from: 'user',
      versions: [
        {
          id: 'section-version',
          content: `${section}${section}${section}Final observation`,
        },
      ],
    },
  ]

  saveMessages(repeatedMessages, 1)

  expect(loadMessages(1)).toEqual(repeatedMessages)
})

test('recovering an interrupted response retains its original persisted record', () => {
  const interrupted: Message[] = [
    {
      key: 'interrupted',
      from: 'assistant',
      status: 'streaming',
      createdAt: 1000,
      versions: [
        {
          id: 'partial',
          content: '<think>unfinished reasoning</think>partial answer',
        },
      ],
    },
  ]
  saveMessages(interrupted, 1)
  const saved = localStorage.getItem(`${STORAGE_KEYS.MESSAGES}:1`)

  expect(loadMessages(1)?.[0]).toMatchObject({
    status: 'complete',
    versions: [{ id: 'partial', content: 'partial answer' }],
    reasoning: { content: 'unfinished reasoning' },
  })
  expect(localStorage.getItem(`${STORAGE_KEYS.MESSAGES}:1`)).toBe(saved)
})

test('a rejected storage write reports failure and preserves the last saved conversation', () => {
  expect(saveMessages(messages, 1)).toBe(true)
  const saved = localStorage.getItem(`${STORAGE_KEYS.MESSAGES}:1`)
  vi.spyOn(console, 'error').mockImplementation(() => undefined)
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
    throw new DOMException('Storage is full', 'QuotaExceededError')
  })

  expect(
    saveMessages(
      [
        ...messages,
        {
          key: 'new',
          from: 'user',
          versions: [{ id: 'new-v', content: 'Unsaved instruction' }],
        },
      ],
      1
    )
  ).toBe(false)
  expect(localStorage.getItem(`${STORAGE_KEYS.MESSAGES}:1`)).toBe(saved)
})

test('recovering an interrupted response and saving another turn retains all earlier versions', () => {
  const interrupted: Message[] = [
    {
      key: 'interrupted-versions',
      from: 'assistant',
      status: 'streaming',
      versions: [
        { id: 'current', content: '<think>reasoning</think>partial reply' },
        { id: 'earlier', content: 'Earlier reply' },
      ],
    },
  ]
  saveMessages(interrupted, 1)

  const recovered = loadMessages(1)
  expect(recovered?.[0].versions).toEqual([
    { id: 'current', content: 'partial reply' },
    { id: 'earlier', content: 'Earlier reply' },
  ])
  saveMessages([...(recovered ?? []), ...messages], 1)

  expect(loadMessages(1)?.[0].versions).toEqual(recovered?.[0].versions)
})
