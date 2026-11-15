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
import { useState } from 'react'
import { expect, test, vi } from 'vitest'

import type { ViewMode } from '../../constants'
import type { TokenUnit } from '../../types'
import { PricingToolbar, type PricingToolbarProps } from '../pricing-toolbar'

const props: PricingToolbarProps = {
  filteredCount: 0,
  sortBy: 'name',
  onSortChange: () => undefined,
  tokenUnit: 'M',
  onTokenUnitChange: () => undefined,
  showRechargePrice: false,
  onRechargePriceChange: () => undefined,
  viewMode: 'card',
  onViewModeChange: () => undefined,
  quotaTypeFilter: 'all',
  endpointTypeFilter: 'all',
  vendorFilter: 'all',
  groupFilter: 'all',
  tagFilter: 'all',
  onQuotaTypeChange: () => undefined,
  onEndpointTypeChange: () => undefined,
  onVendorChange: () => undefined,
  onGroupChange: () => undefined,
  onTagChange: () => undefined,
  vendors: [],
  groups: [],
  tags: [],
  models: [],
  hasActiveFilters: false,
  activeFilterCount: 0,
  onClearFilters: () => undefined,
}

test('opening model sort keeps the page interactive and choosing a price order updates sorting', async () => {
  const user = userEvent.setup()
  const onSortChange = vi.fn()
  const outsideAction = vi.fn()
  render(
    <>
      <button type='button' onClick={outsideAction}>
        Outside action
      </button>
      <PricingToolbar {...props} onSortChange={onSortChange} />
    </>
  )
  await user.click(screen.getByRole('button', { name: 'Name' }))
  expect(await screen.findByRole('menu')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Outside action' })).toBeVisible()
  expect(document.documentElement.style.overflow).not.toBe('hidden')
  expect(document.body.style.overflow).not.toBe('hidden')
  await user.click(screen.getByRole('button', { name: 'Outside action' }))
  expect(outsideAction).toHaveBeenCalledOnce()
  await user.click(screen.getByRole('button', { name: 'Name' }))
  await user.click(
    await screen.findByRole('menuitem', { name: 'Input Price: Low to High' })
  )
  expect(onSortChange).toHaveBeenCalledWith('input-price-low')
})

function PriceControlsFixture() {
  const [tokenUnit, setTokenUnit] = useState<TokenUnit>('M')
  const [showRechargePrice, setShowRechargePrice] = useState(false)
  const [viewMode, setViewMode] = useState<ViewMode>('card')
  return (
    <PricingToolbar
      {...props}
      tokenUnit={tokenUnit}
      onTokenUnitChange={setTokenUnit}
      showRechargePrice={showRechargePrice}
      onRechargePriceChange={setShowRechargePrice}
      viewMode={viewMode}
      onViewModeChange={setViewMode}
    />
  )
}

test('price and view controls retain independent selections with keyboard access', async () => {
  const user = userEvent.setup()
  render(<PriceControlsFixture />)
  expect(
    screen.getByRole('group', { name: 'Price display mode' })
  ).toBeVisible()
  expect(screen.getByRole('group', { name: 'Token unit' })).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Recharge' }))
  const unit = screen.getByRole('button', { name: '/1K' })
  unit.focus()
  await user.keyboard('{Enter}')
  await user.click(screen.getByRole('button', { name: 'Table view' }))
  expect(screen.getByRole('button', { name: 'Recharge' })).toHaveAttribute(
    'aria-pressed',
    'true'
  )
  expect(screen.getByRole('button', { name: 'Standard' })).toHaveAttribute(
    'aria-pressed',
    'false'
  )
  expect(unit).toHaveAttribute('aria-pressed', 'true')
  expect(screen.getByRole('button', { name: '/1M' })).toHaveAttribute(
    'aria-pressed',
    'false'
  )
  expect(screen.getByRole('button', { name: 'Table view' })).toHaveAttribute(
    'aria-pressed',
    'true'
  )
  expect(screen.getByRole('button', { name: 'Card view' })).toHaveAttribute(
    'aria-pressed',
    'false'
  )
})
