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
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { GatewayAtlas } from '../components/gateway-atlas'

beforeEach(() => {
  vi.stubGlobal('IntersectionObserver', class {})
  vi.stubGlobal('OffscreenCanvas', class {})
  vi.stubGlobal('Worker', class {})
  vi.spyOn(navigator, 'hardwareConcurrency', 'get').mockReturnValue(8)
})

afterEach(() => vi.unstubAllGlobals())

test('playback controls stay hidden until the worker has presented a frame', () => {
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({
    canvas: document.createElement('canvas'),
    transferFromImageBitmap: vi.fn(),
  })
  vi.stubGlobal(
    'IntersectionObserver',
    class {
      observe() {}
      disconnect() {}
    }
  )

  const { container } = render(<GatewayAtlas />)
  expect(container.querySelector('img')).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'Pause animation' })
  ).not.toBeInTheDocument()
})

test('a reader requesting reduced motion sees the static panorama without the playback control', () => {
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
  const { container } = render(<GatewayAtlas />)
  expect(container.querySelector('img')).toHaveAttribute('width', '1400')
  expect(container.querySelector('img')).toHaveAttribute('height', '700')
  expect(container.querySelector('canvas')).not.toHaveAttribute('data-ready')
  expect(
    screen.queryByRole('button', { name: /animation/i })
  ).not.toBeInTheDocument()
})

test('a device saving data shows a still landscape without a playback control', () => {
  const previous = Object.getOwnPropertyDescriptor(navigator, 'connection')
  Object.defineProperty(navigator, 'connection', {
    configurable: true,
    value: { saveData: true },
  })
  try {
    const { container } = render(<GatewayAtlas />)
    expect(container.querySelector('img')).toHaveAttribute(
      'src',
      expect.stringContaining('earthrise')
    )
    expect(
      screen.queryByRole('button', { name: /animation/i })
    ).not.toBeInTheDocument()
  } finally {
    if (previous) Object.defineProperty(navigator, 'connection', previous)
    else Reflect.deleteProperty(navigator, 'connection')
  }
})
