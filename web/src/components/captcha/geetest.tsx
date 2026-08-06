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
import { useEffect, useId, useRef } from 'react'
import i18next from 'i18next'
import type { CaptchaChannelProps } from './types'

interface GeetestInstance {
  appendTo: (selector: string) => void
  onSuccess: (cb: () => void) => void
  onError: (cb: () => void) => void
  getValidate: () => Record<string, unknown> | undefined
  destroy?: () => void
}

declare global {
  interface Window {
    initGeetest4?: (
      config: Record<string, unknown>,
      handler: (captcha: GeetestInstance) => void
    ) => void
  }
}

interface GeetestProps extends CaptchaChannelProps {
  captchaId: string
}

// GeeTest v4 widget languages use ISO 639-2 style codes.
function geetestLanguage(): string {
  const lang = i18next.language || 'zh'
  if (lang.startsWith('zh')) return 'zho'
  if (lang.startsWith('ja')) return 'jpn'
  if (lang.startsWith('ru')) return 'rus'
  if (lang.startsWith('vi')) return 'vie'
  if (lang.startsWith('fr')) return 'fra'
  return 'eng'
}

export function Geetest({
  captchaId,
  onVerify,
  onExpire,
  className,
}: GeetestProps) {
  const containerId = useId().replace(/[^a-zA-Z0-9_-]/g, '')
  const domId = `geetest-${containerId}`
  const onVerifyRef = useRef(onVerify)
  const onExpireRef = useRef(onExpire)
  onVerifyRef.current = onVerify
  onExpireRef.current = onExpire

  useEffect(() => {
    let cancelled = false
    let instance: GeetestInstance | undefined

    const render = () => {
      if (cancelled || !window.initGeetest4) return
      window.initGeetest4(
        {
          captchaId,
          product: 'float',
          language: geetestLanguage(),
        },
        (captcha) => {
          if (cancelled) {
            captcha.destroy?.()
            return
          }
          instance = captcha
          captcha.onSuccess(() => {
            const validate = captcha.getValidate()
            if (validate) onVerifyRef.current(JSON.stringify(validate))
          })
          captcha.onError(() => onExpireRef.current?.())
          captcha.appendTo(`#${domId}`)
        }
      )
    }

    if (window.initGeetest4) {
      render()
    } else {
      const scriptId = 'geetest-gt4'
      const existing = document.getElementById(
        scriptId
      ) as HTMLScriptElement | null
      if (existing) {
        existing.addEventListener('load', render)
      } else {
        const s = document.createElement('script')
        s.id = scriptId
        s.src = 'https://static.geetest.com/v4/gt4.js'
        s.async = true
        s.onload = () => render()
        document.head.appendChild(s)
      }
    }
    return () => {
      cancelled = true
      instance?.destroy?.()
    }
  }, [captchaId, domId])

  return <div id={domId} className={className} />
}
