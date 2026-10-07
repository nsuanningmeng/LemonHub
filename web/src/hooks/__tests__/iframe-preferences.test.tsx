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
import { act, fireEvent, render, screen } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { ThemeProvider, useTheme } from '@/context/theme-provider'
import { createLocaleI18n } from '@/i18n/__tests__/helpers'

import { useIframePreferences } from '../use-iframe-preferences'

function Frame(props: { src: string; sandbox: string }) {
  const { iframeRef, syncPreferences } = useIframePreferences(props.src)
  const { setTheme } = useTheme()
  return (
    <>
      <iframe
        ref={iframeRef}
        src={props.src}
        sandbox={props.sandbox}
        title='custom page'
        onLoad={syncPreferences}
      />
      <button type='button' onClick={() => setTheme('dark')}>
        Dark theme
      </button>
    </>
  )
}

const sandboxDescriptor = Object.getOwnPropertyDescriptor(
  HTMLIFrameElement.prototype,
  'sandbox'
)
beforeEach(() => {
  vi.useFakeTimers()
  localStorage.clear()
  // jsdom does not implement iframe.sandbox; emulate only this browser DOM API.
  Object.defineProperty(HTMLIFrameElement.prototype, 'sandbox', {
    configurable: true,
    get(this: HTMLIFrameElement) {
      const tokens = document.createElement('div').classList
      tokens.value = this.getAttribute('sandbox') ?? ''
      return tokens
    },
  })
})
afterEach(() => {
  vi.useRealTimers()
  localStorage.clear()
  if (sandboxDescriptor) {
    Object.defineProperty(
      HTMLIFrameElement.prototype,
      'sandbox',
      sandboxDescriptor
    )
  } else {
    Reflect.deleteProperty(HTMLIFrameElement.prototype, 'sandbox')
  }
})

function frameWindow(frame: HTMLIFrameElement): Window {
  const child = frame.contentWindow
  if (!child) throw new Error('The fixture iframe must have a child window')
  return child
}

test('bounded retries deliver to a legacy listener registered after two seconds', async () => {
  const i18n = await createLocaleI18n()
  const result = render(
    <I18nextProvider i18n={i18n}>
      <ThemeProvider defaultTheme='light'>
        <Frame src='https://embed.example/page' sandbox='allow-scripts' />
      </ThemeProvider>
    </I18nextProvider>
  )
  const frame = screen.getByTitle<HTMLIFrameElement>('custom page')
  let listening = false
  const received: unknown[] = []
  const post = vi
    .spyOn(frameWindow(frame), 'postMessage')
    .mockImplementation((message) => {
      if (listening) received.push(message)
    })
  fireEvent.load(frame)
  act(() => {
    vi.advanceTimersByTime(2000)
  })
  expect(received).toEqual([])
  listening = true
  act(() => {
    vi.advanceTimersByTime(500)
  })
  expect(received).toEqual([{ themeMode: 'light' }, { lang: 'en' }])
  act(() => {
    vi.advanceTimersByTime(10000)
  })
  const sent = post.mock.calls.length
  act(() => {
    vi.advanceTimersByTime(10000)
  })
  expect(post).toHaveBeenCalledTimes(sent)
  for (const call of post.mock.calls) {
    expect(call[1]).toBe('*')
  }
  result.unmount()
})

test.each([
  ['allow-scripts', 'null', '*'],
  [
    'allow-scripts allow-same-origin',
    'https://embed.example',
    'https://embed.example',
  ],
])(
  'ready is restricted to this frame and its actual origin contract: %s',
  async (sandbox, origin, targetOrigin) => {
    const i18n = await createLocaleI18n()
    const result = render(
      <I18nextProvider i18n={i18n}>
        <Frame src='https://embed.example/page' sandbox={sandbox} />
      </I18nextProvider>
    )
    const frame = screen.getByTitle<HTMLIFrameElement>('custom page')
    const child = frameWindow(frame)
    const post = vi
      .spyOn(child, 'postMessage')
      .mockImplementation(() => undefined)
    fireEvent.load(frame)
    const initial = post.mock.calls.length
    for (const event of [
      new MessageEvent('message', {
        source: window,
        origin,
        data: 'newapi:custom-home:ready',
      }),
      new MessageEvent('message', {
        source: child,
        origin: 'https://wrong.example',
        data: 'newapi:custom-home:ready',
      }),
      new MessageEvent('message', {
        source: child,
        origin,
        data: 'unrelated:ready',
      }),
    ]) {
      act(() => {
        window.dispatchEvent(event)
      })
    }
    expect(post).toHaveBeenCalledTimes(initial)
    act(() => {
      window.dispatchEvent(
        new MessageEvent('message', {
          source: child,
          origin,
          data: { type: 'newapi:custom-page:ready' },
        })
      )
    })
    expect(post.mock.calls.slice(initial)).toEqual([
      [{ themeMode: 'light' }, targetOrigin],
      [{ lang: 'en' }, targetOrigin],
    ])
    act(() => {
      window.dispatchEvent(
        new MessageEvent('message', {
          source: child,
          origin,
          data: 'newapi:custom-home:ready',
        })
      )
      vi.advanceTimersByTime(10000)
    })
    expect(post).toHaveBeenCalledTimes(initial + 2)
    result.unmount()
  }
)

test('preference changes replace stale retries and unmount removes sends and ready handling', async () => {
  const i18n = await createLocaleI18n()
  const result = render(
    <I18nextProvider i18n={i18n}>
      <ThemeProvider defaultTheme='light'>
        <Frame src='https://embed.example/page' sandbox='allow-scripts' />
      </ThemeProvider>
    </I18nextProvider>
  )
  const frame = screen.getByTitle<HTMLIFrameElement>('custom page')
  const child = frameWindow(frame)
  const post = vi
    .spyOn(child, 'postMessage')
    .mockImplementation(() => undefined)
  fireEvent.load(frame)
  await act(async () => {
    await i18n.changeLanguage('zhCN')
  })
  fireEvent.click(screen.getByRole('button', { name: 'Dark theme' }))
  post.mockClear()
  act(() => {
    vi.advanceTimersByTime(5000)
  })
  expect(post.mock.calls.map(([message]) => message)).toEqual(
    Array.from({ length: 5 }, () => [
      { themeMode: 'dark' },
      { lang: 'zhCN' },
    ]).flat()
  )
  result.unmount()
  const sent = post.mock.calls.length
  act(() => {
    window.dispatchEvent(
      new MessageEvent('message', {
        source: child,
        origin: 'null',
        data: 'newapi:custom-home:ready',
      })
    )
    vi.advanceTimersByTime(10000)
  })
  expect(post).toHaveBeenCalledTimes(sent)
})
