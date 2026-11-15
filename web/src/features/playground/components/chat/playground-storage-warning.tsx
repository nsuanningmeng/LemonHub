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
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'

import type { Message } from '../../types'

type PlaygroundStorageWarningProps = {
  messages: Message[]
  onRetry: () => void
}

export function PlaygroundStorageWarning(props: PlaygroundStorageWarningProps) {
  const { t } = useTranslation()

  function downloadConversation(): void {
    const blob = new Blob([JSON.stringify(props.messages, null, 2)], {
      type: 'application/json',
    })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = 'playground-conversation.json'
    document.body.appendChild(link)
    link.click()
    link.remove()
    window.setTimeout(() => URL.revokeObjectURL(url), 0)
  }

  return (
    <Alert variant='destructive' className='mx-auto w-full max-w-4xl shrink-0'>
      <AlertDescription className='flex flex-wrap items-center gap-2'>
        <p className='w-full'>
          {t(
            'Conversation could not be saved in this browser. Download a copy before leaving.'
          )}
        </p>
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={downloadConversation}
        >
          {t('Download conversation')}
        </Button>
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={props.onRetry}
        >
          {t('Retry saving')}
        </Button>
      </AlertDescription>
    </Alert>
  )
}
