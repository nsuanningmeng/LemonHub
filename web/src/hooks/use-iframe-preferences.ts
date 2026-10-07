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
import { useCallback, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { useTheme } from '@/context/theme-provider'
import { isHttpUrl } from '@/lib/content-format'

const RETRY_DELAYS = [100, 300, 1000, 2500, 5000]
const READY_MESSAGES = new Set([
  'newapi:custom-home:ready',
  'newapi:custom-page:ready',
])

/**
 * Custom pages receive the legacy separate { themeMode } and { lang } messages.
 * A cooperating page may send either ready string above (or { type: string })
 * after installing its listener. Non-cooperating pages get a bounded retry window.
 * Sandboxed opaque pages necessarily use targetOrigin '*'; only public display
 * preferences are sent. No user, authentication or API-key data belongs here.
 */
export function useIframePreferences(src: string | null) {
  const iframeRef = useRef<HTMLIFrameElement>(null)
  const timers = useRef<number[]>([])
  const readyHandled = useRef(false)
  const { i18n } = useTranslation()
  const { resolvedTheme } = useTheme()

  const clearTimers = useCallback(() => {
    timers.current.forEach((timer) => window.clearTimeout(timer))
    timers.current = []
  }, [])

  const sendPreferences = useCallback(() => {
    const frame = iframeRef.current
    if (
      !src ||
      !isHttpUrl(src) ||
      !frame ||
      frame.getAttribute('src') !== src
    ) {
      return
    }
    const opaque =
      frame.hasAttribute('sandbox') &&
      !frame.sandbox.contains('allow-same-origin')
    const targetOrigin = opaque ? '*' : new URL(src).origin
    try {
      frame.contentWindow?.postMessage(
        { themeMode: resolvedTheme },
        targetOrigin
      )
      frame.contentWindow?.postMessage({ lang: i18n.language }, targetOrigin)
    } catch {
      // Navigation/removal can invalidate access to the current child window.
    }
  }, [src, resolvedTheme, i18n.language])

  const syncPreferences = useCallback(() => {
    clearTimers()
    readyHandled.current = false
    if (!src || !isHttpUrl(src)) return
    sendPreferences()
    timers.current = RETRY_DELAYS.map((delay) =>
      window.setTimeout(sendPreferences, delay)
    )
  }, [src, clearTimers, sendPreferences])

  useEffect(() => {
    if (!src || !isHttpUrl(src)) return
    const receiveReady = (event: MessageEvent<unknown>) => {
      const frame = iframeRef.current
      if (
        !frame ||
        frame.getAttribute('src') !== src ||
        event.source !== frame.contentWindow ||
        readyHandled.current
      ) {
        return
      }
      const opaque =
        frame.hasAttribute('sandbox') &&
        !frame.sandbox.contains('allow-same-origin')
      const expectedOrigin = opaque ? 'null' : new URL(src).origin
      if (event.origin !== expectedOrigin) return
      const data = event.data
      let type: unknown = data
      if (typeof data === 'object' && data !== null) {
        type = (data as Record<string, unknown>).type
      }
      if (typeof type !== 'string' || !READY_MESSAGES.has(type)) return
      readyHandled.current = true
      clearTimers()
      sendPreferences()
    }
    window.addEventListener('message', receiveReady)
    syncPreferences()
    return () => {
      clearTimers()
      window.removeEventListener('message', receiveReady)
    }
  }, [src, clearTimers, sendPreferences, syncPreferences])

  return { iframeRef, syncPreferences }
}
