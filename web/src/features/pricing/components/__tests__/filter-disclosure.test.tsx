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
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { expect, test } from 'vitest'

import { PricingSidebar } from '../pricing-sidebar'

function Filters() {
  const [tag, setTag] = useState('all')
  return (
    <PricingSidebar
      quotaTypeFilter='all'
      endpointTypeFilter='all'
      vendorFilter='all'
      groupFilter='all'
      tagFilter={tag}
      onQuotaTypeChange={() => undefined}
      onEndpointTypeChange={() => undefined}
      onVendorChange={() => undefined}
      onGroupChange={() => undefined}
      onTagChange={setTag}
      vendors={[{ id: 1, name: 'Example AI' }]}
      groups={['default']}
      tags={['reasoning']}
      models={[
        {
          id: 1,
          model_name: 'reasoner',
          quota_type: 0,
          model_ratio: 1,
          completion_ratio: 2,
          vendor_name: 'Example AI',
          tags: 'reasoning',
          enable_groups: ['default'],
        },
      ]}
      hasActiveFilters={tag !== 'all'}
      onClearFilters={() => setTag('all')}
    />
  )
}

test('secondary catalog filters stay folded until requested and keep the active value visible when folded again', async () => {
  const user = userEvent.setup()
  render(<Filters />)
  const sidebar = within(
    screen.getByRole('complementary', { name: 'Catalog filters' })
  )
  expect(sidebar.getByRole('button', { name: /Example AI/ })).toBeVisible()
  const tags = sidebar.getByRole('button', { name: 'Model Tags' })
  expect(tags).toHaveAttribute('aria-expanded', 'false')
  expect(
    sidebar.queryByRole('button', { name: /^reasoning/ })
  ).not.toBeInTheDocument()
  await user.click(tags)
  const reasoning = sidebar.getByRole('button', { name: /^reasoning/ })
  await user.click(reasoning)
  expect(reasoning).toHaveAttribute('aria-pressed', 'true')
  await user.click(tags)
  expect(tags).toHaveAttribute('aria-expanded', 'false')
  expect(tags).toHaveTextContent('reasoning')
  await user.click(sidebar.getByRole('button', { name: 'Reset' }))
  expect(tags).not.toHaveTextContent('reasoning')
  await user.click(tags)
  expect(sidebar.getByRole('button', { name: /^reasoning/ })).toHaveAttribute(
    'aria-pressed',
    'false'
  )
})
