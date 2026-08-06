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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2, Plus, RefreshCw, Trash2, Upload } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { api } from '@/lib/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import { SettingsSection } from '../components/settings-section'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { useUpdateOption } from '../hooks/use-update-option'

type SuppressionReason = 'hard_bounce' | 'complaint'

type EmailSuppression = {
  id: number
  email: string
  reason: SuppressionReason
  source: string
  detail: string
  created_at: number
  updated_at: number
}

type SuppressionListData = {
  page: number
  page_size: number
  total: number
  items: EmailSuppression[] | null
}

const PAGE_SIZE = 10

async function listSuppressions(
  page: number,
  keyword: string
): Promise<{ success: boolean; message?: string; data?: SuppressionListData }> {
  const res = await api.get('/api/email-suppression/', {
    params: { p: page, page_size: PAGE_SIZE, keyword },
  })
  return res.data
}

function formatSuppressionTimestamp(ts: number): string {
  if (!ts) return '-'
  const millis = ts < 1e12 ? ts * 1000 : ts
  return new Date(millis).toLocaleString()
}

type EmailSuppressionSectionProps = {
  defaultValues: {
    EmailDeliveryEventToken: string
  }
}

export function EmailSuppressionSection(_props: EmailSuppressionSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const queryClient = useQueryClient()

  // --- delivery-event callback token (write-only option, like SMTP tokens) ---
  const [callbackToken, setCallbackToken] = useState('')

  const onSaveCallbackToken = async () => {
    const trimmed = callbackToken.trim()
    if (!trimmed) {
      toast.info(t('No changes to save'))
      return
    }
    await updateOption.mutateAsync({
      key: 'EmailDeliveryEventToken',
      value: trimmed,
    })
    setCallbackToken('')
  }

  // --- suppression list ---
  const [page, setPage] = useState(1)
  const [keyword, setKeyword] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [addOpen, setAddOpen] = useState(false)
  const [importOpen, setImportOpen] = useState(false)
  const [addEmail, setAddEmail] = useState('')
  const [addReason, setAddReason] = useState<SuppressionReason>('hard_bounce')
  const [importContent, setImportContent] = useState('')

  const suppressionsQuery = useQuery({
    queryKey: ['email-suppressions', page, keyword],
    queryFn: () => listSuppressions(page, keyword),
  })

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ['email-suppressions'] })

  const addMutation = useMutation({
    mutationFn: async () => {
      const res = await api.post('/api/email-suppression/', {
        email: addEmail.trim(),
        reason: addReason,
      })
      return res.data
    },
    onSuccess: (data) => {
      if (!data.success) {
        toast.error(data.message || t('Operation failed'))
        return
      }
      toast.success(t('Address suppressed'))
      setAddOpen(false)
      setAddEmail('')
      invalidate()
    },
  })

  const importMutation = useMutation({
    mutationFn: async () => {
      const res = await api.post('/api/email-suppression/import', {
        content: importContent,
      })
      return res.data
    },
    onSuccess: (data) => {
      if (!data.success) {
        toast.error(data.message || t('Operation failed'))
        return
      }
      toast.success(
        t('Imported {{imported}} addresses, {{failed}} failed', {
          imported: data.data?.imported ?? 0,
          failed: data.data?.failed ?? 0,
        })
      )
      setImportOpen(false)
      setImportContent('')
      invalidate()
    },
  })

  const deleteMutation = useMutation({
    mutationFn: async (id: number) => {
      const res = await api.delete(`/api/email-suppression/${id}`)
      return res.data
    },
    onSuccess: (data) => {
      if (!data.success) {
        toast.error(data.message || t('Operation failed'))
        return
      }
      toast.success(t('Suppression removed'))
      invalidate()
    },
  })

  const listData = suppressionsQuery.data?.data
  const items = listData?.items ?? []
  const total = listData?.total ?? 0
  const hasNext = page * PAGE_SIZE < total

  const REASON_BADGE: Record<SuppressionReason, string> = {
    hard_bounce:
      'bg-destructive/10 text-destructive border-destructive/30',
    complaint:
      'bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/30',
  }

  return (
    <SettingsSection title={t('Email Suppression')}>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Addresses that hard-bounced or drew a spam complaint are never mailed again, protecting your sender reputation.'
        )}
      </p>

      <SettingsPageFormActions
        onSave={onSaveCallbackToken}
        isSaving={updateOption.isPending}
        saveLabel='Save delivery callback settings'
      />

      {/* Delivery-event callback */}
      <Card>
        <CardHeader>
          <CardTitle>{t('Delivery event callback')}</CardTitle>
          <CardDescription>
            {t(
              'Automatically learn bounces and spam complaints pushed by your email provider (e.g. Aliyun DirectMail via EventBridge or MNS HTTP push).'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='flex flex-col gap-3'>
          <div className='grid gap-2'>
            <Label htmlFor='delivery-callback-token'>
              {t('Delivery event callback token')}
            </Label>
            <Input
              id='delivery-callback-token'
              type='password'
              autoComplete='off'
              placeholder={t('Enter new token to update')}
              value={callbackToken}
              onChange={(event) => setCallbackToken(event.target.value)}
            />
            <p className='text-muted-foreground text-sm'>
              {t('Leave blank to keep the existing credential')}
            </p>
          </div>
          <div className='text-muted-foreground text-sm'>
            {t('Configure your provider to POST delivery events to:')}{' '}
            <code className='bg-muted rounded px-1 py-0.5 break-all'>
              {`${window.location.origin}/api/email/delivery-events?key=<token>`}
            </code>
          </div>
        </CardContent>
      </Card>

      {/* Suppression list */}
      <Card>
        <CardHeader>
          <div className='flex flex-wrap items-center justify-between gap-2'>
            <div className='grid gap-1'>
              <CardTitle>{t('Suppression list')}</CardTitle>
              <CardDescription>
                {t('{{total}} suppressed address(es)', { total })}
              </CardDescription>
            </div>
            <div className='flex flex-wrap items-center gap-2'>
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={() => suppressionsQuery.refetch()}
                disabled={suppressionsQuery.isFetching}
              >
                <RefreshCw data-icon='inline-start' />
                <span>{t('Refresh')}</span>
              </Button>
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={() => setImportOpen(true)}
              >
                <Upload data-icon='inline-start' />
                <span>{t('Import list')}</span>
              </Button>
              <Button type='button' size='sm' onClick={() => setAddOpen(true)}>
                <Plus data-icon='inline-start' />
                <span>{t('Add address')}</span>
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent className='flex flex-col gap-3'>
          <form
            className='flex gap-2'
            onSubmit={(event) => {
              event.preventDefault()
              setPage(1)
              setKeyword(searchInput.trim())
            }}
          >
            <Input
              placeholder={t('Search email')}
              value={searchInput}
              onChange={(event) => setSearchInput(event.target.value)}
              className='max-w-xs'
            />
            <Button type='submit' variant='outline'>
              {t('Search')}
            </Button>
          </form>

          {suppressionsQuery.isLoading && (
            <div className='text-muted-foreground flex min-h-24 items-center justify-center text-sm'>
              {t('Loading...')}
            </div>
          )}
          {!suppressionsQuery.isLoading && items.length === 0 && (
            <div className='text-muted-foreground flex min-h-24 items-center justify-center text-sm'>
              {t('No suppressed addresses.')}
            </div>
          )}
          {!suppressionsQuery.isLoading && items.length > 0 && (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Email')}</TableHead>
                  <TableHead>{t('Reason')}</TableHead>
                  <TableHead>{t('Source')}</TableHead>
                  <TableHead>{t('Detail')}</TableHead>
                  <TableHead>{t('Created')}</TableHead>
                  <TableHead className='w-12' />
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((item) => (
                  <TableRow key={item.id}>
                    <TableCell className='max-w-[14rem] truncate font-medium'>
                      {item.email}
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant='outline'
                        className={REASON_BADGE[item.reason] ?? ''}
                      >
                        {item.reason === 'complaint'
                          ? t('Spam complaint')
                          : t('Hard bounce')}
                      </Badge>
                    </TableCell>
                    <TableCell className='text-muted-foreground'>
                      {item.source}
                    </TableCell>
                    <TableCell className='text-muted-foreground max-w-[16rem] truncate'>
                      {item.detail || '-'}
                    </TableCell>
                    <TableCell className='text-muted-foreground'>
                      {formatSuppressionTimestamp(item.created_at)}
                    </TableCell>
                    <TableCell>
                      <Button
                        type='button'
                        variant='ghost'
                        size='icon-sm'
                        aria-label={t('Delete')}
                        onClick={() => deleteMutation.mutate(item.id)}
                        disabled={deleteMutation.isPending}
                      >
                        <Trash2 />
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}

          <div className='flex items-center justify-end gap-2'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={page <= 1}
              onClick={() => setPage((prev) => Math.max(1, prev - 1))}
            >
              {t('Previous')}
            </Button>
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={!hasNext}
              onClick={() => setPage((prev) => prev + 1)}
            >
              {t('Next')}
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Add dialog */}
      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('Add address')}</DialogTitle>
            <DialogDescription>
              {t('The address will never receive email of the selected scope again.')}
            </DialogDescription>
          </DialogHeader>
          <div className='grid gap-4'>
            <div className='grid gap-2'>
              <Label htmlFor='suppression-add-email'>{t('Email')}</Label>
              <Input
                id='suppression-add-email'
                type='email'
                autoComplete='off'
                placeholder='user@example.com'
                value={addEmail}
                onChange={(event) => setAddEmail(event.target.value)}
              />
            </div>
            <div className='grid gap-2'>
              <Label>{t('Reason')}</Label>
              <Select
                items={[
                  { value: 'hard_bounce', label: t('Hard bounce') },
                  { value: 'complaint', label: t('Spam complaint') },
                ]}
                value={addReason}
                onValueChange={(value) =>
                  setAddReason(value as SuppressionReason)
                }
              >
                <SelectTrigger className='w-full'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    <SelectItem value='hard_bounce'>
                      {t('Hard bounce')}
                    </SelectItem>
                    <SelectItem value='complaint'>
                      {t('Spam complaint')}
                    </SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
              <p className='text-muted-foreground text-sm'>
                {t(
                  'Hard bounce blocks all email; spam complaint blocks marketing email only.'
                )}
              </p>
            </div>
          </div>
          <DialogFooter>
            <DialogClose render={<Button variant='outline' />}>
              {t('Cancel')}
            </DialogClose>
            <Button
              type='button'
              onClick={() => addMutation.mutate()}
              disabled={addMutation.isPending || !addEmail.trim()}
            >
              {addMutation.isPending ? (
                <Loader2 data-icon='inline-start' className='animate-spin' />
              ) : (
                <Plus data-icon='inline-start' />
              )}
              <span>{t('Add')}</span>
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Import dialog */}
      <Dialog open={importOpen} onOpenChange={setImportOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('Import invalid addresses')}</DialogTitle>
            <DialogDescription>
              {t(
                'Paste the invalid-address export from your email provider console. Every email address found will be suppressed as a hard bounce.'
              )}
            </DialogDescription>
          </DialogHeader>
          <Textarea
            className='min-h-40'
            placeholder={'a@example.com\nb@example.com\n...'}
            value={importContent}
            onChange={(event) => setImportContent(event.target.value)}
          />
          <DialogFooter>
            <DialogClose render={<Button variant='outline' />}>
              {t('Cancel')}
            </DialogClose>
            <Button
              type='button'
              onClick={() => importMutation.mutate()}
              disabled={importMutation.isPending || !importContent.trim()}
            >
              {importMutation.isPending ? (
                <Loader2 data-icon='inline-start' className='animate-spin' />
              ) : (
                <Upload data-icon='inline-start' />
              )}
              <span>{t('Import')}</span>
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SettingsSection>
  )
}
