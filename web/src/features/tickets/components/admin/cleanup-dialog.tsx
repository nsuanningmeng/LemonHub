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
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'

import { cleanupAttachments } from '../../api'
import type { CleanupResult } from '../../types'

interface CleanupDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function CleanupDialog({ open, onOpenChange }: CleanupDialogProps) {
  const { t } = useTranslation()
  const [orphanHours, setOrphanHours] = useState('72')
  const [closedDays, setClosedDays] = useState('30')
  const [purgeClosed, setPurgeClosed] = useState(false)
  const [running, setRunning] = useState(false)
  const [result, setResult] = useState<CleanupResult | null>(null)

  const handleRun = async () => {
    const orphan = Number.parseInt(orphanHours, 10)
    const closed = Number.parseInt(closedDays, 10)
    setRunning(true)
    setResult(null)
    try {
      const res = await cleanupAttachments({
        orphan_hours: Number.isFinite(orphan) ? orphan : undefined,
        closed_days: Number.isFinite(closed) ? closed : undefined,
        purge_closed_tickets: purgeClosed,
      })
      if (res.success && res.data) {
        setResult(res.data)
        toast.success(t('Cleanup completed'))
      } else {
        toast.error(res.message || t('Cleanup failed'))
      }
    } finally {
      setRunning(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('Cleanup attachments')}</DialogTitle>
          <DialogDescription>
            {t(
              'Delete orphaned uploads and attachments from long-closed tickets.'
            )}
          </DialogDescription>
        </DialogHeader>

        <div className='space-y-4'>
          <div className='space-y-1.5'>
            <Label>{t('Orphan upload age (hours)')}</Label>
            <Input
              type='number'
              min={0}
              value={orphanHours}
              onChange={(event) => setOrphanHours(event.target.value)}
            />
          </div>
          <div className='space-y-1.5'>
            <Label>{t('Closed ticket age (days)')}</Label>
            <Input
              type='number'
              min={0}
              value={closedDays}
              onChange={(event) => setClosedDays(event.target.value)}
            />
          </div>
          <div className='flex items-center justify-between gap-3'>
            <div className='space-y-0.5'>
              <Label>{t('Also delete the closed tickets')}</Label>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Permanently remove the closed tickets and their messages, not just attachments.'
                )}
              </p>
            </div>
            <Switch checked={purgeClosed} onCheckedChange={setPurgeClosed} />
          </div>

          {result && (
            <div className='bg-muted/40 space-y-1 rounded-lg border p-3 text-sm'>
              <div className='flex justify-between'>
                <span className='text-muted-foreground'>
                  {t('Deleted rows')}
                </span>
                <span className='tabular-nums'>{result.deleted_rows}</span>
              </div>
              <div className='flex justify-between'>
                <span className='text-muted-foreground'>
                  {t('Deleted files')}
                </span>
                <span className='tabular-nums'>{result.deleted_files}</span>
              </div>
              <div className='flex justify-between'>
                <span className='text-muted-foreground'>
                  {t('Failed files')}
                </span>
                <span className='tabular-nums'>{result.failed_files}</span>
              </div>
              {result.deleted_tickets !== undefined && (
                <div className='flex justify-between'>
                  <span className='text-muted-foreground'>
                    {t('Deleted tickets')}
                  </span>
                  <span className='tabular-nums'>{result.deleted_tickets}</span>
                </div>
              )}
              {result.deleted_messages !== undefined && (
                <div className='flex justify-between'>
                  <span className='text-muted-foreground'>
                    {t('Deleted messages')}
                  </span>
                  <span className='tabular-nums'>{result.deleted_messages}</span>
                </div>
              )}
              {result.errors && result.errors.length > 0 && (
                <ul className='text-destructive mt-1 list-disc space-y-0.5 pl-4 text-xs'>
                  {result.errors.map((error) => (
                    <li key={error}>{error}</li>
                  ))}
                </ul>
              )}
            </div>
          )}
        </div>

        <DialogFooter>
          <Button
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={running}
          >
            {t('Close')}
          </Button>
          <Button onClick={handleRun} disabled={running}>
            {running && <Spinner className='size-3.5' />}
            {t('Run cleanup')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
