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
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { PageFooterPortal } from '../page-footer'
import { SectionPageLayout } from '../section-page-layout'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

test('long action groups wrap before squeezing the page title away', () => {
  render(
    <SectionPageLayout>
      <SectionPageLayout.Title>Support tickets</SectionPageLayout.Title>
      <SectionPageLayout.Description>
        Manage support requests
      </SectionPageLayout.Description>
      <SectionPageLayout.Actions>
        <button type='button'>Filter by ticket status</button>
        <button type='button'>Create ticket</button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>Ticket list</SectionPageLayout.Content>
    </SectionPageLayout>
  )
  const title = screen.getByRole('heading', { name: 'Support tickets' })
  expect(title.parentElement).toHaveClass('basis-32')
  expect(title.parentElement).toContainElement(
    screen.getByText('Manage support requests')
  )
  expect(
    screen.getByRole('button', { name: 'Create ticket' }).parentElement
  ).toHaveClass('max-w-full', 'min-w-0')
})

test.each([false, true])(
  'fixedContent=%s keeps desktop scrolling inside the content region and exposes a page heading',
  (fixedContent) => {
    const matchMedia = window.matchMedia
    vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
      ...matchMedia(query),
      matches: query === '(min-width: 768px) and (min-height: 640px)',
    }))
    render(
      <SectionPageLayout fixedContent={fixedContent}>
        <SectionPageLayout.Title>Usage analytics</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <article aria-label='Usage chart'>Daily usage</article>
        </SectionPageLayout.Content>
      </SectionPageLayout>
    )
    expect(
      screen.getByRole('heading', { name: 'Usage analytics', level: 1 })
    ).toBeVisible()
    expect(
      screen.getByRole('article', { name: 'Usage chart' }).parentElement
    ).toHaveClass('min-h-0', fixedContent ? 'overflow-hidden' : 'overflow-auto')
  }
)

test.each([false, true])(
  'fixedContent=%s lets the title, content and pagination scroll together in a compact viewport',
  (fixedContent) => {
    render(
      <SectionPageLayout fixedContent={fixedContent}>
        <SectionPageLayout.Title>API access</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <article aria-label='API keys'>Production key</article>
          <PageFooterPortal>
            <nav aria-label='Table pagination'>Next page</nav>
          </PageFooterPortal>
        </SectionPageLayout.Content>
      </SectionPageLayout>
    )
    const main = screen.getByRole('main')
    expect(main).toHaveClass('overflow-y-auto')
    expect(screen.getByRole('article').parentElement).toHaveClass('shrink-0')
    expect(screen.getByRole('article').parentElement).not.toHaveClass(
      'overflow-hidden',
      'flex-1'
    )
    expect(main).toContainElement(
      screen.getByRole('navigation', { name: 'Table pagination' })
    )
  }
)
