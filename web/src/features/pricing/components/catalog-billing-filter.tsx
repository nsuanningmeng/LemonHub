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
import { HStack } from '@astryxdesign/core/HStack'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import { getQuotaTypeLabels, QUOTA_TYPES } from '../constants'
import type { PricingModel } from '../types'

type CatalogBillingFilterProps = {
  models: PricingModel[]
  value: string
  onChange: (value: string) => void
}

export function CatalogBillingFilter(props: CatalogBillingFilterProps) {
  const { t } = useTranslation()
  const labels = getQuotaTypeLabels(t)
  const options = [
    { value: QUOTA_TYPES.ALL, count: props.models.length },
    {
      value: QUOTA_TYPES.TOKEN,
      count: props.models.filter((model) => model.quota_type === 0).length,
    },
    {
      value: QUOTA_TYPES.REQUEST,
      count: props.models.filter((model) => model.quota_type === 1).length,
    },
  ]

  return (
    <HStack gap={3} wrap='wrap' role='group' aria-label={t('Pricing Type')}>
      {options.map((option) => (
        <button
          key={option.value}
          type='button'
          aria-pressed={props.value === option.value}
          onClick={() => props.onChange(option.value)}
          className={cn(
            'focus-visible:ring-ring flex min-w-0 items-center gap-3 rounded-xl border px-4 py-3 text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:outline-none',
            props.value === option.value
              ? 'surface-brand border-transparent'
              : 'border-tone-violet-border bg-card text-tone-violet-strong hover:bg-tone-violet'
          )}
        >
          {labels[option.value]}
          <b className='font-mono text-xs font-normal opacity-75'>
            {option.count.toLocaleString()}
          </b>
        </button>
      ))}
    </HStack>
  )
}
