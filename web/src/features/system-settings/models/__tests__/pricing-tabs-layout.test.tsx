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
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { RatioSettingsCard } from '../ratio-settings-card'

let client: QueryClient
let heading: HTMLHeadingElement
let titleStatus: HTMLSpanElement
const modelDefaults = {
  ModelPrice: '{}',
  ModelRatio: '{}',
  CacheRatio: '{}',
  CreateCacheRatio: '{}',
  CompletionRatio: '{}',
  ImageRatio: '{}',
  AudioRatio: '{}',
  AudioCompletionRatio: '{}',
  ExposeRatioEnabled: false,
  BillingMode: '{}',
  BillingExpr: '{}',
}
const groupDefaults = {
  GroupRatio: '{}',
  TopupGroupRatio: '{}',
  UserUsableGroups: '{}',
  GroupGroupRatio: '{}',
  AutoGroups: '[]',
  MaxTokenAutoGroups: 5,
  DefaultUseAutoGroup: false,
  GroupSpecialUsableGroup: '{}',
}

beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: [] } })
  heading = document.createElement('h1')
  heading.textContent = 'Model Pricing'
  titleStatus = document.createElement('span')
  heading.append(titleStatus)
  document.body.append(heading)
})

afterEach(() => {
  cleanup()
  client.clear()
  heading.remove()
})

test('pricing sections remain outside the page heading and all tabs can be selected', async () => {
  render(
    <QueryClientProvider client={client}>
      <SettingsPageProvider
        actionsContainer={null}
        titleStatusContainer={titleStatus}
      >
        <RatioSettingsCard
          titleKey='Model Pricing'
          modelDefaults={modelDefaults}
          groupDefaults={groupDefaults}
          toolPricesDefault='{}'
          visibleTabs={[
            'models',
            'unset-models',
            'tool-prices',
            'upstream-sync',
          ]}
        />
      </SettingsPageProvider>
    </QueryClientProvider>
  )
  const tabs = screen.getByRole('tablist')
  expect(tabs.closest('h1')).toBeNull()
  expect(screen.getAllByRole('tab')).toHaveLength(4)
  await userEvent
    .setup()
    .click(screen.getByRole('tab', { name: 'Upstream price sync' }))
  expect(
    screen.getByRole('tab', { name: 'Upstream price sync' })
  ).toHaveAttribute('aria-selected', 'true')
})
