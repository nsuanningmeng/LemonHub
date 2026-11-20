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
import { expect, test } from 'vitest'

import i18n, { i18nReady } from '../config'

test('updates the document language with standard tags when switching Chinese scripts', async () => {
  await i18nReady
  const previous = i18n.language
  try {
    await i18n.changeLanguage('zhTW')
    expect(document.documentElement.lang).toBe('zh-TW')
    expect(i18n.t('One gateway.')).toBe('一個閘道。')
    await i18n.changeLanguage('zhCN')
    expect(document.documentElement.lang).toBe('zh-CN')
    expect(i18n.t('One gateway.')).toBe('一个网关。')
  } finally {
    await i18n.changeLanguage(previous)
  }
})
