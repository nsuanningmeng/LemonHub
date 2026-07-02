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
import { useEffect, useRef } from 'react'
import i18next from 'i18next'
import 'altcha'
import 'altcha/i18n/zh-cn'
import 'altcha/i18n/fr-fr'
import 'altcha/i18n/ja'
import 'altcha/i18n/ru'
import 'altcha/i18n/vi'
import type { CaptchaChannelProps } from './types'

declare module 'react' {
  namespace JSX {
    interface IntrinsicElements {
      'altcha-widget': React.DetailedHTMLProps<
        React.HTMLAttributes<HTMLElement>,
        HTMLElement
      > & {
        challengeurl?: string
        auto?: string
        language?: string
        hidelogo?: boolean
        hidefooter?: boolean
      }
    }
  }
}

function altchaLanguage(): string {
  const lang = i18next.language || 'zh'
  if (lang.startsWith('zh')) return 'zh-cn'
  if (lang.startsWith('fr')) return 'fr-fr'
  return lang.slice(0, 2)
}

// Self-hosted ALTCHA proof-of-work widget. The challenge comes from this
// deployment's own backend, so no third-party domain is involved and the
// widget behaves identically on every network.
export function Altcha({ onVerify, onExpire, className }: CaptchaChannelProps) {
  const ref = useRef<HTMLElement | null>(null)
  const onVerifyRef = useRef(onVerify)
  const onExpireRef = useRef(onExpire)
  onVerifyRef.current = onVerify
  onExpireRef.current = onExpire

  useEffect(() => {
    const el = ref.current
    if (!el) return
    const handleStateChange = (ev: Event) => {
      const detail = (ev as CustomEvent).detail as
        | { state?: string; payload?: string }
        | undefined
      if (detail?.state === 'verified' && detail.payload) {
        onVerifyRef.current(detail.payload)
      } else if (detail?.state === 'expired' || detail?.state === 'error') {
        onExpireRef.current?.()
      }
    }
    el.addEventListener('statechange', handleStateChange)
    return () => el.removeEventListener('statechange', handleStateChange)
  }, [])

  return (
    <altcha-widget
      ref={ref}
      challengeurl='/api/captcha/altcha'
      language={altchaLanguage()}
      hidelogo
      hidefooter
      className={className}
    />
  )
}
