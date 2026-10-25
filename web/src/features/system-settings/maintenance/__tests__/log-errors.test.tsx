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
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { toast } from 'sonner'
import { afterEach, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'

import { LogSettingsSection } from '../log-settings-section'

let client: QueryClient

afterEach(() => client?.clear())

test('shows a translated cleanup failure and keeps its original text after language changes', async () => {
  const locale = createInstance()
  await locale.use(initReactI18next).init({
    lng: 'zh',
    fallbackLng: 'en',
    resources: { en, zh },
  })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/performance/logs') {
      return {
        data: {
          success: true,
          data: { enabled: false, file_count: 0, total_size: 0 },
        },
      }
    }
    if (url === '/api/system-task/current') {
      return {
        data: {
          success: true,
          data: {
            task_id: 'cleanup-1',
            type: 'log_cleanup',
            status: 'failed',
            error: 'no log rows were deleted',
            state: { total: 5, processed: 0, progress: 0 },
          },
        },
      }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={locale}>
        <LogSettingsSection defaultEnabled />
      </I18nextProvider>
    </QueryClientProvider>
  )
  expect(
    await screen.findByText(
      locale.t('No log entries were deleted. Please try again.')
    )
  ).toHaveAttribute('title', 'no log rows were deleted')
  await act(async () => {
    await locale.changeLanguage('en')
  })
  expect(
    screen.getByText(locale.t('No log entries were deleted. Please try again.'))
  ).toBeVisible()
})

test('translates an exact cleanup endpoint error before showing its toast', async () => {
  const user = userEvent.setup()
  const locale = createInstance()
  await locale.use(initReactI18next).init({
    lng: 'zh',
    fallbackLng: 'en',
    resources: { en, zh },
  })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/performance/logs') {
      return {
        data: {
          success: true,
          data: { enabled: false, file_count: 0, total_size: 0 },
        },
      }
    }
    if (url === '/api/system-task/current') {
      return { data: { success: true, data: null } }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
  vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: false, message: 'target timestamp is required' },
  })
  const showError = vi.spyOn(toast, 'error')
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={locale}>
        <LogSettingsSection defaultEnabled />
      </I18nextProvider>
    </QueryClientProvider>
  )
  await user.click(screen.getByRole('button', { name: locale.t('Clean logs') }))
  await user.click(
    screen.getByRole('button', { name: locale.t('Delete logs') })
  )
  await waitFor(() =>
    expect(showError).toHaveBeenCalledWith(
      locale.t('Select a timestamp before clearing logs.')
    )
  )
})
