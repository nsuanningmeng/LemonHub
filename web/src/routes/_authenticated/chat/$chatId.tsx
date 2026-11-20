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
import { Link, createFileRoute, redirect } from '@tanstack/react-router'
import { MessageCircleWarning } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { buttonVariants } from '@/components/ui/button'
import { useActiveChatKey } from '@/features/chat/hooks/use-active-chat-key'
import { useChatPresets } from '@/features/chat/hooks/use-chat-presets'
import {
  chatLinkRequiresApiKey,
  resolveChatUrl,
} from '@/features/chat/lib/chat-links'

export const Route = createFileRoute('/_authenticated/chat/$chatId')({
  loader: async ({ params }) => {
    if (!Number.isInteger(Number(params.chatId))) {
      throw redirect({ to: '/dashboard' })
    }
  },
  component: ChatRouteComponent,
})

function ChatRouteComponent() {
  const { t } = useTranslation()
  const { chatId } = Route.useParams()
  const {
    chatPresets,
    serverAddress,
    loading,
    error: configError,
    refetch,
  } = useChatPresets()
  const preset = useMemo(() => {
    const index = Number(chatId)
    if (!Number.isInteger(index)) return undefined
    return chatPresets.find((candidate) => candidate.id === String(index))
  }, [chatId, chatPresets])

  const isWebLink = preset?.type === 'web'

  const requiresActiveKey = useMemo(() => {
    if (!preset || !isWebLink) return false
    return chatLinkRequiresApiKey(preset.url ?? '')
  }, [isWebLink, preset])

  const {
    data: activeKey,
    isPending,
    isError,
    error,
  } = useActiveChatKey(!loading && Boolean(preset && requiresActiveKey))

  const iframeSrc = useMemo(() => {
    if (!preset || !isWebLink) return ''
    if (requiresActiveKey && !activeKey) return ''
    return resolveChatUrl({
      template: preset.url,
      apiKey: requiresActiveKey ? activeKey : undefined,
      serverAddress,
    })
  }, [activeKey, isWebLink, preset, requiresActiveKey, serverAddress])

  if (loading) {
    return (
      <LoadingState className='min-h-80' message={t('Loading configuration')} />
    )
  }

  const dashboardLink = (
    <Link to='/dashboard' className={buttonVariants({ variant: 'outline' })}>
      {t('Return to dashboard')}
    </Link>
  )

  if (!preset && configError) {
    return (
      <ErrorState
        title={t('Unable to open chat')}
        onRetry={() => void refetch()}
        action={dashboardLink}
      />
    )
  }

  if (!preset) {
    return (
      <div className='flex h-full flex-col items-center justify-center gap-4 p-6 text-center'>
        <MessageCircleWarning className='text-muted-foreground h-12 w-12' />
        <div className='space-y-1'>
          <h2 className='text-lg font-semibold'>
            {t('Chat preset not found')}
          </h2>
          <p className='text-muted-foreground'>
            {t('The requested chat preset does not exist or has been removed.')}
          </p>
        </div>
        {dashboardLink}
      </div>
    )
  }

  if (!isWebLink) {
    return (
      <div className='flex h-full flex-col items-center justify-center gap-4 p-6 text-center'>
        <MessageCircleWarning className='text-muted-foreground h-12 w-12' />
        <div className='space-y-1'>
          <h2 className='text-lg font-semibold'>{t('Use sidebar shortcut')}</h2>
          <p className='text-muted-foreground'>
            {preset.name}{' '}
            {t(
              'opens in an external client. Trigger it from the sidebar or API key actions to launch the configured application.'
            )}
          </p>
        </div>
        {dashboardLink}
      </div>
    )
  }

  if (requiresActiveKey && isPending) {
    return (
      <LoadingState
        className='min-h-80'
        message={t('Preparing your chat link…')}
      />
    )
  }

  if (requiresActiveKey && (isError || !activeKey || !iframeSrc)) {
    const message =
      error instanceof Error
        ? error.message
        : 'Unable to generate chat link. Please check your API keys.'
    return (
      <div className='flex h-full flex-col items-center justify-center p-6'>
        <Alert variant='destructive' className='max-w-xl'>
          <AlertTitle>{t('Unable to open chat')}</AlertTitle>
          <AlertDescription>{message}</AlertDescription>
        </Alert>
      </div>
    )
  }

  if (!requiresActiveKey && !iframeSrc) {
    return (
      <div className='flex h-full flex-col items-center justify-center p-6'>
        <Alert variant='destructive' className='max-w-xl'>
          <AlertTitle>{t('Unable to open chat')}</AlertTitle>
          <AlertDescription>
            {t(
              'Unable to generate chat link. Please contact your administrator.'
            )}
          </AlertDescription>
        </Alert>
      </div>
    )
  }

  // The route parameter selects a numeric index. The URL comes from the
  // administrator's web chat presets. These clients may use their own origin
  // for login popups and storage, and request camera or microphone access.
  return (
    <iframe
      src={iframeSrc}
      key={iframeSrc}
      className='h-full w-full border-0'
      allow='camera; microphone'
      title={preset.name.trim() || t('Chat')}
    />
  )
}
