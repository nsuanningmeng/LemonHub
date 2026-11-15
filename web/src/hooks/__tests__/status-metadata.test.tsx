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
import { cleanup, renderHook, waitFor, act } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { useStatus } from '@/hooks/use-status'
import { api } from '@/lib/api'
import { useSystemConfigStore } from '@/stores/system-config-store'

const originalState = useSystemConfigStore.getState()
const clients: QueryClient[] = []
afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  useSystemConfigStore.setState(originalState, true)
  document.head.innerHTML = ''
  localStorage.clear()
  vi.restoreAllMocks()
})

test('a refreshed status immediately updates the title and resets cleared SEO fields to page defaults', async () => {
  document.head.innerHTML =
    '<title>LemonHub</title><meta name="title" content="LemonHub"><meta name="description" content="Default description"><meta name="keywords" content="Default keywords">'
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      data: {
        system_name: 'Research',
        seo_description: 'Research description',
        seo_keywords: 'research, api',
      },
    },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  const { result } = renderHook(() => useStatus(), {
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    ),
  })
  await waitFor(() =>
    expect(result.current.status?.system_name).toBe('Research')
  )
  expect(document.title).toBe('Research')
  expect(document.querySelector('meta[name="description"]')).toHaveAttribute(
    'content',
    'Research description'
  )
  get.mockResolvedValue({
    data: {
      data: {
        system_name: 'Research Studio',
        seo_description: '',
        seo_keywords: '',
      },
    },
  })
  await act(() => client.invalidateQueries({ queryKey: ['status'] }))
  await waitFor(() => expect(document.title).toBe('Research Studio'))
  expect(document.querySelector('meta[name="title"]')).toHaveAttribute(
    'content',
    'Research Studio'
  )
  expect(document.querySelector('meta[name="description"]')).toHaveAttribute(
    'content',
    'Default description'
  )
  expect(document.querySelector('meta[name="keywords"]')).toHaveAttribute(
    'content',
    'Default keywords'
  )
  expect(JSON.parse(localStorage.getItem('status') ?? '{}').system_name).toBe(
    'Research Studio'
  )
})
