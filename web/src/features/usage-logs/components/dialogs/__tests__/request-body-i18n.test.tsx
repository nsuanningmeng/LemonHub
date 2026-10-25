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
import { act, cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'

import { RequestBodyDialog } from '../request-body-dialog'

const animationsDescriptor = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'getAnimations'
)

beforeEach(() => {
  Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
})

afterEach(() => {
  cleanup()
  if (animationsDescriptor) {
    Object.defineProperty(
      HTMLElement.prototype,
      'getAnimations',
      animationsDescriptor
    )
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, 'getAnimations')
  }
})

async function renderRequestBody() {
  const language = createInstance()
  await language.use(initReactI18next).init({
    lng: 'zh',
    fallbackLng: 'en',
    resources: { en, zh },
  })
  render(
    <I18nextProvider i18n={language}>
      <RequestBodyDialog
        requestId='request_42'
        open
        onOpenChange={() => undefined}
      />
    </I18nextProvider>
  )
  return language
}

test.each([
  ['request body not recorded for this request', '该请求未记录请求体'],
  ['request_id is required', '缺少请求 ID'],
  ['无权查看该用户的请求体', '无权查看该用户的请求体'],
])('translates the known request-body error %s', async (message, chinese) => {
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: false, message } })
  await renderRequestBody()
  expect(await screen.findByRole('alert')).toHaveTextContent(chinese)
})

test('switches the displayed request-body error language without refetching and keeps the original error available', async () => {
  const user = userEvent.setup()
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: false,
      message: 'request body not recorded for this request',
    },
  })
  const language = await renderRequestBody()
  expect(await screen.findByRole('alert')).toHaveTextContent(
    '该请求未记录请求体'
  )
  await user.click(screen.getByText('原始错误'))
  expect(
    screen.getByText('request body not recorded for this request')
  ).toBeVisible()
  await act(async () => {
    await language.changeLanguage('en')
  })
  expect(screen.getByRole('alert')).toHaveTextContent(
    'Request body not recorded'
  )
  expect(get).toHaveBeenCalledTimes(1)
})

test('keeps an unknown backend diagnostic verbatim instead of translating fragments', async () => {
  const message =
    'E502 provider/model_error: request body not recorded for this request (req_42)'
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: false, message } })
  await renderRequestBody()
  expect(await screen.findByRole('alert')).toHaveTextContent(message)
  expect(screen.queryByText('原始错误')).not.toBeInTheDocument()
})

test('localizes a failed request while keeping the request ID unchanged', async () => {
  const get = vi
    .spyOn(api, 'get')
    .mockRejectedValue(new Error('network unavailable'))
  await renderRequestBody()
  expect(await screen.findByRole('alert')).toHaveTextContent('加载请求体失败')
  expect(get).toHaveBeenCalledWith('/api/log/request_body/request_42')
})
