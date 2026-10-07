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
import { createInstance } from 'i18next'
import { initReactI18next } from 'react-i18next'

import en from '../locales/en.json'
import fr from '../locales/fr.json'
import ja from '../locales/ja.json'
import ru from '../locales/ru.json'
import vi from '../locales/vi.json'
import zhTW from '../locales/zh-TW.json'
import zhCN from '../locales/zh.json'

export const localeResources = { en, fr, ja, ru, vi, zhCN, zhTW }
export type TestLocale = keyof typeof localeResources

export async function createLocaleI18n(language: TestLocale = 'en') {
  const instance = createInstance()
  await instance.use(initReactI18next).init({
    lng: language,
    fallbackLng: 'en',
    resources: localeResources,
    nsSeparator: false,
    interpolation: { escapeValue: false },
  })
  return instance
}
