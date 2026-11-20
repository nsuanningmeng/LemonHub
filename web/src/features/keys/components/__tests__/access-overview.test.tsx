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
import { expect, it } from 'vitest'

import { apiKeySchema } from '../../types'
import { ApiKeysOverview } from '../api-keys-overview'

const enabledKey = apiKeySchema.parse({
  id: 1,
  name: 'Production',
  key: 'masked',
  status: 1,
  effective_status: 1,
  remain_quota: 100,
  used_quota: 0,
  unlimited_quota: false,
  expired_time: -1,
  created_time: 1,
  accessed_time: 1,
  model_limits_enabled: false,
})

it('labels enabled counts as page-scoped and excludes derived expired keys', () => {
  render(
    <ApiKeysOverview
      keys={[enabledKey, { ...enabledKey, id: 2, effective_status: 3 }]}
      total={42}
      isLoading={false}
      filtered={false}
    />
  )
  expect(screen.getByText('Total keys').nextElementSibling).toHaveTextContent(
    '42'
  )
  expect(
    screen.getByText('Active on this page').nextElementSibling
  ).toHaveTextContent('1')
  expect(
    screen.getByText('Inactive on this page').nextElementSibling
  ).toHaveTextContent('1')
})

it('labels filtered totals as matching keys and shows zero for no matches', () => {
  render(<ApiKeysOverview keys={[]} total={0} isLoading={false} filtered />)
  expect(
    screen.getByText('Matching keys').nextElementSibling
  ).toHaveTextContent('0')
  expect(screen.queryByText('Total keys')).not.toBeInTheDocument()
  expect(
    screen.getByText('Active on this page').nextElementSibling
  ).toHaveTextContent('0')
  expect(
    screen.getByText('Inactive on this page').nextElementSibling
  ).toHaveTextContent('0')
})

it('marks the overview busy during initial loading and avoids showing a false zero count', () => {
  render(<ApiKeysOverview keys={[]} total={0} isLoading filtered={false} />)
  expect(
    screen.getByRole('region', { name: 'Access overview' })
  ).toHaveAttribute('aria-busy', 'true')
  expect(
    screen.getByText('Total keys').nextElementSibling
  ).not.toHaveTextContent('0')
  expect(
    screen.getByText('Inactive on this page').nextElementSibling
  ).not.toHaveTextContent('0')
})

it('counts disabled, expired, and exhausted keys as inactive on this page without using the total result count', () => {
  render(
    <ApiKeysOverview
      keys={[
        enabledKey,
        { ...enabledKey, id: 2, status: 2, effective_status: 2 },
        { ...enabledKey, id: 3, effective_status: 3 },
        { ...enabledKey, id: 4, effective_status: 4 },
      ]}
      total={64}
      isLoading={false}
      filtered={false}
    />
  )

  expect(
    screen.getByText('Active on this page').nextElementSibling
  ).toHaveTextContent('1')
  expect(
    screen.getByText('Inactive on this page').nextElementSibling
  ).toHaveTextContent('3')
})

it('allows large totals to wrap in narrow metric columns while retaining the full value', () => {
  render(
    <ApiKeysOverview
      keys={[enabledKey]}
      total={1234567}
      isLoading={false}
      filtered={false}
    />
  )
  const count = screen.getByText('Total keys').nextElementSibling
  expect(count).toHaveTextContent('1,234,567')
  expect(count).toHaveClass('wrap-anywhere')
})
