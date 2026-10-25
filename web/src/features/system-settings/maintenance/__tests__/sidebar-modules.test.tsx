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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import {
  parseSidebarModulesAdmin,
  serializeSidebarModulesAdmin,
} from '../config'
import { SidebarModulesSection } from '../sidebar-modules-section'

let client: QueryClient
let actions: HTMLDivElement

afterEach(() => {
  client?.clear()
  actions?.remove()
})

function renderSettings() {
  client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
  actions = document.createElement('div')
  document.body.append(actions)
  const config = parseSidebarModulesAdmin(
    JSON.stringify({ personal: { enabled: true }, admin: { enabled: true } })
  )
  render(
    <QueryClientProvider client={client}>
      <SettingsPageProvider actionsContainer={actions}>
        <SidebarModulesSection
          config={config}
          initialSerialized={serializeSidebarModulesAdmin(config)}
        />
      </SettingsPageProvider>
    </QueryClientProvider>
  )
}

test('legacy sidebar configuration exposes both reward switches and saves their disabled values', async () => {
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true } })
  renderSettings()
  const personalRewards = screen.getByRole('switch', { name: 'My Rewards' })
  const adminRewards = screen.getByRole('switch', { name: 'Reward Center' })
  expect(personalRewards).toBeChecked()
  expect(adminRewards).toBeChecked()

  fireEvent.click(personalRewards)
  fireEvent.click(adminRewards)
  fireEvent.click(screen.getByRole('button', { name: 'Save sidebar modules' }))

  await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
  const payload = put.mock.calls[0]?.[1] as { key: string; value: string }
  expect(payload.key).toBe('SidebarModulesAdmin')
  expect(JSON.parse(payload.value)).toMatchObject({
    personal: { rewards: false },
    admin: { reward_campaigns: false },
  })
})

test('disabling a sidebar section disables its reward switch until the section is enabled again', () => {
  renderSettings()
  const section = screen.getByRole('switch', { name: 'Personal area' })
  const rewards = screen.getByRole('switch', { name: 'My Rewards' })
  fireEvent.click(section)
  expect(rewards).toHaveAttribute('aria-disabled', 'true')
  expect(rewards).toBeChecked()
  fireEvent.click(rewards)
  expect(rewards).toBeChecked()
  fireEvent.click(section)
  expect(rewards).not.toHaveAttribute('aria-disabled', 'true')
})
