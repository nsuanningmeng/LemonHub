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
import * as z from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Eye, Loader2, Pencil, RefreshCw, Send } from 'lucide-react'
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
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Markdown } from '@/components/ui/markdown'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'
import { safeNumberFieldProps } from '../utils/numeric-field'

// ============================================================================
// Campaign data layer (reuses the shared `api` axios instance)
// ============================================================================

type CampaignSource = 'manual' | 'announcement'
type CampaignStatus = 'pending' | 'sending' | 'completed' | 'failed'

type EmailCampaign = {
  id: number
  subject: string
  content: string
  target_group: string
  target_status: number
  source: CampaignSource
  status: CampaignStatus
  total_count: number
  sent_count: number
  fail_count: number
  created_at: number
  finished_at: number
}

type CampaignListData = {
  page: number
  page_size: number
  total: number
  items: EmailCampaign[]
}

type CreateCampaignRequest = {
  subject: string
  content: string
  target_group: string
  target_status: number
}

async function listEmailCampaigns(): Promise<{
  success: boolean
  message?: string
  data?: CampaignListData
}> {
  const res = await api.get('/api/email-campaign/', {
    params: { p: 1, page_size: 10 },
  })
  return res.data
}

async function createEmailCampaign(body: CreateCampaignRequest): Promise<{
  success: boolean
  message?: string
  data?: EmailCampaign
}> {
  const res = await api.post('/api/email-campaign/', body)
  return res.data
}

// ============================================================================
// Settings form (announcement toggle + send rate)
// ============================================================================

const promotionSettingsSchema = z.object({
  announcementEmailEnabled: z.boolean(),
  ratePerMinute: z.coerce.number().int().min(1),
})

type PromotionSettingsInput = z.input<typeof promotionSettingsSchema>
type PromotionSettingsValues = z.output<typeof promotionSettingsSchema>

type EmailPromotionSectionProps = {
  defaultValues: {
    announcementEmailEnabled: boolean
    ratePerMinute: number
  }
}

// ============================================================================
// Compose form
// ============================================================================

const composeSchema = z.object({
  subject: z.string().trim().min(1, 'Subject is required'),
  content: z.string().trim().min(1, 'Email content is required'),
  targetGroup: z.string(),
  targetStatus: z.enum(['0', '1']),
})

type ComposeValues = z.infer<typeof composeSchema>

const STATUS_BADGE: Record<
  CampaignStatus,
  { label: string; className: string }
> = {
  pending: {
    label: 'Pending',
    className: 'bg-muted text-muted-foreground border-transparent',
  },
  sending: {
    label: 'Sending',
    className:
      'bg-blue-500/10 text-blue-600 dark:text-blue-400 border-blue-500/30',
  },
  completed: {
    label: 'Completed',
    className:
      'bg-green-500/10 text-green-600 dark:text-green-400 border-green-500/30',
  },
  failed: {
    label: 'Failed',
    className:
      'bg-destructive/10 text-destructive border-destructive/30',
  },
}

function formatTimestamp(ts: number): string {
  if (!ts) return '-'
  const millis = ts < 1e12 ? ts * 1000 : ts
  return new Date(millis).toLocaleString()
}

