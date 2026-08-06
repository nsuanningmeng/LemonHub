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
import { useStatus } from '@/hooks/use-status'
import { Altcha } from './altcha'
import { Geetest } from './geetest'
import { TencentCaptchaWidget } from './tencent'
import { Turnstile } from './turnstile'
import type { CaptchaChannelProps } from './types'

// CaptchaWidget renders whichever human-verification channel the admin has
// configured. The produced token is provider-specific but always travels to
// the backend as one opaque string.
export function CaptchaWidget({
  onVerify,
  onExpire,
  className,
}: CaptchaChannelProps) {
  const { status } = useStatus()
  switch (status?.captcha_provider) {
    case 'geetest':
      return (
        <Geetest
          captchaId={status?.geetest_captcha_id || ''}
          onVerify={onVerify}
          onExpire={onExpire}
          className={className}
        />
      )
    case 'altcha':
      return (
        <Altcha onVerify={onVerify} onExpire={onExpire} className={className} />
      )
    case 'tencent':
      return (
        <TencentCaptchaWidget
          appId={status?.tencent_captcha_app_id || ''}
          onVerify={onVerify}
          onExpire={onExpire}
          className={className}
        />
      )
    default:
      return (
        <Turnstile
          siteKey={status?.turnstile_site_key || ''}
          onVerify={onVerify}
          onExpire={onExpire}
          className={className}
        />
      )
  }
}
