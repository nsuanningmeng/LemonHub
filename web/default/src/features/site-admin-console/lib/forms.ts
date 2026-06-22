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
import { z } from 'zod'
import type { TFunction } from 'i18next'
import { parseQuotaFromDollars } from '@/lib/format'
import { REDEMPTION_VALIDATION } from '../constants'
import {
  type BrandingPayload,
  type CreateRedemptionPayload,
  type PayConfig,
} from '../types'

// ============================================================================
// Redemption Generate Form
// ============================================================================

export function getGenerateFormSchema(t: TFunction) {
  return z.object({
    name: z
      .string()
      .min(REDEMPTION_VALIDATION.NAME_MIN_LENGTH, t('Name is required'))
      .max(REDEMPTION_VALIDATION.NAME_MAX_LENGTH, t('Name is too long')),
    quota_dollars: z.number().min(0, t('Quota must be a positive number')),
    count: z
      .number()
      .min(REDEMPTION_VALIDATION.COUNT_MIN)
      .max(REDEMPTION_VALIDATION.COUNT_MAX),
    expired_time: z.date().optional(),
  })
}

export type GenerateFormValues = {
  name: string
  quota_dollars: number
  count: number
  expired_time?: Date
}

export const GENERATE_FORM_DEFAULT_VALUES: GenerateFormValues = {
  name: '',
  quota_dollars: 10,
  count: 1,
  expired_time: undefined,
}

export function transformGenerateFormToPayload(
  data: GenerateFormValues
): CreateRedemptionPayload {
  return {
    name: data.name,
    quota: parseQuotaFromDollars(data.quota_dollars),
    count: data.count || 1,
    expired_time: data.expired_time
      ? Math.floor(data.expired_time.getTime() / 1000)
      : 0,
  }
}

// ============================================================================
// Branding Form
// ============================================================================

export function getBrandingFormSchema(t: TFunction) {
  return z.object({
    name: z
      .string()
      .min(1, t('Name is required'))
      .max(100, t('Name is too long')),
    logo: z.string().optional(),
    notice: z.string().optional(),
    footer: z.string().optional(),
    home_badge: z.string().optional(),
    home_title_line1: z.string().optional(),
    home_title_line2: z.string().optional(),
  })
}

export type BrandingFormValues = {
  name: string
  logo?: string
  notice?: string
  footer?: string
  home_badge?: string
  home_title_line1?: string
  home_title_line2?: string
}

export const BRANDING_FORM_DEFAULT_VALUES: BrandingFormValues = {
  name: '',
  logo: '',
  notice: '',
  footer: '',
  home_badge: '',
  home_title_line1: '',
  home_title_line2: '',
}

export function transformBrandingFormToPayload(
  data: BrandingFormValues
): BrandingPayload {
  return {
    name: data.name,
    logo: data.logo || '',
    notice: data.notice || '',
    footer: data.footer || '',
    home_badge: data.home_badge || '',
    home_title_line1: data.home_title_line1 || '',
    home_title_line2: data.home_title_line2 || '',
  }
}

// ============================================================================
// Pay Config Form
// ============================================================================

export function getPayConfigFormSchema(t: TFunction) {
  return z.object({
    epay_id: z.string().min(1, t('Merchant ID is required')),
    epay_key: z.string().min(1, t('Merchant Key is required')),
    pay_address: z
      .string()
      .min(1, t('Payment Gateway URL is required'))
      .url(t('Payment Gateway URL must be a valid URL')),
    pay_methods: z.string().optional(),
  })
}

export type PayConfigFormValues = {
  epay_id: string
  epay_key: string
  pay_address: string
  pay_methods?: string
}

export const PAY_CONFIG_FORM_DEFAULT_VALUES: PayConfigFormValues = {
  epay_id: '',
  epay_key: '',
  pay_address: '',
  pay_methods: '',
}

export function transformPayConfigFormToPayload(
  data: PayConfigFormValues
): PayConfig {
  const raw = data.pay_methods?.trim() ?? ''
  const methods = raw
    ? raw
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean)
    : []
  return {
    epay_id: data.epay_id,
    epay_key: data.epay_key,
    pay_address: data.pay_address,
    pay_methods: methods,
  }
}

export function transformPayConfigToFormValues(
  config: PayConfig
): PayConfigFormValues {
  return {
    epay_id: config.epay_id,
    epay_key: config.epay_key,
    pay_address: config.pay_address,
    pay_methods: Array.isArray(config.pay_methods)
      ? config.pay_methods.join(',')
      : '',
  }
}
