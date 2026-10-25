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
import { afterEach, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'

import { ViewLogsDialog } from '../view-logs-dialog'

let client: QueryClient

afterEach(() => client?.clear())

test('translates deployment stream and known status labels without changing API values or original log output', async () => {
  const user = userEvent.setup()
  const locale = createInstance()
  await locale.use(initReactI18next).init({
    lng: 'zh',
    fallbackLng: 'en',
    resources: { en, zh },
  })
  const rawLog = 'running stdout upstream/model-x E104 request-123'
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/deployments/42/containers') {
      return {
        data: {
          success: true,
          data: {
            containers: [
              { container_id: 'container-1', status: 'running' },
              { container_id: 'container-2', status: 'VENDOR_WARMING' },
            ],
          },
        },
      }
    }
    if (url === '/api/deployments/42/logs') {
      return { data: { success: true, data: rawLog } }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={locale}>
        <ViewLogsDialog open onOpenChange={() => undefined} deploymentId={42} />
      </I18nextProvider>
    </QueryClientProvider>
  )
  await screen.findByText(rawLog)
  const [containers, streams] = screen.getAllByRole('combobox')
  expect(containers).toHaveTextContent(`container-1 (${locale.t('Running')})`)
  expect(streams).toHaveTextContent(locale.t('Standard output (stdout)'))
  await user.click(containers)
  expect(
    screen.getByRole('option', { name: 'container-2 (VENDOR_WARMING)' })
  ).toBeVisible()
  await user.keyboard('{Escape}')
  await user.click(streams)
  await user.click(
    screen.getByRole('option', { name: locale.t('All streams') })
  )
  await waitFor(() =>
    expect(get).toHaveBeenCalledWith('/api/deployments/42/logs', {
      params: { container_id: 'container-1', stream: 'all', limit: 500 },
    })
  )
  await act(async () => {
    await locale.changeLanguage('en')
  })
  expect(streams).toHaveTextContent(locale.t('All streams'))
  expect(containers).toHaveTextContent(`container-1 (${locale.t('Running')})`)
  expect(screen.getByText(rawLog)).toBeVisible()
})
