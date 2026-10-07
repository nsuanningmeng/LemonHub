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
import { cleanup, render, screen } from '@testing-library/react'
import type { PropsWithChildren } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { Contact } from '..'

vi.mock('@/components/layout', () => ({
  PublicLayout: (props: PropsWithChildren) => <main>{props.children}</main>,
}))

afterEach(cleanup)

function renderContact(content: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { enabled: false, retry: false } },
  })
  client.setQueryData(['contact-content'], { success: true, data: content })
  return render(
    <QueryClientProvider client={client}>
      <Contact />
    </QueryClientProvider>
  )
}

test('configured contact HTML removes executable content and retains safe formatting', () => {
  renderContact(`
    <h2>Support team</h2>
    <a href="https://example.com/support">Support</a>
    <img alt="Brand" src="https://example.com/logo.png" onerror="alert(document.domain)">
    <a href="javascript:alert(document.domain)">Unsafe link</a>
    <svg onload="alert(document.domain)"></svg>
    <script>alert(document.domain)</script>
    <iframe srcdoc="<script>alert(parent.document.domain)</script>"></iframe>
  `)

  expect(screen.getByRole('heading', { name: 'Support team' })).toBeVisible()
  expect(screen.getByRole('link', { name: 'Support' })).toHaveAttribute(
    'href',
    'https://example.com/support'
  )
  expect(screen.getByAltText('Brand')).not.toHaveAttribute('onerror')
  expect(screen.getByText('Unsafe link')).not.toHaveAttribute('href')
  expect(
    screen.getByRole('main').querySelector('script, iframe, [onload]')
  ).toBeNull()
})

test('configured contact URL allows embedded scripts without granting the site origin', () => {
  renderContact('https://example.com/contact')
  const frame = screen.getByTitle('Contact Us')
  expect(frame).toHaveAttribute('src', 'https://example.com/contact')
  const sandbox = frame.getAttribute('sandbox')?.split(/\s+/) ?? []
  expect(sandbox).toContain('allow-scripts')
  expect(sandbox).not.toContain('allow-same-origin')
  expect(frame).toHaveAttribute('referrerpolicy', 'no-referrer')
})

test('a complete contact document keeps its styles without allowing executable content or site-origin access', () => {
  renderContact(`<!DOCTYPE html>
    <html><head>
      <style>.card-icon svg { width: 24px; height: 24px; }</style>
      <base href="https://untrusted.example/" target="_top">
      <meta http-equiv="refresh" content="0; url=https://untrusted.example/">
      <script>alert(document.domain)</script>
    </head><body onload="alert(document.domain)">
      <h2>Support team</h2>
      <a href="https://example.com/support" target="_blank">Support</a>
      <img alt="Brand" src="https://example.com/logo.png" onerror="alert(document.domain)">
      <a href="javascript:alert(document.domain)">Unsafe link</a>
      <iframe srcdoc="<script>alert(parent.document.domain)</script>"></iframe>
    </body></html>`)

  const frame = screen.getByTitle('Contact Us')
  const doc = new DOMParser().parseFromString(
    frame.getAttribute('srcdoc') ?? '',
    'text/html'
  )
  expect(doc.head.querySelector('style')?.textContent).toContain(
    '.card-icon svg { width: 24px; height: 24px; }'
  )
  expect(
    doc.querySelector('script, iframe, base, meta, [onload], [onerror]')
  ).toBeNull()
  expect(doc.querySelector('a[href^="javascript:"]')).toBeNull()
  expect(
    doc
      .querySelector('a[href="https://example.com/support"]')
      ?.getAttribute('target')
  ).toBe('_blank')
  const sandbox = frame.getAttribute('sandbox')?.split(/\s+/) ?? []
  expect(sandbox).not.toContain('allow-scripts')
  expect(sandbox).not.toContain('allow-same-origin')
  expect(sandbox).toContain('allow-popups')
  expect(sandbox).toContain('allow-popups-to-escape-sandbox')
  expect(frame).toHaveAttribute('referrerpolicy', 'no-referrer')
})
