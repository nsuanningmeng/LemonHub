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
import {
  QueryCache,
  QueryClient,
  QueryClientProvider,
} from '@tanstack/react-query'
import { act, cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AxiosError, AxiosHeaders } from 'axios'
import type { i18n } from 'i18next'
import { StrictMode } from 'react'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { createLocaleI18n } from '@/i18n/__tests__/helpers'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import type { UsageActivity } from '../../types'
import { UsageActivityCard } from '../usage-activity-card'

const activity: UsageActivity = {
  timezone: 'Asia/Shanghai',
  as_of: '2026-10-12T12:00:00+08:00',
  summary: {
    total_tokens: 1300,
    peak_daily_tokens: 600,
    current_streak_days: 2,
    longest_streak_days: 2,
    longest_request_seconds: 95,
  },
  daily_usage: [
    { date: '2025-12-31', tokens: 300 },
    { date: '2026-10-11', tokens: 400 },
    { date: '2026-10-12', tokens: 600 },
  ],
}

const originalAuthState = useAuthStore.getState()
let queryClient: QueryClient
let englishI18n: i18n

beforeEach(async () => {
  localStorage.clear()
  englishI18n = await createLocaleI18n('en')
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'first-user', role: 1 })
})

afterEach(() => {
  cleanup()
  queryClient.clear()
  useAuthStore.setState(originalAuthState, true)
  localStorage.clear()
})

function renderActivityCard() {
  return render(
    <I18nextProvider i18n={englishI18n}>
      <QueryClientProvider client={queryClient}>
        <UsageActivityCard />
      </QueryClientProvider>
    </I18nextProvider>
  )
}

