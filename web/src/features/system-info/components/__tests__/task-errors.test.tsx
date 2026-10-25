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
import { act, render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'

import { SystemTasksPanel } from '../system-tasks-panel'

let client: QueryClient

afterEach(() => {
  client?.clear()
  vi.restoreAllMocks()
})

test('localizes known task failures on language changes while retaining raw diagnostics and unknown errors', async () => {
  const locale = createInstance()
  await locale.use(initReactI18next).init({
    lng: 'zh',
    fallbackLng: 'en',
    resources: { en, zh },
  })
  const task = {
    id: 1,
    task_id: 'task-1',
    type: 'log_cleanup',
    status: 'failed',
    updated_at: 100,
    created_at: 90,
    error: 'task lease expired',
    locked_by: 'worker-a',
  }
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: [
        task,
        {
          ...task,
          id: 2,
          task_id: 'task-2',
          error: 'upstream/model-x: E104 request-123',
        },
      ],
    },
  })
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={locale}>
        <SystemTasksPanel />
      </I18nextProvider>
    </QueryClientProvider>
  )
  expect(
    await screen.findByText(
      locale.t('The task stopped because its execution lease expired.')
    )
  ).toHaveAttribute('title', 'task lease expired')
  expect(screen.getByText('upstream/model-x: E104 request-123')).toBeVisible()
  await act(async () => {
    await locale.changeLanguage('en')
  })
  expect(
    screen.getByText(
      locale.t('The task stopped because its execution lease expired.')
    )
  ).toBeVisible()
  expect(screen.getByText('upstream/model-x: E104 request-123')).toBeVisible()
})

test('updates a cached task-list loading error when the display language changes', async () => {
  const locale = createInstance()
  await locale.use(initReactI18next).init({
    lng: 'en',
    fallbackLng: 'en',
    resources: { en, zh },
  })
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: false } })
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={locale}>
        <SystemTasksPanel />
      </I18nextProvider>
    </QueryClientProvider>
  )
  const englishError = locale.t('We could not load system tasks.')
  await screen.findAllByText(englishError)
  await act(async () => {
    await locale.changeLanguage('zh')
  })
  expect(
    screen.getAllByText(locale.t('We could not load system tasks.')).length
  ).toBeGreaterThan(0)
  expect(screen.queryByText(englishError)).not.toBeInTheDocument()
})

test('localizes automatic reward tasks on language changes while retaining the raw task type', async () => {
  const locale = createInstance()
  await locale.use(initReactI18next).init({
    lng: 'zh',
    fallbackLng: 'en',
    resources: { en, zh },
  })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: [
        {
          id: 9,
          task_id: 'reward-job-9',
          type: 'reward_distribution',
          status: 'running',
          updated_at: 100,
          created_at: 90,
          error: '',
          locked_by: 'worker-a',
        },
      ],
    },
  })
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={locale}>
        <SystemTasksPanel />
      </I18nextProvider>
    </QueryClientProvider>
  )

  expect(await screen.findByText('奖励自动发放')).toBeVisible()
  expect(screen.getByText('reward_distribution')).toBeVisible()
  await act(async () => {
    await locale.changeLanguage('en')
  })

  expect(screen.getByText('Automatic reward distribution')).toBeVisible()
  expect(screen.getByText('reward_distribution')).toBeVisible()
  expect(screen.queryByText('奖励自动发放')).not.toBeInTheDocument()
})
