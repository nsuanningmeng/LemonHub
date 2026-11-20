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
import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import {
  mapStatusDataToConfig,
  useSystemConfig,
} from '@/hooks/use-system-config'
import { DEFAULT_LOGO } from '@/lib/constants'
import { useSystemConfigStore } from '@/stores/system-config-store'

const originalState = useSystemConfigStore.getState()

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  document.querySelectorAll('link[rel="icon"]').forEach((link) => link.remove())
  useSystemConfigStore.setState(originalState, true)
  localStorage.clear()
})

test('stock server branding resolves to the workspace identity without changing currency settings', () => {
  const config = mapStatusDataToConfig({
    system_name: 'New API',
    logo: '/logo.png',
    quota_display_type: 'CNY',
    usd_exchange_rate: 7,
  })

  expect(config.systemName).toBe('LemonHub')
  expect(config.logo).toBe(DEFAULT_LOGO)
  expect(config.currency?.quotaDisplayType).toBe('CNY')
  expect(config.currency?.usdExchangeRate).toBe(7)
})

test('administrator branding and custom footer remain intact', () => {
  const config = mapStatusDataToConfig({
    system_name: 'Research Workspace',
    logo: 'https://example.com/custom.svg',
    footer_html: '<p>Research team</p>',
  })

  expect(config.systemName).toBe('Research Workspace')
  expect(config.logo).toBe('https://example.com/custom.svg')
  expect(config.footerHtml).toBe('<p>Research team</p>')
})

test('a cached stock identity is updated before the status request completes', async () => {
  useSystemConfigStore.getState().setConfig({
    systemName: 'New API',
    logo: '/logo.png',
  })
  const { result } = renderHook(() => useSystemConfig())

  await waitFor(() => expect(result.current.systemName).toBe('LemonHub'))
  expect(result.current.logo).toBe(DEFAULT_LOGO)

  useSystemConfigStore.getState().setConfig({
    systemName: 'Custom team',
    logo: '/custom.svg',
  })
  await waitFor(() => expect(result.current.systemName).toBe('Custom team'))
  expect(result.current.logo).toBe('/custom.svg')
})

test('a failed custom logo falls back to the bundled logo and favicon without changing saved branding', async () => {
  const images: HTMLImageElement[] = []
  vi.stubGlobal(
    'Image',
    class {
      constructor() {
        const image = document.createElement('img')
        images.push(image)
        return image
      }
    }
  )
  vi.spyOn(console, 'error').mockImplementation(() => {})
  useSystemConfigStore.getState().setConfig({
    logo: 'https://example.test/missing.svg',
    systemName: 'Research Studio',
  })
  const { result } = renderHook(() => useSystemConfig())
  await waitFor(() => expect(images).toHaveLength(1))
  act(() => images[0].dispatchEvent(new Event('error')))
  await waitFor(() => expect(result.current.logo).toBe(DEFAULT_LOGO))
  expect(result.current.systemName).toBe('Research Studio')
  expect(result.current.logoLoaded).toBe(true)
  expect(useSystemConfigStore.getState().config.logo).toBe(
    'https://example.test/missing.svg'
  )
  expect(document.querySelector('link[rel="icon"]')).toHaveAttribute(
    'href',
    DEFAULT_LOGO
  )
})

test('an already loaded bundled logo replaces a stale favicon after custom branding is cleared', async () => {
  const favicon = document.createElement('link')
  favicon.rel = 'icon'
  favicon.href = 'https://example.test/old.svg'
  document.head.appendChild(favicon)
  useSystemConfigStore.getState().setConfig({ logo: '' })
  renderHook(() => useSystemConfig())
  await waitFor(() =>
    expect(document.querySelector('link[rel="icon"]')).toHaveAttribute(
      'href',
      DEFAULT_LOGO
    )
  )
})
