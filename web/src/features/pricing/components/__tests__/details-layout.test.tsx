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
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { ModelDetailsContent } from '../model-details'

// Canvas charts are outside jsdom; this regression exercises the real detail layout.
vi.mock('@visactor/react-vchart', () => ({ VChart: () => null }))
vi.mock('@visactor/vchart', () => ({
  ThemeManager: { setCurrentTheme: () => undefined },
}))

afterEach(cleanup)

test('an unbroken model identifier wraps while its copy action remains reachable in narrow details', () => {
  const client = new QueryClient({
    defaultOptions: { queries: { enabled: false, retry: false } },
  })
  const name = `deployment_${'custommodelidentifierwithlargereasoningcontext'.repeat(3)}`
  render(
    <QueryClientProvider client={client}>
      <ModelDetailsContent
        model={{
          id: 1,
          model_name: name,
          quota_type: 0,
          model_ratio: 1,
          completion_ratio: 3,
          enable_groups: ['default'],
        }}
        groupRatio={{ default: 1 }}
        usableGroup={{ default: { desc: 'Default', ratio: 1 } }}
        endpointMap={{}}
        autoGroups={[]}
        priceRate={1}
        usdExchangeRate={1}
        tokenUnit='M'
      />
    </QueryClientProvider>
  )

  expect(screen.getByRole('heading', { name })).toHaveClass(
    'min-w-0',
    'break-all'
  )
  expect(screen.getByRole('button', { name: 'Copy model name' })).toHaveClass(
    'shrink-0'
  )
  client.clear()
})