describe('profile usage activity', () => {
  test('shows loading until the authenticated request returns and sends only the browser time zone', async () => {
    let completeRequest = () => {}
    const requestPending = new Promise<void>((resolve) => {
      completeRequest = resolve
    })
    const get = vi.spyOn(api, 'get').mockImplementation(async () => {
      await requestPending
      return { data: { success: true, data: activity } }
    })

    renderActivityCard()

    expect(screen.getByRole('status')).toHaveTextContent(
      'Loading usage activity'
    )
    expect(
      screen.getByRole('button', { name: 'Refresh usage activity' })
    ).toBeDisabled()
    expect(
      screen.queryByRole('definition', { name: 'Total tokens: 1,300' })
    ).not.toBeInTheDocument()
    expect(get).toHaveBeenCalledWith('/api/user/self/activity', {
      params: {
        timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC',
      },
      signal: expect.any(AbortSignal),
      disableDuplicate: true,
      skipErrorHandler: true,
      skipBusinessError: true,
    })

    completeRequest()

    expect(
      await screen.findByRole('definition', { name: 'Total tokens: 1,300' })
    ).toBeVisible()
    expect(
      screen.getByRole('definition', { name: 'Peak daily tokens: 600' })
    ).toBeVisible()
    expect(
      screen.getByRole('definition', { name: 'Current streak: 2: days' })
    ).toBeVisible()
    expect(
      screen.getByRole('definition', { name: 'Longest request: 95: seconds' })
    ).toBeVisible()
    expect(screen.queryByText('Loading usage activity')).not.toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Refresh usage activity' })
    ).toBeEnabled()
    expect(screen.getByText('Time zone: Asia/Shanghai')).toBeVisible()
  })

  test('loads activity after StrictMode aborts the first mounted request', async () => {
    const get = vi
      .spyOn(api, 'get')
      .mockImplementationOnce(
        (_url, config) =>
          new Promise((_resolve, reject) => {
            config?.signal?.addEventListener?.(
              'abort',
              () => {
                reject(new DOMException('Request aborted', 'AbortError'))
              },
              { once: true }
            )
          })
      )
      .mockResolvedValueOnce({ data: { success: true, data: activity } })

    render(
      <StrictMode>
        <I18nextProvider i18n={englishI18n}>
          <QueryClientProvider client={queryClient}>
            <UsageActivityCard />
          </QueryClientProvider>
        </I18nextProvider>
      </StrictMode>
    )

    expect(
      await screen.findByRole('definition', { name: 'Total tokens: 1,300' })
    ).toBeVisible()
    expect(get.mock.calls[0]?.[1]?.signal?.aborted).toBe(true)
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Refresh usage activity' })
    ).toBeEnabled()
  })

  test('shows the first-use explanation and zero totals when the account has no recorded usage', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: {
          ...activity,
          summary: {
            total_tokens: 0,
            peak_daily_tokens: 0,
            current_streak_days: 0,
            longest_streak_days: 0,
            longest_request_seconds: 0,
          },
          daily_usage: [],
        } satisfies UsageActivity,
      },
    })

    renderActivityCard()

    expect(await screen.findByText('No usage activity yet')).toBeVisible()
    expect(
      screen.getByText('Your activity will appear after you use the API.')
    ).toBeVisible()
    expect(
      screen.getByRole('definition', { name: 'Total tokens: 0' })
    ).toBeVisible()
    expect(
      screen.queryByRole('combobox', { name: 'Activity year' })
    ).not.toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  test.each(['network', 'application'] as const)(
    'recovers from a %s failure when the user retries',
    async (failure) => {
      const user = userEvent.setup()
      const get = vi.spyOn(api, 'get')
      if (failure === 'network') {
        get.mockRejectedValueOnce(new Error('Connection interrupted'))
      } else {
        get.mockResolvedValueOnce({
          data: { success: false, message: 'Unavailable' },
        })
      }
      get.mockResolvedValueOnce({ data: { success: true, data: activity } })

      renderActivityCard()

      const alert = await screen.findByRole('alert')
      expect(alert).toHaveTextContent('Unable to load usage activity')
      expect(
        screen.queryByRole('definition', { name: 'Total tokens: 1,300' })
      ).not.toBeInTheDocument()

      await user.click(within(alert).getByRole('button', { name: 'Retry' }))

      expect(
        await screen.findByRole('definition', { name: 'Total tokens: 1,300' })
      ).toBeVisible()
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    }
  )

  test('reports HTTP 500 as a local query error and allows retry without a global HTTP failure', async () => {
    const user = userEvent.setup()
    const reportedErrors: Error[] = []
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
      queryCache: new QueryCache({
        onError: (error) => reportedErrors.push(error),
      }),
    })
    const serverError = new AxiosError(
      'Internal Server Error',
      AxiosError.ERR_BAD_RESPONSE,
      undefined,
      undefined,
      {
        status: 500,
        statusText: 'Internal Server Error',
        headers: {},
        config: { headers: new AxiosHeaders() },
        data: { success: false, message: 'Activity temporarily unavailable' },
      }
    )
    vi.spyOn(api, 'get')
      .mockRejectedValueOnce(serverError)
      .mockResolvedValueOnce({ data: { success: true, data: activity } })

    renderActivityCard()

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('Unable to load usage activity')
    expect(reportedErrors).toHaveLength(1)
    // Global query handling navigates to /500 for AxiosError responses.
    expect(reportedErrors[0]).not.toBeInstanceOf(AxiosError)

    await user.click(within(alert).getByRole('button', { name: 'Retry' }))

    expect(
      await screen.findByRole('definition', { name: 'Total tokens: 1,300' })
    ).toBeVisible()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  test('keeps existing usage visible and offers retry when a manual refresh fails', async () => {
    const user = userEvent.setup()
    let failRefresh = () => {}
    const refreshPending = new Promise<void>((resolve) => {
      failRefresh = resolve
    })
    vi.spyOn(api, 'get')
      .mockResolvedValueOnce({ data: { success: true, data: activity } })
      .mockImplementationOnce(async () => {
        await refreshPending
        throw new Error('Refresh unavailable')
      })

    renderActivityCard()
    const total = await screen.findByRole('definition', {
      name: 'Total tokens: 1,300',
    })
    const refresh = screen.getByRole('button', {
      name: 'Refresh usage activity',
    })

    await user.click(refresh)

    expect(refresh).toBeDisabled()
    expect(total).toBeVisible()
    failRefresh()

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('Unable to load usage activity')
    expect(within(alert).getByRole('button', { name: 'Retry' })).toBeEnabled()
    expect(total).toBeVisible()
    expect(refresh).toBeEnabled()
  })

  test('allows selecting a previous activity year without requesting the same history again', async () => {
    const user = userEvent.setup()
    const get = vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: activity },
    })

    renderActivityCard()
    const year = await screen.findByRole('combobox', { name: 'Activity year' })
    expect(year).toHaveValue('2026')
    expect(
      within(year)
        .getAllByRole('option')
        .map((option) => option.textContent)
    ).toEqual(['2026', '2025'])
    expect(
      screen.getByRole('status', { name: 'Selected chart data' })
    ).toHaveTextContent('Oct 12, 2026: 600 tokens')

    await user.selectOptions(year, '2025')

    expect(year).toHaveValue('2025')
    expect(
      screen.getByRole('status', { name: 'Selected chart data' })
    ).toHaveTextContent('Dec 31, 2025: 300 tokens')
    expect(
      screen.getByRole('button', { name: 'December 31, 2025: 300 tokens' })
    ).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'October 12, 2026: 600 tokens' })
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole('definition', { name: 'Total tokens: 1,300' })
    ).toBeVisible()
    expect(get).toHaveBeenCalledTimes(1)
  })

  test('clears the previous account history while the next account loads and resets its selected year', async () => {
    const user = userEvent.setup()
    let completeSecondAccount = () => {}
    const secondAccountPending = new Promise<void>((resolve) => {
      completeSecondAccount = resolve
    })
    vi.spyOn(api, 'get')
      .mockResolvedValueOnce({ data: { success: true, data: activity } })
      .mockImplementationOnce(async () => {
        await secondAccountPending
        return {
          data: {
            success: true,
            data: {
              ...activity,
              summary: { ...activity.summary, total_tokens: 10001 },
              daily_usage: [
                { date: '2025-01-01', tokens: 1 },
                { date: '2026-10-12', tokens: 10000 },
              ],
            } satisfies UsageActivity,
          },
        }
      })

    renderActivityCard()
    await user.selectOptions(
      await screen.findByRole('combobox', { name: 'Activity year' }),
      '2025'
    )
    expect(
      screen.getByRole('definition', { name: 'Total tokens: 1,300' })
    ).toBeVisible()

    act(() => {
      useAuthStore
        .getState()
        .auth.setUser({ id: 2, username: 'second-user', role: 1 })
    })

    expect(screen.getByRole('status')).toHaveTextContent(
      'Loading usage activity'
    )
    expect(
      screen.queryByRole('definition', { name: 'Total tokens: 1,300' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('combobox', { name: 'Activity year' })
    ).not.toBeInTheDocument()

    completeSecondAccount()

    expect(
      await screen.findByRole('definition', { name: 'Total tokens: 10,001' })
    ).toBeVisible()
    expect(screen.getByRole('combobox', { name: 'Activity year' })).toHaveValue(
      '2026'
    )
    expect(
      screen.queryByRole('definition', { name: 'Total tokens: 1,300' })
    ).not.toBeInTheDocument()
  })

  test('hides usage and does not request account activity when signed out', () => {
    useAuthStore.getState().auth.setUser(null)
    const get = vi.spyOn(api, 'get')

    const view = renderActivityCard()

    expect(view.container).toBeEmptyDOMElement()
    expect(get).not.toHaveBeenCalled()
  })

  test('uses card width for statistic columns and wraps large totals and long streak details', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: {
          ...activity,
          summary: {
            ...activity.summary,
            total_tokens: 9007199254740991,
            longest_streak_days: 36500,
          },
        } satisfies UsageActivity,
      },
    })

    renderActivityCard()

    const total = await screen.findByRole('definition', {
      name: 'Total tokens: 9,007,199,254,740,991',
    })
    const statistics = total.closest('dl')
    expect(statistics).toHaveClass('grid-cols-2', '@xl/usage:grid-cols-4')
    expect(statistics).not.toHaveClass('sm:grid-cols-4')
    expect(statistics?.closest('[class~="@container/usage"]')).not.toBeNull()
    expect(total).toHaveClass('[overflow-wrap:anywhere]')
    expect(total).toHaveAttribute('title', '9,007,199,254,740,991')
    expect(screen.getByText('Longest streak: 36,500 days')).toHaveClass(
      '[overflow-wrap:anywhere]'
    )
  })

  test('formats token totals and renders the chart with the internal zhCN locale', async () => {
    const i18n = await createLocaleI18n('zhCN')
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: activity },
    })

    render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <UsageActivityCard />
        </QueryClientProvider>
      </I18nextProvider>
    )

    const total = await screen.findByRole('definition', {
      name: `${i18n.t('Total tokens')}: 1,300`,
    })
    expect(total).toBeVisible()
    expect(total).toHaveAttribute('title', '1,300')
    expect(
      screen.getByRole('combobox', { name: i18n.t('Activity year') })
    ).toHaveValue('2026')
    expect(
      screen.getByRole('slider', { name: i18n.t('Explore chart data') })
    ).toHaveAttribute(
      'aria-valuetext',
      i18n.t('{{date}}: {{tokens}} tokens', {
        date: '2026年10月12日',
        tokens: '600',
      })
    )
  })
})
