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
import { render, screen } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { expect, test } from 'vitest'

import { createLocaleI18n } from '@/i18n/__tests__/helpers'
import { TableColumnFixture } from '@/i18n/__tests__/table-fixture'

import { subscriptionPlanSchema } from '../../types'
import { useSubscriptionsColumns } from '../subscriptions-columns'

function StatusFixture() {
  const columns = useSubscriptionsColumns()
  const plan = subscriptionPlanSchema.parse({
    id: 1,
    title: 'Local',
    price_amount: 5,
    duration_unit: 'month',
    duration_value: 1,
    quota_reset_period: 'never',
    enabled: true,
    sort_order: 0,
    max_purchase_per_user: 1,
    total_amount: 100,
  })
  return (
    <TableColumnFixture
      columns={columns}
      data={[{ plan }, { plan: { ...plan, id: 2, enabled: false } }]}
      columnId='enabled'
    />
  )
}

test('plan status uses state labels while the dictionary retains distinct action verbs', async () => {
  const i18n = await createLocaleI18n('en')
  render(
    <I18nextProvider i18n={i18n}>
      <StatusFixture />
    </I18nextProvider>
  )
  expect(screen.getByText('Enabled', { exact: true })).toBeInTheDocument()
  expect(screen.getByText('Disabled', { exact: true })).toBeInTheDocument()
  expect(i18n.t('Enable')).toBe('Enable')
  expect(i18n.t('Disable')).toBe('Disable')
})
