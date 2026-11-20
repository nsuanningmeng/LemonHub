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
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'

import { FILTER_ALL } from '../../constants'
import type { PricingModel } from '../../types'
import { useFilters } from '../use-filters'

const models: PricingModel[] = [
  {
    id: 1,
    vendor_name: 'OpenAI',
    model_name: 'gpt-example',
    model_ratio: 1,
    completion_ratio: 1,
    quota_type: 0,
    enable_groups: ['default'],
  },
  {
    id: 2,
    vendor_name: 'Anthropic',
    model_name: 'claude-example',
    model_ratio: 1,
    completion_ratio: 1,
    quota_type: 0,
    enable_groups: ['default'],
  },
]

function Catalog() {
  const { filteredModels, clearFilters, vendorFilter } = useFilters(models)
  return (
    <>
      <button type='button' onClick={clearFilters}>
        Clear filters
      </button>
      <output aria-label='Selected provider'>{vendorFilter}</output>
      <ul>
        {filteredModels.map((model) => (
          <li key={model.id}>{model.model_name}</li>
        ))}
      </ul>
    </>
  )
}

test('the provider link on the homepage opens a filtered catalog and clearing it restores the full catalog', async () => {
  const root = createRootRoute()
  const pricing = createRoute({
    getParentRoute: () => root,
    path: 'pricing/',
    component: Catalog,
    validateSearch: (search) => ({
      vendor: typeof search.vendor === 'string' ? search.vendor : undefined,
    }),
  })
  const router = createRouter({
    routeTree: root.addChildren([pricing]),
    history: createMemoryHistory({
      initialEntries: ['/pricing?vendor=Anthropic'],
    }),
  })
  await router.load()
  render(<RouterProvider router={router} />)
  expect(await screen.findByLabelText('Selected provider')).toHaveTextContent(
    'Anthropic'
  )
  expect(screen.getByText('claude-example')).toBeVisible()
  expect(screen.queryByText('gpt-example')).not.toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: 'Clear filters' }))
  expect(screen.getByLabelText('Selected provider')).toHaveTextContent(
    FILTER_ALL
  )
  expect(screen.getByText('gpt-example')).toBeVisible()
})
