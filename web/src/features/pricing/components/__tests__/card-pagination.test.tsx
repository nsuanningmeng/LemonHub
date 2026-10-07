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
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { SORT_OPTIONS } from '../../constants'
import { useFilters } from '../../hooks/use-filters'
import type { PricingModel } from '../../types'
import { ModelCardGrid } from '../model-card-grid'

const models: PricingModel[] = Array.from({ length: 100 }, (_, index) => ({
  id: index + 1,
  model_name: `model-${String(index + 1).padStart(3, '0')}`,
  description: index < 25 ? 'searchable' : 'other',
  vendor_name: index < 25 ? 'Target' : 'Other',
  tags: index < 25 ? 'featured' : '',
  enable_groups: index < 25 ? ['vip'] : ['default'],
  quota_type: 0,
  model_ratio: index + 1,
  completion_ratio: 1,
}))
const clients: QueryClient[] = []
function FilteredGrid() {
  const filters = useFilters(models)
  return (
    <>
      <button type='button' onClick={() => filters.setGroupFilter('vip')}>
        Group filter
      </button>
      <button type='button' onClick={() => filters.setVendorFilter('Target')}>
        Vendor filter
      </button>
      <button type='button' onClick={() => filters.setTagFilter('featured')}>
        Tag filter
      </button>
      <button
        type='button'
        onClick={() => filters.setSearchInput('searchable')}
      >
        Search filter
      </button>
      <button
        type='button'
        onClick={() => filters.setSortBy(SORT_OPTIONS.PRICE_HIGH)}
      >
        Price sort
      </button>
      <button type='button' onClick={() => filters.setGroupFilter('missing')}>
        Empty group
      </button>
      <button type='button' onClick={() => filters.clearFilters()}>
        Clear filters
      </button>
      <button type='button' onClick={() => filters.setTokenUnit('K')}>
        Change price unit
      </button>
      <ModelCardGrid
        models={filters.filteredModels}
        selectedGroup={filters.groupFilter}
        tokenUnit={filters.tokenUnit}
        onModelClick={() => undefined}
      />
    </>
  )
}
async function renderGrid() {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { models: [] } },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  const root = createRootRoute()
  const route = createRoute({
    getParentRoute: () => root,
    path: 'pricing/',
    component: FilteredGrid,
  })
  const router = createRouter({
    routeTree: root.addChildren([route]),
    history: createMemoryHistory({ initialEntries: ['/pricing/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  await screen.findByText('Page 1 of 5')
  return userEvent.setup()
}
afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
})
test.each(['Group filter', 'Vendor filter', 'Tag filter', 'Search filter'])(
  'page five resets to first page after %s and a single Previous click returns from page two',
  async (filter) => {
    const user = await renderGrid()
    for (let page = 1; page < 5; page++) {
      await user.click(screen.getByRole('button', { name: 'Next page' }))
    }
    expect(screen.getByText('Page 5 of 5')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: filter }))
    await screen.findByText('Page 1 of 2')
    expect(
      screen.getByRole('heading', { name: 'model-001' })
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Previous page' })).toBeDisabled()
    await user.click(screen.getByRole('button', { name: 'Next page' }))
    expect(screen.getByText('Page 2 of 2')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Previous page' }))
    expect(screen.getByText('Page 1 of 2')).toBeInTheDocument()
  }
)
test('sorting resets the page and changes cards while a price-unit update preserves the current page', async () => {
  const user = await renderGrid()
  await user.click(screen.getByRole('button', { name: 'Next page' }))
  await user.click(screen.getByRole('button', { name: 'Change price unit' }))
  expect(screen.getByText('Page 2 of 5')).toBeInTheDocument()
  expect(screen.getByRole('heading', { name: 'model-021' })).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Price sort' }))
  expect(screen.getByText('Page 1 of 5')).toBeInTheDocument()
  expect(screen.getByRole('heading', { name: 'model-100' })).toBeInTheDocument()
})
test('an empty result does not keep a hidden page when models become available again', async () => {
  const user = await renderGrid()
  await user.click(screen.getByRole('button', { name: 'Next page' }))
  await user.click(screen.getByRole('button', { name: 'Empty group' }))
  expect(screen.queryByRole('heading')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Previous page' })
  ).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Clear filters' }))
  expect(screen.getByText('Page 1 of 5')).toBeInTheDocument()
})

test('group changes with the same models reset the page, and a single model hides navigation', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { models: [] } },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  const firstTwoPages = models.slice(0, 25)
  const grid = (data: PricingModel[], group: string) => (
    <QueryClientProvider client={client}>
      <ModelCardGrid
        models={data}
        selectedGroup={group}
        onModelClick={() => undefined}
      />
    </QueryClientProvider>
  )
  const view = render(grid(firstTwoPages, 'default'))
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Next page' }))
  expect(screen.getByText('Page 2 of 2')).toBeInTheDocument()
  view.rerender(grid(firstTwoPages, 'vip'))
  expect(screen.getByText('Page 1 of 2')).toBeInTheDocument()
  view.rerender(grid(models.slice(0, 1), 'vip'))
  expect(screen.getAllByRole('heading')).toHaveLength(1)
  expect(
    screen.queryByRole('button', { name: 'Next page' })
  ).not.toBeInTheDocument()
  view.rerender(grid(firstTwoPages, 'vip'))
  expect(screen.getByText('Page 1 of 2')).toBeInTheDocument()
})
