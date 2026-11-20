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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import { AxiosError, AxiosHeaders } from 'axios'
import type { ReactNode } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { useHomePageContent } from '../use-home-page-content'

const clients: QueryClient[] = []
afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  vi.restoreAllMocks()
  localStorage.clear()
})
function renderContent() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  const hook = renderHook(() => useHomePageContent(), {
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    ),
  })
  return { ...hook, client }
}

test('invalidating a saved home page replaces the mounted content and clearing it restores the default home', async () => {
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValue({ data: { success: true, data: '# Team home' } })
  const { result, client } = renderContent()
  await waitFor(() => expect(result.current.content).toBe('# Team home'))
  get.mockResolvedValue({
    data: { success: true, data: 'https://team.example.test/' },
  })
  await act(() => client.invalidateQueries({ queryKey: ['home-page-content'] }))
  await waitFor(() => expect(result.current.isUrl).toBe(true))
  expect(localStorage.getItem('home_page_content')).toBe(
    'https://team.example.test/'
  )
  get.mockResolvedValue({ data: { success: true, data: '' } })
  await act(() => client.invalidateQueries({ queryKey: ['home-page-content'] }))
  await waitFor(() => expect(result.current.content).toBe(''))
  expect(result.current.isLoaded).toBe(true)
  expect(localStorage.getItem('home_page_content')).toBeNull()
})

test('a server rejection preserves the cached custom home instead of erasing it', async () => {
  localStorage.setItem('home_page_content', '# Cached home')
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: false, message: 'Unavailable' },
  })
  const { result, client } = renderContent()
  await waitFor(() =>
    expect(client.getQueryState(['home-page-content'])?.status).toBe('error')
  )
  expect(result.current.isLoaded).toBe(true)
  expect(result.current.content).toBe('# Cached home')
  expect(localStorage.getItem('home_page_content')).toBe('# Cached home')
})

test('blocked browser storage still allows the custom home to load', async () => {
  vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
    throw new Error('Storage blocked')
  })
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
    throw new Error('Storage blocked')
  })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: '# Live home' },
  })
  const { result } = renderContent()
  await waitFor(() => expect(result.current.isLoaded).toBe(true))
  expect(result.current.content).toBe('# Live home')
})

test('a network failure is handled locally while the cached home remains visible', async () => {
  localStorage.setItem('home_page_content', '# Saved studio')
  const failure = new AxiosError('Server unavailable')
  failure.response = {
    status: 500,
    data: {},
    statusText: 'Server error',
    headers: {},
    config: { headers: new AxiosHeaders() },
  }
  vi.spyOn(api, 'get').mockRejectedValue(failure)
  const { result, client } = renderContent()
  await waitFor(() =>
    expect(client.getQueryState(['home-page-content'])?.status).toBe('error')
  )
  expect(result.current.content).toBe('# Saved studio')
  expect(result.current.isLoaded).toBe(true)
  // This domain error must not trigger the app-wide Axios 500 page redirect.
  expect(client.getQueryState(['home-page-content'])?.error).not.toBeInstanceOf(
    AxiosError
  )
})
