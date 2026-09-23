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
import i18next from 'i18next'
import { expect, test } from 'vitest'

import zh from '@/i18n/locales/zh.json'

import { getPaymentErrorMessage } from '../payment'

test('explains the wallet balance limit when the server rejects a top-up', () => {
  expect(
    getPaymentErrorMessage({
      message: 'error',
      data: 'top-up quota limit exceeded',
    })
  ).toBe(
    'This top-up exceeds the wallet balance limit. Reduce the amount or contact the administrator.'
  )
})

test('preserves other backend payment errors', () => {
  expect(
    getPaymentErrorMessage({ message: 'error', data: '充值金额过低' })
  ).toBe('充值金额过低')
})

test('shows the wallet balance limit in Chinese for a Chinese session', async () => {
  const previousLanguage = i18next.language
  i18next.addResourceBundle('zh', 'translation', zh.translation)
  try {
    await i18next.changeLanguage('zh')
    expect(
      getPaymentErrorMessage({
        message: 'error',
        data: 'top-up quota limit exceeded',
      })
    ).toBe('本次充值会超出钱包余额上限，请减少充值金额或联系管理员。')
  } finally {
    await i18next.changeLanguage(previousLanguage)
    i18next.removeResourceBundle('zh', 'translation')
  }
})
