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
import { act, render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { expect, test } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'

import { usageLogSchema } from '../../../data/schema'
import { DetailsDialog } from '../details-dialog'

const reasons = [
  ['done', 'Completion signal received', '收到完成标记'],
  ['timeout', 'Stream timed out', '流响应超时'],
  ['client_gone', 'Client disconnected', '客户端已断开'],
  ['scanner_error', 'Stream read failed', '流读取失败'],
  ['handler_stop', 'Stopped by response handler', '响应处理器结束流'],
  ['eof', 'Upstream stream ended', '上游数据流结束'],
  ['panic', 'Stream handler panic', '流处理异常'],
  ['ping_fail', 'Keep-alive failed', '保活失败'],
] as const

async function renderStreamDetails(status: string, reason: string) {
  const language = createInstance()
  await language.use(initReactI18next).init({
    lng: 'zh',
    fallbackLng: 'en',
    resources: { en, zh },
  })
  const log = usageLogSchema.parse({
    id: 1,
    user_id: 1,
    created_at: 1,
    type: 2,
    content: '',
    model_name: 'provider/model-error',
    request_id: 'req_timeout_42',
    other: JSON.stringify({
      is_model_mapped: true,
      upstream_model_name: 'upstream/model-42',
      stream_status: {
        status,
        end_reason: reason,
        error_count: 1,
        end_error: 'E429 upstream/model timeout: req_timeout_42',
        errors: ['scanner_error: untouched provider detail'],
      },
    }),
  })
  render(
    <I18nextProvider i18n={language}>
      <DetailsDialog log={log} isAdmin open onOpenChange={() => undefined} />
    </I18nextProvider>
  )
  return language
}

test.each(reasons)(
  'translates the controlled stream end reason %s and retains its diagnostic code',
  async (reason, _english, chinese) => {
    await renderStreamDetails('error', reason)
    expect(screen.getByText(chinese)).toBeVisible()
    expect(screen.getByText(chinese)).toHaveAttribute('title', reason)
  }
)

test('switches stream status and reason language without translating model names or diagnostics', async () => {
  const language = await renderStreamDetails('error', 'timeout')
  expect(screen.getByTitle('error')).toHaveTextContent('错误')
  expect(screen.getByText('流响应超时')).toBeVisible()
  await act(async () => {
    await language.changeLanguage('en')
  })
  expect(screen.getByTitle('error')).toHaveTextContent('Error')
  expect(screen.getByText('Stream timed out')).toBeVisible()
  expect(screen.getByText('provider/model-error')).toBeVisible()
  expect(
    screen.getByText('E429 upstream/model timeout: req_timeout_42')
  ).toBeVisible()
  expect(
    screen.getByText('scanner_error: untouched provider detail')
  ).toBeVisible()
})

test('keeps unrecognized stream status and end reasons unchanged', async () => {
  await renderStreamDetails('partial', 'upstream-custom-reason')
  expect(screen.getByText('partial')).toBeVisible()
  expect(screen.getByText('upstream-custom-reason')).toBeVisible()
})
