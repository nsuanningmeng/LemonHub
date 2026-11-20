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
import { useTranslation } from 'react-i18next'

import { getQuotaTypeLabels, QUOTA_TYPES } from '../constants'
import { getModelQuotaType } from '../lib/filters'
import type { PricingModel } from '../types'
import { FilterSection } from './catalog-filter-section'

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
    <FilterSection
      defaultOpen
      title={t('Pricing Type')}
      value={props.value}
      options={options.map((option) => ({
        ...option,
        label: labels[option.value],
      }))}
      onChange={props.onChange}
    />
  )
}
