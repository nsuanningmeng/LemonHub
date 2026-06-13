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
import { SITE_VALIDATION } from '../constants'
import { type Site, type SiteCreatePayload } from '../types'

// ============================================================================
// Form Schema
// ============================================================================

export function getSiteFormSchema(t: TFunction) {
  return z.object({
    name: z
      .string()
      .min(SITE_VALIDATION.NAME_MIN_LENGTH, t('Name is required'))
      .max(SITE_VALIDATION.NAME_MAX_LENGTH, t('Name is too long')),
    domains_text: z
      .string()
      .min(1, t('At least one domain is required')),
    owner_username: z.string().optional(),
    logo: z.string().optional(),
    notice: z.string().optional(),
    footer: z.string().optional(),
    discount_rate: z.number().min(0).max(10000),
    status: z.number(),
    wallet_warn_threshold: z.number().min(0),
    pay_config: z.string().optional(),
  })
}

export type SiteFormValues = {
  name: string
  domains_text: string
  owner_username?: string
  logo?: string
  notice?: string
  footer?: string
  discount_rate: number
  status: number
  wallet_warn_threshold: number
  pay_config?: string
}

// ============================================================================
// Form Defaults
// ============================================================================

export const SITE_FORM_DEFAULT_VALUES: SiteFormValues = {
  name: '',
  domains_text: '',
  owner_username: '',
  logo: '',
  notice: '',
  footer: '',
  discount_rate: 10000,
  status: 1,
  wallet_warn_threshold: 0,
  pay_config: '',
}

// ============================================================================
// Form Data Transformation
// ============================================================================

/**
 * Transform form values to API create/update payload.
 * Splits domains_text by newline, trims each, drops blank lines.
 */
export function transformFormToPayload(data: SiteFormValues): SiteCreatePayload {
  const domains = data.domains_text
    .split('\n')
    .map((d) => d.trim())
    .filter(Boolean)

  return {
    name: data.name,
    domains,
    owner_username: data.owner_username || '',
    logo: data.logo || '',
    notice: data.notice || '',
    footer: data.footer || '',
    discount_rate: data.discount_rate,
    status: data.status,
    wallet_warn_threshold: data.wallet_warn_threshold,
    pay_config: data.pay_config || '',
  }
}

/**
 * Transform Site record to form defaults for editing.
 * Joins the site's domain strings with newlines for the textarea.
 */
export function transformSiteToForm(site: Site): SiteFormValues {
  return {
    name: site.name,
    domains_text: site.domains.join('\n'),
    owner_username: site.owner_username,
    logo: site.logo || '',
    notice: site.notice || '',
    footer: site.footer || '',
    discount_rate: site.discount_rate,
    status: site.status,
    wallet_warn_threshold: site.wallet_warn_threshold,
    pay_config: site.pay_config || '',
  }
}
