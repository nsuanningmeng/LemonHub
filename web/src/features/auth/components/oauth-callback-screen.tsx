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
import { Loader2, Send, Shield, UserRound, type LucideIcon } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { IconGithub, IconLinuxDo, IconWeChat } from '@/assets/brand-icons'

import { AuthLayout } from '../auth-layout'

type OAuthCallbackScreenProps = {
  provider: string
  mode: 'login' | 'bind'
}

type ProviderMeta = {
  label: string
  Icon: LucideIcon | ((props: { className?: string }) => React.JSX.Element)
}

const providerDictionary: Record<string, ProviderMeta> = {
  github: {
    label: 'GitHub',
    Icon: (props: { className?: string }) => (
      <IconGithub className={props.className} focusable='false' />
    ),
  },
  oidc: { label: 'OIDC', Icon: Shield },
  linuxdo: {
    label: 'LinuxDO',
    Icon: (props: { className?: string }) => (
      <IconLinuxDo className={props.className} focusable='false' />
    ),
  },
  telegram: { label: 'Telegram', Icon: Send },
  wechat: {
    label: 'WeChat',
    Icon: (props: { className?: string }) => (
      <IconWeChat className={props.className} focusable='false' />
    ),
  },
}

export function OAuthCallbackScreen({
  provider,
  mode,
}: OAuthCallbackScreenProps) {
  const { t } = useTranslation()
  const { label, Icon } = useMemo(() => {
    const normalized = provider?.toLowerCase() ?? ''
    return (
      providerDictionary[normalized] || {
        label: 'account',
        Icon: UserRound,
      }
    )
  }, [provider])

  const providerLabel = t(label)
  const isBindMode = mode === 'bind'

  const headline = isBindMode
    ? t('Binding your {{provider}} account', { provider: providerLabel })
    : t('Signing you in with {{provider}}', { provider: providerLabel })

  const secondaryNote = isBindMode
    ? t(
        'You can close this tab once the binding completes or a success message appears in the original window.'
      )
    : t(
        "You'll be redirected automatically. You can return to the previous page if nothing happens after a few seconds."
      )

  return (
    <AuthLayout title={headline} description={secondaryNote}>
      <div
        role='status'
        aria-live='polite'
        className='flex min-h-24 items-center gap-4'
      >
        <Icon className='text-primary size-8 shrink-0' />
        <span className='min-w-0 flex-1 font-medium'>
          {t('Processing OAuth response...')}
        </span>
        <Loader2
          aria-hidden='true'
          className='text-muted-foreground size-5 shrink-0 animate-spin motion-reduce:animate-none'
        />
      </div>
    </AuthLayout>
  )
}