export function EmailPromotionSection({
  defaultValues,
}: EmailPromotionSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const queryClient = useQueryClient()

  const [previewMode, setPreviewMode] = useState(false)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [pendingPayload, setPendingPayload] =
    useState<CreateCampaignRequest | null>(null)

  // --- settings form ---
  const settingsForm = useForm<
    PromotionSettingsInput,
    unknown,
    PromotionSettingsValues
  >({
    resolver: zodResolver(promotionSettingsSchema),
    defaultValues: {
      announcementEmailEnabled: defaultValues.announcementEmailEnabled,
      ratePerMinute: defaultValues.ratePerMinute,
    },
  })

  useResetForm(settingsForm, {
    announcementEmailEnabled: defaultValues.announcementEmailEnabled,
    ratePerMinute: defaultValues.ratePerMinute,
  })

  const onSaveSettings = async (values: PromotionSettingsValues) => {
    const updates: Array<{ key: string; value: string | boolean | number }> = []

    if (
      values.announcementEmailEnabled !== defaultValues.announcementEmailEnabled
    ) {
      updates.push({
        key: 'email_promotion_setting.announcement_email_enabled',
        value: values.announcementEmailEnabled,
      })
    }

    if (values.ratePerMinute !== defaultValues.ratePerMinute) {
      updates.push({
        key: 'email_promotion_setting.rate_per_minute',
        value: values.ratePerMinute,
      })
    }

    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }
  }

  // --- compose form ---
  const composeForm = useForm<ComposeValues>({
    resolver: zodResolver(composeSchema),
    defaultValues: {
      subject: '',
      content: '',
      targetGroup: '',
      // Default to enabled users only — mailing disabled accounts must be an
      // explicit admin choice, mirroring the backend default.
      targetStatus: '1',
    },
  })

  const composeContent = composeForm.watch('content')

  const createMutation = useMutation({
    mutationFn: createEmailCampaign,
    onSuccess: (data) => {
      if (!data.success) return
      toast.success(t('Bulk email queued successfully'))
      composeForm.reset({
        subject: '',
        content: '',
        targetGroup: '',
        targetStatus: '1',
      })
      setPreviewMode(false)
      setConfirmOpen(false)
      setPendingPayload(null)
      queryClient.invalidateQueries({ queryKey: ['email-campaigns'] })
    },
  })

  const onComposeSubmit = (values: ComposeValues) => {
    setPendingPayload({
      subject: values.subject.trim(),
      content: values.content,
      target_group: values.targetGroup.trim(),
      target_status: Number(values.targetStatus),
    })
    setConfirmOpen(true)
  }

  // --- campaign history ---
  const campaignsQuery = useQuery({
    queryKey: ['email-campaigns'],
    queryFn: listEmailCampaigns,
    refetchInterval: (query) => {
      const items = query.state.data?.data?.items ?? []
      return items.some(
        (item) => item.status === 'sending' || item.status === 'pending'
      )
        ? 5000
        : false
    },
  })

  const campaigns = [...(campaignsQuery.data?.data?.items ?? [])].sort(
    (a, b) => b.id - a.id
  )

  return (
    <SettingsSection title={t('Email Promotion')}>
      <Form {...settingsForm}>
        <SettingsForm onSubmit={settingsForm.handleSubmit(onSaveSettings)}>
          <SettingsPageFormActions
            onSave={settingsForm.handleSubmit(onSaveSettings)}
            isSaving={updateOption.isPending}
            saveLabel='Save email promotion settings'
          />
          <FormField
            control={settingsForm.control}
            name='announcementEmailEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Email new announcements to users')}</FormLabel>
                  <FormDescription>
                    {t(
                      'When a new announcement is published, automatically email it to your users.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />
          <FormField
            control={settingsForm.control}
            name='ratePerMinute'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Send rate (emails per minute)')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={1}
                    step={1}
                    {...safeNumberFieldProps(field)}
                  />
                </FormControl>
                <FormDescription>
                  {t('Throttle outgoing bulk email to avoid provider limits.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>

      {/* Compose bulk email */}
      <Card>
        <CardHeader>
          <CardTitle>{t('Compose bulk email')}</CardTitle>
          <CardDescription>
            {t('Send a one-off email to a group of users.')}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Form {...composeForm}>
            <form
              onSubmit={composeForm.handleSubmit(onComposeSubmit)}
              className='flex flex-col gap-4'
            >
              <FormField
                control={composeForm.control}
                name='subject'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Subject')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('Email subject')}
                        {...field}
                        onChange={(event) => field.onChange(event.target.value)}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={composeForm.control}
                name='content'
                render={({ field }) => (
                  <FormItem>
                    <div className='flex items-center justify-between gap-2'>
                      <FormLabel>{t('Content (Markdown)')}</FormLabel>
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        onClick={() => setPreviewMode((prev) => !prev)}
                      >
                        {previewMode ? (
                          <>
                            <Pencil data-icon='inline-start' />
                            <span>{t('Edit')}</span>
                          </>
                        ) : (
                          <>
                            <Eye data-icon='inline-start' />
                            <span>{t('Preview')}</span>
                          </>
                        )}
                      </Button>
                    </div>
                    {previewMode ? (
                      <div className='min-h-32 rounded-lg border px-3 py-2'>
                        {composeContent.trim() ? (
                          <Markdown>{composeContent}</Markdown>
                        ) : (
                          <p className='text-muted-foreground text-sm'>
                            {t('Nothing to preview yet.')}
                          </p>
                        )}
                      </div>
                    ) : (
                      <FormControl>
                        <Textarea
                          className='min-h-32'
                          placeholder={t('Write your email in Markdown...')}
                          {...field}
                          onChange={(event) =>
                            field.onChange(event.target.value)
                          }
                        />
                      </FormControl>
                    )}
                    <FormMessage />
                  </FormItem>
                )}
              />

              <div className='grid gap-4 md:grid-cols-2 items-start'>
                <FormField
                  control={composeForm.control}
                  name='targetGroup'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Target group')}</FormLabel>
                      <FormControl>
                        <Input
                          placeholder={t('Leave empty for all groups')}
                          {...field}
                          onChange={(event) =>
                            field.onChange(event.target.value)
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        {t('Only users in this group will receive the email.')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={composeForm.control}
                  name='targetStatus'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Target status')}</FormLabel>
                      <Select
                        items={[
                          { value: '0', label: t('All users') },
                          { value: '1', label: t('Enabled users only') },
                        ]}
                        value={field.value}
                        onValueChange={field.onChange}
                      >
                        <FormControl>
                          <SelectTrigger className='w-full'>
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent alignItemWithTrigger={false}>
                          <SelectGroup>
                            <SelectItem value='0'>{t('All users')}</SelectItem>
                            <SelectItem value='1'>
                              {t('Enabled users only')}
                            </SelectItem>
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <div className='flex justify-end'>
                <Button type='submit' disabled={createMutation.isPending}>
                  <Send data-icon='inline-start' />
                  <span>{t('Send')}</span>
                </Button>
              </div>
            </form>
          </Form>
        </CardContent>
      </Card>

      {/* Campaign history */}
      <Card>
        <CardHeader>
          <div className='flex items-center justify-between gap-2'>
            <div className='grid gap-1'>
              <CardTitle>{t('Campaign history')}</CardTitle>
              <CardDescription>
                {t('Recent bulk email campaigns and their delivery progress.')}
              </CardDescription>
            </div>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => campaignsQuery.refetch()}
              disabled={campaignsQuery.isFetching}
            >
              <RefreshCw data-icon='inline-start' />
              <span>{t('Refresh')}</span>
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {campaignsQuery.isLoading && (
            <div className='text-muted-foreground flex min-h-24 items-center justify-center text-sm'>
              {t('Loading campaigns...')}
            </div>
          )}
          {!campaignsQuery.isLoading && campaigns.length === 0 && (
            <div className='text-muted-foreground flex min-h-24 items-center justify-center text-sm'>
              {t('No campaigns yet.')}
            </div>
          )}
          {!campaignsQuery.isLoading && campaigns.length > 0 && (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Subject')}</TableHead>
                  <TableHead>{t('Source')}</TableHead>
                  <TableHead>{t('Status')}</TableHead>
                  <TableHead>{t('Progress')}</TableHead>
                  <TableHead>{t('Created')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {campaigns.map((campaign) => {
                  const badge = STATUS_BADGE[campaign.status]
                  return (
                    <TableRow key={campaign.id}>
                      <TableCell className='max-w-[16rem] truncate font-medium'>
                        {campaign.subject}
                      </TableCell>
                      <TableCell>
                        {campaign.source === 'announcement'
                          ? t('Announcement')
                          : t('Manual')}
                      </TableCell>
                      <TableCell>
                        <Badge variant='outline' className={badge.className}>
                          {t(badge.label)}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <span className='tabular-nums'>
                          {campaign.sent_count}/{campaign.total_count}
                        </span>
                        {campaign.fail_count > 0 ? (
                          <span className='text-destructive ml-2'>
                            {t('failed:')} {campaign.fail_count}
                          </span>
                        ) : null}
                      </TableCell>
                      <TableCell className='text-muted-foreground'>
                        {formatTimestamp(campaign.created_at)}
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      {/* Send confirmation */}
      <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('Send bulk email?')}</DialogTitle>
            <DialogDescription>
              {t(
                'Send this email to the selected users? This action cannot be undone.'
              )}
            </DialogDescription>
          </DialogHeader>
          {pendingPayload && (
            <div className='text-muted-foreground text-sm'>
              <span className='text-foreground font-medium'>
                {t('Audience')}:
              </span>{' '}
              {pendingPayload.target_group || t('All groups')}
              {' · '}
              {pendingPayload.target_status === 1
                ? t('Enabled users only')
                : t('All users')}
            </div>
          )}
          <DialogFooter>
            <DialogClose
              render={<Button variant='outline' />}
              disabled={createMutation.isPending}
            >
              {t('Cancel')}
            </DialogClose>
            <Button
              type='button'
              onClick={() => {
                if (pendingPayload) createMutation.mutate(pendingPayload)
              }}
              disabled={createMutation.isPending}
            >
              {createMutation.isPending ? (
                <Loader2 data-icon='inline-start' className='animate-spin' />
              ) : (
                <Send data-icon='inline-start' />
              )}
              <span>{t('Send')}</span>
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SettingsSection>
  )
}
