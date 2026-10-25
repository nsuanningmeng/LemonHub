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
import type { TFunction } from 'i18next'
import { z } from 'zod'

import { getCurrencyDisplay } from '@/lib/currency'

import type { RewardCampaignInput } from '../types'

export function rewardDollarsToQuota(amount: string): number {
  return Math.round(Number(amount) * getCurrencyDisplay().config.quotaPerUnit)
}

export function getCampaignFormSchema(t: TFunction) {
  const expiryError = t('Enter a whole number of days between 1 and 3650')
  const amount = z
    .string()
    .trim()
    .refine((value) => {
      const quota = rewardDollarsToQuota(value)
      return (
        /^\d+(\.\d+)?$/.test(value) && Number.isSafeInteger(quota) && quota > 0
      )
    }, t('Enter a positive reward amount within the wallet limit'))
  return z
    .object({
      title: z
        .string()
        .trim()
        .min(1, t('Enter a campaign title'))
        .max(120, t('Campaign title must be at most 120 characters')),
      description: z
        .string()
        .trim()
        .max(1000, t('Description must be at most 1000 characters')),
      type: z.enum(['fixed', 'random']),
      minimum: amount,
      maximum: z.string(),
      audience: z.enum(['topup', 'usage', 'both', 'either']),
      expires_in_days: z
        .number({ error: expiryError })
        .int(expiryError)
        .min(1, expiryError)
        .max(3650, expiryError),
      start: z.string().min(1, t('Select a start time')),
      end: z.string().min(1, t('Select an end time')),
    })
    .superRefine((values, context) => {
      const start = new Date(values.start).getTime()
      const end = new Date(values.end).getTime()
      if (!Number.isFinite(start) || start <= 0) {
        context.addIssue({
          code: 'custom',
          path: ['start'],
          message: t('Select a valid start time'),
        })
      }
      if (!Number.isFinite(end) || end <= start) {
        context.addIssue({
          code: 'custom',
          path: ['end'],
          message: t('End time must be after start time'),
        })
      }
      if (values.type === 'random') {
        const maximum = amount.safeParse(values.maximum)
        if (!maximum.success) {
          context.addIssue({
            code: 'custom',
            path: ['maximum'],
            message: t(
              'Enter a positive reward amount within the wallet limit'
            ),
          })
        } else if (
          rewardDollarsToQuota(maximum.data) <
          rewardDollarsToQuota(values.minimum)
        ) {
          context.addIssue({
            code: 'custom',
            path: ['maximum'],
            message: t('Maximum reward must be at least the minimum reward'),
          })
        }
      }
    })
}

export type CampaignFormValues = z.infer<
  ReturnType<typeof getCampaignFormSchema>
>

export function defaultCampaignFormValues(): CampaignFormValues {
  const end = new Date()
  const start = new Date(end.getTime() - 7 * 24 * 60 * 60 * 1000)
  const localDate = (date: Date): string =>
    new Date(date.getTime() - date.getTimezoneOffset() * 60000)
      .toISOString()
      .slice(0, 16)
  return {
    title: '',
    description: '',
    type: 'fixed',
    minimum: '1',
    maximum: '5',
    audience: 'either',
    expires_in_days: 7,
    start: localDate(start),
    end: localDate(end),
  }
}

export function campaignFormToInput(
  values: CampaignFormValues,
  requestId: string
): RewardCampaignInput {
  const minimum = rewardDollarsToQuota(values.minimum)
  return {
    title: values.title,
    description: values.description,
    type: values.type,
    min_quota: minimum,
    max_quota:
      values.type === 'random' ? rewardDollarsToQuota(values.maximum) : minimum,
    audience: values.audience,
    expires_in_days: values.expires_in_days,
    start_time: Math.floor(new Date(values.start).getTime() / 1000),
    end_time: Math.floor(new Date(values.end).getTime() / 1000),
    request_id: requestId,
  }
}
