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
import { AxiosError } from 'axios'
import i18next from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { toast } from 'sonner'
import { afterAll, afterEach, beforeAll, expect, test, vi } from 'vitest'

import { createLocaleI18n } from '@/i18n/__tests__/helpers'
import { api } from '@/lib/api'

import WebsiteTraffic from '../website-traffic'

const counts = { page_views: 7, unique_visitors: 2, new_visitors: 1 }
const weekResponse = {
  data: {
    success: true,
    data: {
      summary: counts,
      daily: [
        {
          date: '2026-11-20',
          page_views: 3,
          unique_visitors: 2,
          new_visitors: 1,
        },
        {
          date: '2026-11-21',
          page_views: 4,
          unique_visitors: 1,
          new_visitors: 0,
        },
      ],
    },
  },
}

const needsAnimationsStub = !Element.prototype.getAnimations
beforeAll(() => {
  if (needsAnimationsStub) Element.prototype.getAnimations = () => []
})
afterAll(() => {
  if (needsAnimationsStub) {
    Reflect.deleteProperty(Element.prototype, 'getAnimations')
  }
})

let client: QueryClient
function renderTraffic(locale = i18next) {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={locale}>
        <WebsiteTraffic />
      </I18nextProvider>
    </QueryClientProvider>
  )
}

afterEach(() => {
  cleanup()
  client.clear()
})

test('shows the distinct visitor total for the period and the latest day first', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue(weekResponse)
  renderTraffic()
  expect(screen.getByRole('status', { name: 'Loading' })).toBeVisible()

  const visitors = await screen.findByRole('group', { name: 'Unique visitors' })
  expect(within(visitors).getByText('2')).toBeVisible()
  expect(
    screen.getAllByRole('definition').map((metric) => metric.textContent)
  ).toEqual(['1', '3.5'])
  const table = screen.getByRole('table', { name: 'Daily traffic (UTC)' })
  const days = within(table).getAllByRole('rowheader')
  expect(days.map((day) => day.textContent)).toEqual([
    '2026-11-21',
    '2026-11-20',
  ])
  expect(get).toHaveBeenCalledWith(
    '/api/site-analytics',
    expect.objectContaining({ params: { days: 7 } })
  )
})

test.each(['zhCN', 'zhTW'] as const)(
  'renders the totals, average and daily table with the %s interface language',
  async (language) => {
    const locale = await createLocaleI18n(language)
    vi.spyOn(api, 'get').mockResolvedValue(weekResponse)
    renderTraffic(locale)
    const table = await screen.findByRole('table', {
      name: locale.t('Daily traffic (UTC)'),
    })
    expect(within(table).getAllByRole('rowheader')).toHaveLength(2)
    const views = screen.getByRole('group', { name: locale.t('Page views') })
    expect(within(views).getByText('7')).toBeVisible()
    expect(screen.getByText('3.5')).toBeVisible()
  }
)

test('selecting another period replaces the report with its own empty state', async () => {
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValueOnce(weekResponse)
    .mockResolvedValueOnce({
      data: {
        success: true,
        data: {
          summary: { page_views: 0, unique_visitors: 0, new_visitors: 0 },
          daily: [
            {
              date: '2026-11-22',
              page_views: 0,
              unique_visitors: 0,
              new_visitors: 0,
            },
          ],
        },
      },
    })
  renderTraffic()
  await screen.findByRole('table')
  const today = screen.getByRole('button', { name: 'Today' })
  await userEvent.click(today)
  expect(
    await screen.findByText('No page views recorded in this period.')
  ).toBeVisible()
  expect(today).toHaveAttribute('aria-pressed', 'true')
  expect(screen.queryByRole('table')).toBeNull()
  expect(
    screen.getAllByRole('definition').map((metric) => metric.textContent)
  ).toEqual(['0', '—'])
  expect(get).toHaveBeenLastCalledWith(
    '/api/site-analytics',
    expect.objectContaining({ params: { days: 1 } })
  )
})

test('a failed report shows a retry that recovers the data', async () => {
  vi.spyOn(api, 'get')
    .mockResolvedValueOnce({ data: { success: false, message: 'unavailable' } })
    .mockResolvedValueOnce(weekResponse)
  renderTraffic()
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Failed to load website traffic'
  )
  expect(screen.queryByRole('group', { name: 'Page views' })).toBeNull()
  await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
  expect(await screen.findByRole('table')).toBeVisible()
})

test('an HTTP 500 stays in the report and a manual retry recovers the data', async () => {
  const originalAdapter = api.defaults.adapter
  const notify = vi.spyOn(toast, 'error')
  let recovered = false
  api.defaults.adapter = async (config) => {
    if (!recovered) {
      throw new AxiosError(
        'Unavailable',
        AxiosError.ERR_BAD_RESPONSE,
        config,
        undefined,
        {
          status: 500,
          statusText: 'Server Error',
          data: { success: false, message: 'Unavailable' },
          headers: {},
          config,
        }
      )
    }
    return {
      ...weekResponse,
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    }
  }
  try {
    renderTraffic()
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Failed to load website traffic'
    )
    // The app's global query handler redirects HTTP 500 AxiosErrors to /500.
    expect(client.getQueryCache().getAll()[0].state.error).not.toBeInstanceOf(
      AxiosError
    )
    expect(notify).not.toHaveBeenCalled()

    recovered = true
    await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByRole('table')).toBeVisible()
  } finally {
    api.defaults.adapter = originalAdapter
  }
})
