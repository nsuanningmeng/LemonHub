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
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, test } from 'vitest'

import { About } from '..'

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
})

test('a configured URL keeps embedded content below navigation and within the remaining viewport', async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { enabled: false, retry: false } },
  })
  client.setQueryData(['about-content'], {
    success: true,
    data: 'https://support.example.test/',
  })
  const router = createRouter({
    routeTree: createRootRoute({ component: About }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )

  const frame = await screen.findByTitle('About')
  expect(frame.closest('main')).toHaveClass('pt-16')
  expect(frame).toHaveClass('w-full', 'h-[calc(100svh-4rem)]')
  expect(frame).toHaveAttribute('src', 'https://support.example.test/')
  client.clear()
})
