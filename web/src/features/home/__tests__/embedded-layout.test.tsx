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
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { Home } from '..'

const sandboxDescriptor = Object.getOwnPropertyDescriptor(
  HTMLIFrameElement.prototype,
  'sandbox'
)

beforeEach(() => {
  // jsdom does not implement this browser DOM API.
  Object.defineProperty(HTMLIFrameElement.prototype, 'sandbox', {
    configurable: true,
    get(this: HTMLIFrameElement) {
      const tokens = document.createElement('div').classList
      tokens.value = this.getAttribute('sandbox') ?? ''
      return tokens
    },
  })
})

afterEach(() => {
  cleanup()
  if (sandboxDescriptor) {
    Object.defineProperty(
      HTMLIFrameElement.prototype,
      'sandbox',
      sandboxDescriptor
    )
  } else {
    Reflect.deleteProperty(HTMLIFrameElement.prototype, 'sandbox')
  }
  localStorage.clear()
  vi.restoreAllMocks()
})

test('a configured URL keeps embedded content below navigation and within the remaining viewport', async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { enabled: false, retry: false } },
  })
  client.setQueryDefaults(['home-page-content'], { enabled: true })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: 'https://support.example.test/' },
  })
  const router = createRouter({
    routeTree: createRootRoute({ component: Home }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )

  const frame = await screen.findByTitle('Custom Home Page')
  expect(frame.closest('main')).toHaveClass('pt-16')
  expect(frame).toHaveClass('w-full', 'h-[calc(100svh-4rem)]')
  expect(frame).toHaveAttribute('src', 'https://support.example.test/')
  client.clear()
})

test('configured HTML remains below the fixed header and supplies the skip-link main target', async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { enabled: false, retry: false } },
  })
  client.setQueryDefaults(['home-page-content'], { enabled: true })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: '<h1>Custom studio</h1>' },
  })
  const router = createRouter({
    routeTree: createRootRoute({ component: Home }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  await waitFor(() =>
    expect(document.querySelector('.custom-home-content')).toBeInTheDocument()
  )
  expect(screen.getByRole('main')).toHaveAttribute('id', 'main-content')
  expect(screen.getByRole('main')).toHaveClass('pt-16')
  expect(
    screen.getByRole('main').querySelector('.custom-home-content')
  ).toBeInTheDocument()
  client.clear()
})

test('configured Markdown renders readable headings and links in the standard main area', async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { enabled: false, retry: false } },
  })
  client.setQueryDefaults(['home-page-content'], { enabled: true })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: '# Team workspace\n\n[Read the guide](https://docs.example.test/)',
    },
  })
  const router = createRouter({
    routeTree: createRootRoute({ component: Home }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  const title = await screen.findByRole('heading', { name: 'Team workspace' })
  expect(title.closest('main')).toHaveAttribute('id', 'main-content')
  expect(screen.getByRole('link', { name: 'Read the guide' })).toHaveAttribute(
    'href',
    'https://docs.example.test/'
  )
  client.clear()
})

test('a complete HTML home preserves body and root styles in its own document', async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { enabled: false, retry: false } },
  })
  client.setQueryDefaults(['home-page-content'], { enabled: true })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: '<!doctype html><html lang="zh-CN"><head><style>:root { --page-bg: cyan; } body { margin: 0; background: var(--page-bg); }</style></head><body class="studio"><h1>Studio</h1></body></html>',
    },
  })
  const router = createRouter({
    routeTree: createRootRoute({ component: Home }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  const frame = await screen.findByTitle('Custom Home Page')
  const doc = new DOMParser().parseFromString(
    frame.getAttribute('srcdoc') ?? '',
    'text/html'
  )
  expect(doc.documentElement.lang).toBe('zh-CN')
  expect(doc.body.classList.contains('studio')).toBe(true)
  expect(doc.head.querySelector('style')?.textContent).toContain(
    'body { margin: 0; background: var(--page-bg); }'
  )
  expect(frame.closest('main')).toHaveClass('pt-16')
  expect(frame).toHaveClass('h-[calc(100svh-4rem)]')
  client.clear()
})
