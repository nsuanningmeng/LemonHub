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
import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, test } from 'vitest'

import type { AnnouncementItem } from '@/features/dashboard/types'

import { AnnouncementBanner } from '../announcement-banner'

let queryClient: QueryClient
const originalGetAnimations = Object.getOwnPropertyDescriptor(
  Element.prototype,
  'getAnimations'
)

beforeEach(() => {
  // jsdom does not implement the Web Animations API used by Base UI ScrollArea.
  Object.defineProperty(Element.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
  queryClient = new QueryClient({
    defaultOptions: { queries: { enabled: false, retry: false } },
  })
})

afterEach(() => {
  cleanup()
  queryClient.clear()
  localStorage.clear()
  if (originalGetAnimations) {
    Object.defineProperty(
      Element.prototype,
      'getAnimations',
      originalGetAnimations
    )
  } else {
    Reflect.deleteProperty(Element.prototype, 'getAnimations')
  }
})

function renderBanner(announcements: AnnouncementItem[], enabled = true) {
  queryClient.setQueryData(['status'], {
    announcements_enabled: enabled,
    announcements,
  })

  return render(
    <QueryClientProvider client={queryClient}>
      <AnnouncementBanner />
    </QueryClientProvider>
  )
}

describe('Announcement banner', () => {
  test('shows the newest five announcements in descending date order without an automatic dialog', () => {
    renderBanner([
      { id: 3, content: 'Update C', publishDate: '2026-09-03T00:00:00Z' },
      { id: 1, content: 'Update A', publishDate: '2026-09-01T00:00:00Z' },
      { id: 6, content: 'Update F', publishDate: '2026-09-06T00:00:00Z' },
      { id: 2, content: 'Update B', publishDate: '2026-09-02T00:00:00Z' },
      { id: 5, content: 'Update E', publishDate: '2026-09-05T00:00:00Z' },
      { id: 4, content: 'Update D', publishDate: '2026-09-04T00:00:00Z' },
    ])

    const banner = screen.getByRole('region', { name: 'Latest announcements' })
    const announcements = within(banner).getAllByRole('button', {
      name: /Update [A-F]/,
    })
    expect(announcements).toHaveLength(5)
    expect(announcements[0]).toHaveAccessibleName(/Update F/)
    expect(announcements[1]).toHaveAccessibleName(/Update E/)
    expect(announcements[2]).toHaveAccessibleName(/Update D/)
    expect(announcements[3]).toHaveAccessibleName(/Update C/)
    expect(announcements[4]).toHaveAccessibleName(/Update B/)
    expect(banner).not.toHaveTextContent('Update A')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  test.each([
    {
      description: 'the announcement feature is disabled',
      enabled: false,
      announcements: [{ id: 1, content: 'Hidden update' }],
    },
    {
      description: 'there are no announcements',
      enabled: true,
      announcements: [],
    },
  ])('hides the banner when $description', ({ announcements, enabled }) => {
    const { container } = renderBanner(announcements, enabled)

    expect(container).toBeEmptyDOMElement()
    expect(
      screen.queryByRole('region', { name: 'Latest announcements' })
    ).not.toBeInTheDocument()
  })

  test('clicking a plain text summary opens the complete rich announcement and additional information', async () => {
    const user = userEvent.setup()
    renderBanner([
      {
        id: 1,
        content:
          '## Availability update\n\n**Scheduled maintenance** will improve request routing, account statistics, and service reliability across the platform. Existing API keys remain valid throughout the upgrade. Full detail: requests resume at 09:30 UTC.',
        publishDate: '2026-09-01T00:00:00Z',
        extra: 'Contact support if requests do not resume.',
      },
    ])

    const summary = screen.getByRole('button', { name: /Availability update/ })
    expect(summary).toHaveTextContent(
      'Availability update Scheduled maintenance'
    )
    expect(summary).not.toHaveTextContent('##')
    expect(summary).not.toHaveTextContent('**')
    expect(summary).not.toHaveTextContent('requests resume at 09:30 UTC')

    await user.click(summary)

    const dialog = await screen.findByRole('dialog', {
      name: 'Announcement Details',
    })
    expect(dialog).toHaveTextContent(
      'Full detail: requests resume at 09:30 UTC.'
    )
    expect(
      within(dialog).getByRole('heading', { name: 'Availability update' })
    ).toBeInTheDocument()
    expect(dialog).toHaveTextContent(
      'Contact support if requests do not resume.'
    )
  })

  test.each([
    {
      field: 'content',
      announcement: {
        id: 1,
        content:
          'Security update\n\n<script>1</script>\n<img src=x onerror=alert(1)>\n\n[Bad](javascript:alert%281%29) [Safe](https://example.com/help)',
      },
      safeUrl: 'https://example.com/help',
    },
    {
      field: 'extra',
      announcement: {
        id: 1,
        content: 'Security update',
        extra:
          '<script>1</script>\n<img src=x onerror=x>\n\n[Bad](data:text/html,x) [Safe](https://a.test)',
      },
      safeUrl: 'https://a.test',
    },
  ])(
    'opening an announcement sanitizes executable $field content while preserving safe external links',
    async ({ announcement, safeUrl }) => {
      const user = userEvent.setup()
      renderBanner([announcement])

      const banner = screen.getByRole('region', {
        name: 'Latest announcements',
      })
      expect(banner.querySelector('script, img, a')).toBeNull()

      await user.click(
        within(banner).getByRole('button', { name: /Security update/ })
      )

      const dialog = await screen.findByRole('dialog', {
        name: 'Announcement Details',
      })
      expect(dialog.querySelector('script, [onerror]')).toBeNull()
      expect(within(dialog).getByText('Bad')).not.toHaveAttribute('href')

      const safeLink = within(dialog).getByRole('link', { name: 'Safe' })
      expect(safeLink).toHaveAttribute('href', safeUrl)
      expect(safeLink).toHaveAttribute('target', '_blank')
      expect(safeLink).toHaveAttribute('rel', 'noopener noreferrer')
    }
  )

  test('the pause control announces its paused state and can resume scrolling', async () => {
    const user = userEvent.setup()
    renderBanner([
      { id: 1, content: 'First service update' },
      { id: 2, content: 'Second service update' },
    ])

    const pause = screen.getByRole('button', { name: 'Pause announcements' })
    expect(pause).toHaveAttribute('aria-pressed', 'false')

    await user.click(pause)

    const resume = screen.getByRole('button', { name: 'Resume announcements' })
    expect(resume).toHaveAttribute('aria-pressed', 'true')

    await user.click(resume)

    expect(
      screen.getByRole('button', { name: 'Pause announcements' })
    ).toHaveAttribute('aria-pressed', 'false')
  })

  test('keyboard users can focus an announcement and open its details with Enter', async () => {
    const user = userEvent.setup()
    renderBanner([
      {
        id: 1,
        content: 'Keyboard accessible update',
        extra: 'Complete keyboard announcement details.',
      },
    ])

    const announcement = screen.getByRole('button', {
      name: /Keyboard accessible update/,
    })
    announcement.focus()
    expect(announcement).toHaveFocus()

    await user.keyboard('{Enter}')

    const dialog = await screen.findByRole('dialog', {
      name: 'Announcement Details',
    })
    expect(dialog).toHaveTextContent('Complete keyboard announcement details.')
  })
})
