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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import type { PricingModel } from '../../types'
import { CatalogBillingFilter } from '../catalog-billing-filter'
import { EmptyState } from '../empty-state'
import { LoadingSkeleton } from '../loading-skeleton'
import { ModelCard } from '../model-card'
import { PricingSidebar } from '../pricing-sidebar'
import { SearchBar } from '../search-bar'

const model: PricingModel = {
  id: 1,
  model_name: 'reasoning-model-with-a-deliberately-long-identifier',
  vendor_name: 'Example AI',
  description: 'A model for complex reasoning tasks.',
  quota_type: 0,
  model_ratio: 1,
  completion_ratio: 2,
  enable_groups: ['default'],
}
let client: QueryClient

beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  client.setQueryData(['status'], {})
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: {} } })
})

afterEach(() => client.clear())

function SearchFixture() {
  const [search, setSearch] = useState('')
  return (
    <SearchBar
      value={search}
      onChange={setSearch}
      onClear={() => setSearch('')}
    />
  )
}

function FilterFixture() {
  const [vendor, setVendor] = useState('all')
  return (
    <PricingSidebar
      quotaTypeFilter='all'
      endpointTypeFilter='all'
      vendorFilter={vendor}
      groupFilter='all'
      tagFilter='all'
      onQuotaTypeChange={() => undefined}
      onEndpointTypeChange={() => undefined}
      onVendorChange={setVendor}
      onGroupChange={() => undefined}
      onTagChange={() => undefined}
      vendors={[{ id: 1, name: 'Example AI' }]}
      groups={['default']}
      tags={[]}
      models={[model]}
      hasActiveFilters={vendor !== 'all'}
      onClearFilters={() => setVendor('all')}
    />
  )
}

test('catalog search can be focused from the keyboard, cleared and dismissed', async () => {
  const user = userEvent.setup()
  render(<SearchFixture />)
  const input = screen.getByRole('textbox', { name: 'Search models' })
  await user.keyboard('{Control>}k{/Control}')
  expect(input).toHaveFocus()
  await user.type(input, 'reasoning')
  expect(input).toHaveValue('reasoning')
  await user.click(screen.getByRole('button', { name: 'Clear search' }))
  expect(input).toHaveValue('')
  await user.keyboard('{Control>}k{/Control}{Escape}')
  expect(input).not.toHaveFocus()
})

test('selecting and resetting a provider exposes the selected state to assistive technology', async () => {
  const user = userEvent.setup()
  render(<FilterFixture />)
  const provider = screen.getByRole('button', { name: /Example AI/ })
  expect(provider).toHaveAttribute('aria-pressed', 'false')
  expect(screen.getByRole('button', { name: 'Reset' })).toBeDisabled()
  await user.click(provider)
  expect(provider).toHaveAttribute('aria-pressed', 'true')
  await user.click(screen.getByRole('button', { name: 'Reset' }))
  expect(provider).toHaveAttribute('aria-pressed', 'false')
  expect(screen.getByRole('button', { name: 'Reset' })).toBeDisabled()
})

test('a model dossier keeps long names readable and allows keyboard access to its details', async () => {
  const user = userEvent.setup()
  const onDetails = vi.fn()
  render(
    <QueryClientProvider client={client}>
      <ModelCard model={model} onClick={onDetails} />
    </QueryClientProvider>
  )
  expect(screen.getByRole('article', { name: model.model_name })).toBeVisible()
  expect(screen.getByRole('article', { name: model.model_name })).toHaveClass(
    'surface-violet'
  )
  expect(screen.getByRole('heading', { name: model.model_name })).toHaveClass(
    '[overflow-wrap:anywhere]'
  )
  expect(screen.getByText('Example AI')).toBeVisible()
  expect(screen.getByText('A model for complex reasoning tasks.')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Copy model name' })).toBeEnabled()
  const details = screen.getByRole('button', { name: 'Details' })
  details.focus()
  await user.keyboard('{Enter}')
  expect(onDetails).toHaveBeenCalledOnce()
})

function BillingFixture() {
  const [value, setValue] = useState('all')
  return (
    <CatalogBillingFilter
      models={[
        model,
        { ...model, id: 2, model_name: 'image-generation', quota_type: 1 },
      ]}
      value={value}
      onChange={setValue}
    />
  )
}

test('billing categories expose their model counts and preserve a single selected category', async () => {
  const user = userEvent.setup()
  render(<BillingFixture />)
  const all = screen.getByRole('button', { name: /^All Models/ })
  const token = screen.getByRole('button', { name: /^Token-based/ })
  const request = screen.getByRole('button', { name: /^Per Request/ })
  expect(all).toHaveTextContent('2')
  expect(token).toHaveTextContent('1')
  expect(request).toHaveTextContent('1')
  expect(all).toHaveAttribute('aria-pressed', 'true')
  await user.click(request)
  expect(request).toHaveAttribute('aria-pressed', 'true')
  expect(all).toHaveAttribute('aria-pressed', 'false')
  expect(token).toHaveAttribute('aria-pressed', 'false')
  await user.click(all)
  expect(all).toHaveAttribute('aria-pressed', 'true')
  expect(request).toHaveAttribute('aria-pressed', 'false')
})

test('an empty model search retains a clear recovery action within its colored surface', async () => {
  const user = userEvent.setup()
  const onClear = vi.fn()
  render(
    <EmptyState
      searchQuery='unavailable-model'
      hasActiveFilters
      onClearFilters={onClear}
    />
  )
  expect(screen.getByRole('heading', { name: 'No models found' })).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Clear all filters' }))
  expect(onClear).toHaveBeenCalledOnce()
})

test.each(['card', 'table'] as const)(
  'the %s catalog loading view communicates its busy state without presenting placeholder values as models',
  (viewMode) => {
    render(<LoadingSkeleton viewMode={viewMode} />)
    expect(
      screen.getByRole('status', { name: 'Loading models' })
    ).toHaveAttribute('aria-busy', 'true')
    expect(screen.queryByRole('article')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Details' })
    ).not.toBeInTheDocument()
  }
)
