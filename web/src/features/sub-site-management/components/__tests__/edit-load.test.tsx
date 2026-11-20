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
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { siteSchema } from '../../types'
import { SubSiteMutateDrawer } from '../sub-site-mutate-drawer'
import { SubSiteProvider } from '../sub-site-provider'

afterEach(() => {
  vi.restoreAllMocks()
})

test('failed site details keep editing disabled until a retry loads that site', async () => {
  const site = siteSchema.parse({
    id: 1,
    name: 'Studio',
    logo: '',
    notice: '',
    footer: '',
    owner_user_id: 4,
    owner_username: 'owner',
    status: 1,
    wallet_balance: 0,
    discount_rate: 10000,
    wallet_warn_threshold: 0,
    pay_config: '',
    domains: [],
    created_time: 1,
    updated_time: 1,
  })
  const user = userEvent.setup()
  vi.spyOn(api, 'get')
    .mockResolvedValueOnce({ data: { success: false } })
    .mockResolvedValueOnce({ data: { success: true, data: site } })
  render(
    <SubSiteProvider>
      <SubSiteMutateDrawer open onOpenChange={() => {}} currentRow={site} />
    </SubSiteProvider>
  )
  expect(await screen.findByRole('alert')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  expect(await screen.findByDisplayValue('Studio')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Save changes' })).toBeEnabled()
})
