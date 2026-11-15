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
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

beforeEach(() => {
  vi.resetModules()
  vi.stubEnv('DEV', false)
  vi.doMock('i18next', async () => {
    const actual = await vi.importActual<typeof import('i18next')>('i18next')
    return { ...actual, default: actual.createInstance() }
  })
  localStorage.clear()
  document.documentElement.lang = 'en'
})

afterEach(() => {
  vi.doUnmock('../locales/fr.json')
  vi.doUnmock('i18next')
  vi.unstubAllEnvs()
  localStorage.clear()
  document.documentElement.lang = 'en'
})

async function loadConfiguredLanguage(language: string) {
  localStorage.setItem('i18nextLng', language)
  const config = await import('../config')
  await config.i18nReady
  return config.default
}

describe('application language loading', () => {
  it('loads the saved Chinese locale and English fallback without other locales', async () => {
    const i18n = await loadConfiguredLanguage('zh-CN')

    expect(i18n.t('Cancel')).toBe('取消')
    expect(document.documentElement).toHaveAttribute('lang', 'zh-CN')
    expect(Object.keys(i18n.store.data).sort()).toEqual(['en', 'zhCN'])
  })

  it('loads a switched locale on demand and updates the document language', async () => {
    const i18n = await loadConfiguredLanguage('en')
    expect(i18n.hasResourceBundle('zhTW', 'translation')).toBe(false)

    await i18n.changeLanguage('zhTW')

    expect(i18n.t('Cancel')).toBe('取消')
    expect(document.documentElement).toHaveAttribute('lang', 'zh-TW')
    expect(localStorage.getItem('i18nextLng')).toBe('zhTW')
    expect(Object.keys(i18n.store.data).sort()).toEqual(['en', 'zhTW'])

    await i18n.changeLanguage('ja')
    expect(document.documentElement).toHaveAttribute('lang', 'ja')
    expect(i18n.t('Cancel')).not.toBe('Cancel')
  })

  it('detects a browser locale when no preference has been saved', async () => {
    vi.spyOn(navigator, 'languages', 'get').mockReturnValue(['zh-TW'])
    const config = await import('../config')
    await config.i18nReady

    expect(config.default.language).toBe('zhTW')
    expect(document.documentElement).toHaveAttribute('lang', 'zh-TW')
  })

  it('restores a cached traditional Chinese preference after reloading', async () => {
    const i18n = await loadConfiguredLanguage('zhTW')

    expect(i18n.language).toBe('zhTW')
    expect(document.documentElement).toHaveAttribute('lang', 'zh-TW')
  })

  it.each(['en', 'fr'])(
    'keeps English available if a locale chunk fails after starting in %s',
    async (initialLanguage) => {
      vi.doMock('../locales/fr.json', () => {
        throw new Error('Locale chunk unavailable')
      })
      const i18n = await loadConfiguredLanguage(initialLanguage)
      await i18n.changeLanguage('fr')

      expect(i18n.t('Cancel')).toBe('Cancel')
      expect(document.documentElement).toHaveAttribute('lang', 'en')

      await i18n.changeLanguage('ja')
      expect(document.documentElement).toHaveAttribute('lang', 'ja')
    }
  )
})
