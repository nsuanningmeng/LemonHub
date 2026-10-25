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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Gift, Users } from 'lucide-react'
import { nanoid } from 'nanoid'
import { useMemo, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Form } from '@/components/ui/form'
import { formatQuotaWithCurrency } from '@/lib/currency'
import { handleServerError } from '@/lib/handle-server-error'

import { createRewardCampaign, previewRewardRecipients } from './api'
import { CampaignFields } from './campaign-fields'
import {
  campaignFormToInput,
  defaultCampaignFormValues,
  getCampaignFormSchema,
  type CampaignFormValues,
} from './lib/campaign-form'
import type { RewardCampaignInput } from './types'

type CampaignPreview = { input: RewardCampaignInput; count: number }

export function CampaignForm(props: { onCreated?: () => void }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const schema = useMemo(() => getCampaignFormSchema(t), [t])
  const form = useForm<CampaignFormValues>({
    resolver: zodResolver(schema),
    defaultValues: defaultCampaignFormValues(),
  })
  const [preview, setPreview] = useState<CampaignPreview | null>(null)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const revision = useRef(0)
  const requestId = useRef<string | null>(null)
  const issuing = useRef(false)
  const previewMutation = useMutation({
    mutationFn: previewRewardRecipients,
    onError: handleServerError,
  })
  const createMutation = useMutation({
    mutationFn: createRewardCampaign,
    onError: (error) => {
      handleServerError(error)
      if (error.message === 'reward audience changed since preview') {
        setConfirmOpen(false)
        setPreview(null)
        toast.error(
          t('Recipient count changed. Preview again before issuing rewards.')
        )
      }
    },
    onSuccess: (campaign) => {
      setConfirmOpen(false)
      setPreview(null)
      requestId.current = null
      toast.success(
        t('Rewards issued to {{count}} users', {
          count: campaign.recipient_count,
        })
      )
      void queryClient.invalidateQueries({ queryKey: ['reward-campaigns'] })
      void queryClient.invalidateQueries({ queryKey: ['rewards'] })
      props.onCreated?.()
    },
  })

  const handlePreview = async (values: CampaignFormValues): Promise<void> => {
    const currentRevision = revision.current
    requestId.current ??= nanoid()
    const input = campaignFormToInput(values, requestId.current)
    setPreview(null)
    createMutation.reset()
    try {
      const result = await previewMutation.mutateAsync({
        audience: input.audience,
        start_time: input.start_time,
        end_time: input.end_time,
      })
      if (revision.current === currentRevision) {
        setPreview({ input, count: result.recipient_count })
      }
    } catch {
      // The mutation reports the error and leaves issuance disabled.
    }
  }

  const handleIssue = async (): Promise<void> => {
    if (
      !preview ||
      preview.count === 0 ||
      !Number.isSafeInteger(preview.input.max_quota * preview.count) ||
      preview.count > 100000 ||
      issuing.current
    ) {
      return
    }
    issuing.current = true
    try {
      await createMutation.mutateAsync({
        ...preview.input,
        expected_recipient_count: preview.count,
      })
    } catch {
      // Preserve the request ID so retrying an uncertain response is idempotent.
    } finally {
      issuing.current = false
    }
  }

  const budgetValid =
    preview === null ||
    (Number.isSafeInteger(preview.input.max_quota * preview.count) &&
      preview.count <= 100000)
  let budget = ''
  if (preview) {
    budget = formatQuotaWithCurrency(preview.input.min_quota * preview.count, {
      abbreviate: false,
    })
    if (preview.input.type === 'random') {
      budget += ` – ${formatQuotaWithCurrency(preview.input.max_quota * preview.count, { abbreviate: false })}`
    }
  }

  return (
    <Card className='rounded-2xl'>
      <CardHeader>
        <CardTitle className='flex items-center gap-2'>
          <Gift className='size-5' aria-hidden='true' />
          {t('Create reward campaign')}
        </CardTitle>
        <CardDescription>
          {t('Choose recipients, set the reward, then review before sending.')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Form {...form}>
          <form
            noValidate
            onSubmit={form.handleSubmit(handlePreview)}
            onChangeCapture={() => {
              revision.current += 1
              requestId.current = null
              setPreview(null)
              setConfirmOpen(false)
            }}
            className='space-y-5'
          >
            <fieldset disabled={createMutation.isPending} className='min-w-0'>
              <CampaignFields form={form} />
            </fieldset>
            <div className='flex flex-wrap items-center gap-3'>
              <Button
                type='submit'
                variant='outline'
                disabled={previewMutation.isPending || createMutation.isPending}
              >
                <Users className='size-4' aria-hidden='true' />
                {previewMutation.isPending
                  ? t('Loading...')
                  : t('Preview recipients')}
              </Button>
              {previewMutation.isError && (
                <p role='alert' className='text-destructive text-sm'>
                  {t('Failed to preview recipients')}
                </p>
              )}
            </div>
            {preview && (
              <div
                className='bg-muted/40 space-y-4 rounded-xl border p-4'
                aria-live='polite'
              >
                <dl className='grid gap-4 sm:grid-cols-3'>
                  <div>
                    <dt className='text-muted-foreground text-sm'>
                      {t('Eligible recipients')}
                    </dt>
                    <dd className='mt-1 text-2xl font-semibold tabular-nums'>
                      {preview.count}
                    </dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-sm'>
                      {t('Total reward budget')}
                    </dt>
                    <dd className='mt-1 text-2xl font-semibold tabular-nums'>
                      {budget}
                    </dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-sm'>
                      {t('Reward validity')}
                    </dt>
                    <dd className='mt-1 text-2xl font-semibold tabular-nums'>
                      {t('Valid for {{count}} days', {
                        count: preview.input.expires_in_days,
                      })}
                    </dd>
                  </div>
                </dl>
                <p className='text-muted-foreground text-xs'>
                  {t(
                    'The final recipient count is checked again when you issue the rewards.'
                  )}
                </p>
                {preview.count === 0 && (
                  <p className='text-muted-foreground text-sm'>
                    {t(
                      'No eligible users in this period. Adjust the time range or activity type.'
                    )}
                  </p>
                )}
                {!budgetValid && (
                  <p role='alert' className='text-destructive text-sm'>
                    {t(
                      'Reduce the reward amount or recipient count to stay within the distribution limit.'
                    )}
                  </p>
                )}
                <Button
                  type='button'
                  onClick={() => setConfirmOpen(true)}
                  disabled={
                    preview.count === 0 ||
                    !budgetValid ||
                    createMutation.isPending
                  }
                >
                  {t('Issue rewards')}
                </Button>
              </div>
            )}
          </form>
        </Form>
        <ConfirmDialog
          open={confirmOpen && preview !== null}
          onOpenChange={(open) => {
            if (!issuing.current) setConfirmOpen(open)
          }}
          title={t('Confirm reward distribution')}
          desc={
            <div className='space-y-2'>
              <p className='font-medium break-words'>{preview?.input.title}</p>
              <p>
                {t(
                  'Send one reward to each of {{count}} eligible users. Total budget: {{budget}}.',
                  { count: preview?.count ?? 0, budget }
                )}
              </p>
              <p>
                {t('Valid for {{count}} days', {
                  count: preview?.input.expires_in_days ?? 0,
                })}
              </p>
              <p>
                {t(
                  'Users can claim these rewards immediately after distribution.'
                )}
              </p>
              {createMutation.isError && (
                <p role='alert' className='text-destructive'>
                  {t(
                    'Failed to issue rewards. You can safely retry this request.'
                  )}
                </p>
              )}
            </div>
          }
          confirmText={
            createMutation.isPending ? t('Sending...') : t('Confirm and issue')
          }
          handleConfirm={() => {
            void handleIssue()
          }}
          isLoading={createMutation.isPending}
        />
      </CardContent>
    </Card>
  )
}
