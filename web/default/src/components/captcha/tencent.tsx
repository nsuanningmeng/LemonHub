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
import { useRef, useState } from 'react'
import i18next from 'i18next'
import { useTranslation } from 'react-i18next'
import { CheckCircle2, ShieldCheck } from 'lucide-react'
import { Button } from '@/components/ui/button'
import type { CaptchaChannelProps } from './types'

interface TencentCaptchaResult {
  ret: number
  ticket?: string
  randstr?: string
  errorCode?: number
}

declare global {
  interface Window {
    TencentCaptcha?: new (
      container: HTMLElement,
      appId: string,
      callback: (res: TencentCaptchaResult) => void,
      options: Record<string, unknown>
    ) => { show: () => void }
  }
}

interface TencentProps extends CaptchaChannelProps {
  appId: string
}

const TENCENT_SCRIPT_ID = 'tencent-captcha-global'
const TENCENT_SCRIPT_SRC = 'https://ca.turing.captcha.qcloud.com/TJNCaptcha-global.js'

function loadTencentScript(): Promise<void> {
  if (window.TencentCaptcha) return Promise.resolve()
  return new Promise((resolve, reject) => {
    const existing = document.getElementById(
      TENCENT_SCRIPT_ID
    ) as HTMLScriptElement | null
    if (existing) {
      existing.addEventListener('load', () => resolve())
      existing.addEventListener('error', () => reject(new Error('load error')))
      return
    }
    const s = document.createElement('script')
    s.id = TENCENT_SCRIPT_ID
    s.src = TENCENT_SCRIPT_SRC
    s.async = true
    s.onload = () => resolve()
    s.onerror = () => reject(new Error('load error'))
    document.head.appendChild(s)
  })
}

// Tencent Cloud Captcha (international edition). The widget is popup-based:
// the user clicks the button, completes the challenge in the popup, and the
// resulting ticket/randstr pair becomes the verification token.
export function TencentCaptchaWidget({
  appId,
  onVerify,
  onExpire,
  className,
}: TencentProps) {
  const { t } = useTranslation()
  const containerRef = useRef<HTMLDivElement | null>(null)
  const [verified, setVerified] = useState(false)
  const [opening, setOpening] = useState(false)

  const openCaptcha = async () => {
    if (!containerRef.current) return
    setOpening(true)
    try {
      await loadTencentScript()
      if (!window.TencentCaptcha) throw new Error('script unavailable')
      const captcha = new window.TencentCaptcha(
        containerRef.current,
        appId,
        (res) => {
          // Disaster-recovery tickets ("trerror_" prefix, errorCode set) are
          // client-generated and always rejected by the backend, so treat
          // them as a failed attempt instead of a token.
          if (
            res.ret === 0 &&
            res.ticket &&
            res.randstr &&
            !res.errorCode &&
            !res.ticket.startsWith('trerror')
          ) {
            setVerified(true)
            onVerify(JSON.stringify({ ticket: res.ticket, randstr: res.randstr }))
          } else if (res.ret !== 2) {
            setVerified(false)
            onExpire?.()
          }
        },
        { userLanguage: i18next.language || 'zh-cn' }
      )
      captcha.show()
    } catch {
      onExpire?.()
    } finally {
      setOpening(false)
    }
  }

  return (
    <div className={className}>
      <div ref={containerRef} />
      <Button
        type='button'
        variant='outline'
        className='w-full'
        disabled={opening}
        onClick={openCaptcha}
      >
        {verified ? (
          <>
            <CheckCircle2 className='me-2 h-4 w-4 text-emerald-500' />
            {t('Verification passed')}
          </>
        ) : (
          <>
            <ShieldCheck className='me-2 h-4 w-4' />
            {t('Click to complete human verification')}
          </>
        )}
      </Button>
    </div>
  )
}
