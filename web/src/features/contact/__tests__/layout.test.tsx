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
import { afterEach, expect, test } from 'vitest'

import { Contact } from '..'

const client = new QueryClient({
  defaultOptions: { queries: { enabled: false, retry: false } },
})

afterEach(() => {
  cleanup()
  client.clear()
  localStorage.clear()
})

async function renderContact(content: string) {
  client.setQueryData(['contact-content'], { success: true, data: content })
  const router = createRouter({
    routeTree: createRootRoute({ component: Contact }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  return render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

test('a complete contact document preserves its root styles, icon sizing and responsive layout in an isolated frame', async () => {
  await renderContact(`<!DOCTYPE html>
    <html lang="zh-CN">
      <head><style>
        :root { --contact-width: 960px; --contact-color: #3f6212; }
        body { margin: 0; color: var(--contact-color); }
        .page { max-width: var(--contact-width); margin: 0 auto; }
        .cards { display: grid; grid-template-columns: 1fr; }
        .card-icon svg { width: 24px; height: 24px; }
        @media (min-width: 600px) {
          .cards { grid-template-columns: repeat(2, 1fr); }
        }
      </style></head>
      <body class="contact-page">
        <main class="page"><section class="cards">
          <article class="card-icon"><svg viewBox="0 0 24 24"></svg>Email</article>
          <article>Telegram</article>
        </section></main>
      </body>
    </html>`)

  const frame = screen.getByTitle('Contact Us')
  const doc = new DOMParser().parseFromString(
    frame.getAttribute('srcdoc') ?? '',
    'text/html'
  )
  expect(doc.doctype?.name).toBe('html')
  expect(doc.documentElement.getAttribute('lang')).toBe('zh-CN')
  expect(doc.body.classList.contains('contact-page')).toBe(true)
  expect(doc.head.querySelector('style')?.textContent).toContain(
    ':root { --contact-width: 960px; --contact-color: #3f6212; }'
  )
  expect(doc.head.querySelector('style')?.textContent).toContain(
    '.card-icon svg { width: 24px; height: 24px; }'
  )
  expect(doc.head.querySelector('style')?.textContent).toContain(
    '@media (min-width: 600px)'
  )
  expect(doc.querySelector('.cards')?.children).toHaveLength(2)
  expect(document.head.textContent).not.toContain('--contact-width')
  expect(frame).toHaveClass('w-full', 'h-[calc(100svh-4rem)]')
  expect(screen.getByRole('main')).toHaveClass('pt-16')
})

test.each([
  ['an internal link without a target', '<a href="/pricing">Pricing</a>'],
  [
    'an external link targeting the current frame',
    '<a href="https://example.com/support" target="_self">Support</a>',
  ],
  [
    'an external link targeting the top page',
    '<a href="https://example.com/support" target="_top">Support</a>',
  ],
])(
  'a complete contact document opens %s outside the restricted frame',
  async (_description, linkHtml) => {
    await renderContact(`<!DOCTYPE html><html><body>${linkHtml}</body></html>`)

    const frame = screen.getByTitle('Contact Us')
    const doc = new DOMParser().parseFromString(
      frame.getAttribute('srcdoc') ?? '',
      'text/html'
    )
    const link = doc.querySelector('a')
    expect(link?.getAttribute('target')).toBe('_blank')
    expect(link?.getAttribute('rel')?.split(/\s+/)).toEqual(
      expect.arrayContaining(['noopener', 'noreferrer'])
    )
  }
)

test('a complete contact document keeps fragment links within the document', async () => {
  await renderContact(`<!DOCTYPE html><html><body>
    <a href="#details">Contact details</a>
    <section id="details">Email support</section>
  </body></html>`)

  const frame = screen.getByTitle('Contact Us')
  const doc = new DOMParser().parseFromString(
    frame.getAttribute('srcdoc') ?? '',
    'text/html'
  )
  const link = doc.querySelector('a')
  expect(link?.getAttribute('href')).toBe('about:srcdoc#details')
  expect(link?.getAttribute('target')).toBeNull()
})

test.each([
  ['HTML fragment', '<h2>Support team</h2>'],
  ['Markdown', '## Support team'],
])(
  '%s contact content remains directly readable without a document frame',
  async (_format, content) => {
    await renderContact(content)
    expect(screen.getByRole('heading', { name: 'Support team' })).toBeVisible()
    expect(screen.queryByTitle('Contact Us')).toBeNull()
  }
)
