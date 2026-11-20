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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { GatewayAtlas } from '../components/gateway-atlas'

afterEach(() => vi.unstubAllGlobals())

test('a reader requesting reduced motion gets the complete static globe and the copyable endpoint', async () => {
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
    matches: query.includes('prefers-reduced-motion'),
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }))
  vi.stubGlobal('IntersectionObserver', class {})
  const user = userEvent.setup()
  const { container } = render(
    <GatewayAtlas baseUrl='https://gateway.test/v1' />
  )
  expect(container.querySelector('pre')?.textContent?.split('\n')).toHaveLength(
    30
  )
  expect(
    screen.queryByRole('button', { name: /animation/i })
  ).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Copy URL' }))
  expect(await navigator.clipboard.readText()).toBe('https://gateway.test/v1')
})

test('a device saving data shows a still globe without a playback control', () => {
  const previous = Object.getOwnPropertyDescriptor(navigator, 'connection')
  Object.defineProperty(navigator, 'connection', {
    configurable: true,
    value: { saveData: true },
  })
  vi.stubGlobal('IntersectionObserver', class {})
  try {
    const { container } = render(
      <GatewayAtlas baseUrl='https://gateway.test/v1' />
    )
    expect(container.querySelector('pre')).toHaveTextContent('@')
    expect(
      screen.queryByRole('button', { name: /animation/i })
    ).not.toBeInTheDocument()
  } finally {
    if (previous) Object.defineProperty(navigator, 'connection', previous)
    else Reflect.deleteProperty(navigator, 'connection')
  }
})
