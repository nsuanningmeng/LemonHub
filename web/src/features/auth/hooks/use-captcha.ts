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
import { useState } from 'react'
import i18next from 'i18next'
import { toast } from 'sonner'
import { useStatus } from '@/hooks/use-status'

/**
 * Hook for managing human verification (bot protection) across the
 * configured channel: Turnstile, GeeTest, ALTCHA, or Tencent Captcha.
 * The token is whatever the active channel's widget produced; the backend
 * middleware verifies it for the matching provider.
 */
export function useCaptcha() {
  const { status } = useStatus()
  const [captchaToken, setCaptchaToken] = useState('')

  const captchaProvider = status?.captcha_provider || 'turnstile'
  const providerReady = (() => {
    switch (captchaProvider) {
      case 'geetest':
        return !!status?.geetest_captcha_id
      case 'altcha':
        // The ALTCHA channel needs no public key; the backend serves the
        // challenge itself.
        return true
      case 'tencent':
        return !!status?.tencent_captcha_app_id
      default:
        return !!status?.turnstile_site_key
    }
  })()

  const isCaptchaEnabled = !!status?.turnstile_check && providerReady

  /**
   * Validate that a captcha token is present when the check is required
   */
  const validateCaptcha = (): boolean => {
    if (isCaptchaEnabled && !captchaToken) {
      if (captchaProvider === 'tencent') {
        // Tencent is popup-based: nothing happens until the user clicks.
        toast.info(i18next.t('Please complete the human verification first'))
      } else {
        toast.info(
          i18next.t('Please wait a moment, human check is initializing...')
        )
      }
      return false
    }
    return true
  }

  return {
    isCaptchaEnabled,
    captchaProvider,
    captchaToken,
    setCaptchaToken,
    validateCaptcha,
  }
}
