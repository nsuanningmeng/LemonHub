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
import { formatTimestamp } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import { createRewardCampaign, previewRewardRecipients } from './api'
import { CampaignFields } from './campaign-fields'
import { CampaignRewardEstimate } from './campaign-reward-estimate'
import {
  campaignFormToInput,
  defaultCampaignFormValues,
  getCampaignFormSchema,
  type CampaignFormValues,
} from './lib/campaign-form'
import type { RewardCampaignInput } from './types'

type CampaignPreview = { input: RewardCampaignInput; count: number }
type CampaignConfirmation = {
  input: RewardCampaignInput
  previewCount?: number
}

export function CampaignForm(props: { onCreated?: () => void }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const schema = useMemo(() => getCampaignFormSchema(t), [t])
  const form = useForm<CampaignFormValues>({
    resolver: zodResolver(schema),
    defaultValues: defaultCampaignFormValues(),
  })
  const [preview, setPreview] = useState<CampaignPreview | null>(null)
  const [confirmation, setConfirmation] = useState<CampaignConfirmation | null>(
    null
  )
  const revision = useRef(0)
  const requestId = useRef<string | null>(null)
  const issuing = useRef(false)
  const previewMutation = useMutation({
    mutationFn: previewRewardRecipients,
    onError: handleServerError,
  })
  const createMutation = useMutation({
    mutationFn: createRewardCampaign,
    onError: handleServerError,
    onSuccess: () => {
      revision.current += 1
      previewMutation.reset()
      setConfirmation(null)
      setPreview(null)
      requestId.current = null
      toast.success(
        t(
          'Reward campaign created. Eligible users will receive rewards automatically.'
        )
      )
      void queryClient.invalidateQueries({ queryKey: ['reward-campaigns'] })
      void queryClient.invalidateQueries({ queryKey: ['rewards'] })
      props.onCreated?.()
    },
  })

  const invalidatePreview = (): void => {
    revision.current += 1
    requestId.current = null
    setPreview(null)
    setConfirmation(null)
    previewMutation.reset()
  }

  const handlePreview = async (values: CampaignFormValues): Promise<void> => {
    const currentRevision = revision.current
    requestId.current ??= nanoid()
    const input = campaignFormToInput(values, requestId.current)
    setPreview(null)
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
      // Preview is informational; failure does not block campaign creation.
    }
  }

  const handlePrepare = (values: CampaignFormValues): void => {
    requestId.current ??= nanoid()
    createMutation.reset()
    setConfirmation({
      input: campaignFormToInput(values, requestId.current),
      previewCount: preview?.count,
    })
  }

  const handleCreate = async (): Promise<void> => {
    if (!confirmation || issuing.current) return
    issuing.current = true
    try {
      await createMutation.mutateAsync(confirmation.input)
    } catch {
      // Retain the exact confirmation payload and ID for an uncertain retry.
    } finally {
      issuing.current = false
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
          {t(
            'Choose the activity period and reward. Previewing current recipients is optional.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Form {...form}>
          <form
            noValidate
            onSubmit={form.handleSubmit(handlePrepare)}
            onChangeCapture={invalidatePreview}
            className='space-y-5'
          >
            <fieldset disabled={createMutation.isPending} className='min-w-0'>
              <CampaignFields
                form={form}
                onSelectionChange={invalidatePreview}
              />
            </fieldset>
            <div className='flex flex-wrap items-center gap-3'>
              <Button
                type='button'
                variant='outline'
                onClick={form.handleSubmit(handlePreview)}
                disabled={previewMutation.isPending || createMutation.isPending}
              >
                <Users className='size-4' aria-hidden='true' />
                {previewMutation.isPending
                  ? t('Loading...')
                  : t('Preview recipients')}
              </Button>
              <Button type='submit' disabled={createMutation.isPending}>
                {t('Create campaign')}
              </Button>
            </div>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Preview is optional and only reflects records currently available.'
              )}
            </p>
            {previewMutation.isError && (
              <p role='alert' className='text-destructive text-sm'>
                {t('Failed to preview recipients')}
              </p>
            )}
            {preview && (
              <div
                className='bg-muted/40 space-y-4 rounded-xl border p-4'
                aria-live='polite'
              >
                <CampaignRewardEstimate
                  input={preview.input}
                  recipientCount={preview.count}
                />
                {preview.count === 0 && (
                  <p className='text-muted-foreground text-sm'>
                    {t(
                      'No eligible users yet. You can create the campaign now; future eligible users will be added automatically.'
                    )}
                  </p>
                )}
              </div>
            )}
          </form>
        </Form>
        <ConfirmDialog
          open={confirmation !== null}
          onOpenChange={(open) => {
            if (!open && !issuing.current) setConfirmation(null)
          }}
          title={t('Confirm campaign creation')}
          desc={
            <div className='space-y-4'>
              <p className='font-medium break-words'>
                {confirmation?.input.title}
              </p>
              {confirmation && (
                <>
                  <dl className='grid gap-3 text-sm'>
                    <div>
                      <dt className='text-muted-foreground'>
                        {t('Amount per user')}
                      </dt>
                      <dd className='font-medium tabular-nums'>
                        {formatQuotaWithCurrency(confirmation.input.min_quota, {
                          abbreviate: false,
                        })}
                        {confirmation.input.type === 'random' &&
                          ` – ${formatQuotaWithCurrency(confirmation.input.max_quota, { abbreviate: false })}`}
                      </dd>
                    </div>
                    <div>
                      <dt className='text-muted-foreground'>
                        {t('Activity period')}
                      </dt>
                      <dd>
                        {formatTimestamp(confirmation.input.start_time)} –{' '}
                        {formatTimestamp(confirmation.input.end_time)}
                      </dd>
                    </div>
                  </dl>
                  <CampaignRewardEstimate
                    input={confirmation.input}
                    recipientCount={confirmation.previewCount}
                  />
                </>
              )}
              <p>
                {t(
                  'Eligible users are added automatically throughout the activity period. There is no campaign-wide cap on recipients or total rewards.'
                )}
              </p>
              <p>
                {t(
                  'Each reward expires the configured number of days after it is issued to that user.'
                )}
              </p>
              {createMutation.isError && (
                <p role='alert' className='text-destructive'>
                  {t(
                    'Failed to create reward campaign. You can safely retry this request.'
                  )}
                </p>
              )}
            </div>
          }
          confirmText={
            createMutation.isPending
              ? t('Creating...')
              : t('Confirm and create')
          }
          handleConfirm={() => {
            void handleCreate()
          }}
          isLoading={createMutation.isPending}
        />
      </CardContent>
    </Card>
  )
}
