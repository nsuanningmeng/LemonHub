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
import { getModelQuotaType } from '../lib/filters'
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
      count: props.models.filter(
        (model) => getModelQuotaType(model) === QUOTA_TYPES.TOKEN
      ).length,
    },
    {
      value: QUOTA_TYPES.REQUEST,
      count: props.models.filter(
        (model) => getModelQuotaType(model) === QUOTA_TYPES.REQUEST
      ).length,
    },
  ]

  return (
    <HStack
      gap={6}
      wrap='wrap'
      role='group'
      aria-label={t('Pricing Type')}
      className='border-border/60 border-b'
    >
      {options.map((option) => (
        <button
          key={option.value}
          type='button'
          aria-pressed={props.value === option.value}
          onClick={() => props.onChange(option.value)}
          className={cn(
            'focus-visible:ring-ring flex min-w-0 items-center gap-2 border-b-2 py-3 text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:outline-none',
            props.value === option.value
              ? 'text-primary border-primary'
              : 'border-transparent text-muted-foreground hover:text-foreground'
          )}
        >
          {labels[option.value]}
          <b className='text-xs font-normal tabular-nums'>
            {option.count.toLocaleString()}
          </b>
        </button>
      ))}
    </HStack>
  )
}
