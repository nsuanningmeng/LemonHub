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
import { Copy } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ScrollArea } from '@/components/ui/scroll-area'

type GeneratedKeysDialogProps = {
  keys: string[] | null
  onClose: () => void
}

export function GeneratedKeysDialog({ keys, onClose }: GeneratedKeysDialogProps) {
  const { t } = useTranslation()
  const { copyToClipboard } = useCopyToClipboard()

  const open = keys !== null && keys.length > 0

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>{t('Generated Redemption Codes')}</DialogTitle>
          <DialogDescription>
            {t(
              'Copy and store these codes now. They will not be shown again in full.'
            )}
          </DialogDescription>
        </DialogHeader>

        <ScrollArea className='max-h-[320px] rounded-md border'>
          <div className='flex flex-col gap-1 p-3'>
            {(keys ?? []).map((key) => (
              <code
                key={key}
                className='bg-muted/50 rounded px-2 py-1 font-mono text-xs break-all'
              >
                {key}
              </code>
            ))}
          </div>
        </ScrollArea>

        <DialogFooter>
          <Button
            variant='outline'
            onClick={() => copyToClipboard((keys ?? []).join('\n'))}
          >
            <Copy className='h-4 w-4' />
            {t('Copy All')}
          </Button>
          <Button onClick={onClose}>{t('Done')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
