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
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { Loader2, MessageCircle } from 'lucide-react'
import { useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button, buttonVariants } from '@/components/ui/button'
import { useActiveChatKey } from '@/features/chat/hooks/use-active-chat-key'
import { useChatPresets } from '@/features/chat/hooks/use-chat-presets'
import { resolveChatUrl } from '@/features/chat/lib/chat-links'
import { isSidebarModuleEnabled } from '@/lib/nav-modules'

export const Route = createFileRoute('/_authenticated/chat2link')({
  component: Chat2LinkPage,
})

function Chat2LinkPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { chatPresets, serverAddress, loading, error, refetch } =
    useChatPresets()

  const firstWebPreset = useMemo(
    () => chatPresets.find((p) => p.type === 'web'),
    [chatPresets]
  )

  const { data: activeKey, error: keyError } = useActiveChatKey(
    Boolean(firstWebPreset)
  )

  useEffect(() => {
    if (!firstWebPreset) return

    if (activeKey === undefined && !keyError) return

    if (keyError || !activeKey) {
      const message =
        keyError instanceof Error
          ? keyError.message
          : t('No enabled tokens available')
      toast.error(message)
      navigate({ to: '/keys' })
      return
    }

    const url = resolveChatUrl({
      template: firstWebPreset.url,
      apiKey: activeKey,
      serverAddress,
    })

    if (url) {
      window.location.href = url
    }
  }, [firstWebPreset, activeKey, keyError, serverAddress, navigate, t])

  if (!firstWebPreset && !loading) {
    const playgroundEnabled = isSidebarModuleEnabled('chat', 'playground')
    return (
      <div className='mx-auto flex min-h-80 max-w-md flex-col items-center justify-center gap-4 px-5 text-center'>
        <MessageCircle aria-hidden className='text-muted-foreground size-7' />
        <h1 className='text-xl font-semibold'>
          {error ? t('Unable to open chat') : t('No available Web chat links')}
        </h1>
        <div className='flex flex-wrap justify-center gap-2'>
          {error && (
            <Button onClick={() => void refetch()}>{t('Retry')}</Button>
          )}
          <Link
            to={playgroundEnabled ? '/playground' : '/dashboard'}
            className={buttonVariants({
              variant: error ? 'outline' : 'default',
            })}
          >
            {playgroundEnabled ? t('Playground') : t('Return to dashboard')}
          </Link>
        </div>
      </div>
    )
  }

  return (
    <div
      className='flex min-h-80 flex-col items-center justify-center gap-3'
      role='status'
    >
      <Loader2
        aria-hidden
        className='text-muted-foreground size-7 animate-spin'
      />
      <p className='text-muted-foreground text-sm'>
        {loading && !firstWebPreset
          ? t('Loading configuration')
          : t('Redirecting to chat page...')}
      </p>
    </div>
  )
}
