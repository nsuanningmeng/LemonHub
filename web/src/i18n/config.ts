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
import i18n, { type BackendModule } from 'i18next'
import LanguageDetector from 'i18next-browser-languagedetector'
import { initReactI18next } from 'react-i18next'

import { convertDetectedLanguage, toIntlLocale } from './languages'
import en from './locales/en.json'

type LocaleModule = { default: { translation: Record<string, string> } }

const loadLocale: Partial<Record<string, () => Promise<LocaleModule>>> = {
  fr: () => import('./locales/fr.json'),
  ja: () => import('./locales/ja.json'),
  ru: () => import('./locales/ru.json'),
  vi: () => import('./locales/vi.json'),
  zhCN: () => import('./locales/zh.json'),
  zhTW: () => import('./locales/zh-TW.json'),
}

const bundledLocales: BackendModule = {
  type: 'backend',
  init() {},
  async read(language, namespace) {
    const load = loadLocale[language]
    if (!load || namespace !== 'translation') return {}
    const locale = await load()
    return locale.default.translation
  },
}

// Keep assistive technology and browser typography aligned with the displayed language.
i18n.on('languageChanged', (language) => {
  document.documentElement.lang = toIntlLocale(language) ?? 'en'
})

export const i18nReady = i18n
  .use(bundledLocales)
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources: { en },
    partialBundledLanguages: true,
    initAsync: false,
    fallbackLng: 'en',
    supportedLngs: ['en', 'zhCN', 'fr', 'ru', 'ja', 'vi', 'zhTW'],
    load: 'currentOnly',
    nsSeparator: false, // Allow literal colons in keys (e.g., URLs, labels)
    debug: import.meta.env.DEV,
    interpolation: {
      escapeValue: false, // not needed for react as it escapes by default
    },
    detection: {
      order: ['localStorage', 'navigator'],
      caches: ['localStorage'],
      // Browsers report `zh-CN`/`zh-TW`/`zh`; map them onto our `zhCN`/`zhTW`
      // codes (non-Chinese codes pass through for normal supportedLngs matching).
      convertDetectedLanguage,
    },
  })

export default i18n
