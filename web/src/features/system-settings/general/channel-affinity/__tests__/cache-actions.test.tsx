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
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../../components/settings-page-context'
import { ChannelAffinitySection } from '../index'
import type { AffinityRule, ChannelAffinitySettings } from '../types'

let client: QueryClient
let actions: HTMLDivElement
const rule: AffinityRule = {
  name: 'saved-rule',
  model_regex: ['.*'],
  path_regex: [],
  key_sources: [],
  ttl_seconds: 60,
  skip_retry_on_failure: false,
  include_using_group: true,
  include_model_name: true,
  include_rule_name: true,
}
const defaults: ChannelAffinitySettings = {
  'channel_affinity_setting.enabled': true,
  'channel_affinity_setting.switch_on_success': false,
  'channel_affinity_setting.keep_on_channel_disabled': false,
  'channel_affinity_setting.max_entries': 100,
  'channel_affinity_setting.default_ttl_seconds': 60,
  'channel_affinity_setting.rules': JSON.stringify([
    rule,
    { ...rule, name: 'shared-rule', include_rule_name: false },
    { ...rule, name: 'unsaved-rule' },
  ]),
}

function renderSettings() {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  actions = document.createElement('div')
  document.body.append(actions)
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        total: 2,
        by_rule_name: { 'saved-rule': 2 },
        cache_capacity: 100,
      },
    },
  })
  render(
    <QueryClientProvider client={client}>
      <SettingsPageProvider actionsContainer={actions}>
        <ChannelAffinitySection defaultValues={defaults} />
      </SettingsPageProvider>
    </QueryClientProvider>
  )
  return { get, user: userEvent.setup() }
}

afterEach(() => {
  client?.clear()
  actions?.remove()
  vi.restoreAllMocks()
})

test('refresh only reads statistics and only saved independent rules can be cleared', async () => {
  const remove = vi
    .spyOn(api, 'delete')
    .mockResolvedValue({ data: { success: true } })
  const { get, user } = renderSettings()
  const clear = await screen.findByRole('button', {
    name: 'Clear cache for saved-rule',
  })
  await waitFor(() => expect(clear).toBeEnabled())
  expect(
    screen.getByRole('button', { name: 'Clear cache for shared-rule' })
  ).toBeDisabled()
  expect(
    screen.getByRole('button', { name: 'Clear cache for unsaved-rule' })
  ).toBeDisabled()
  await user.click(
    screen.getByRole('button', { name: 'Refresh cache statistics' })
  )
  await waitFor(() => expect(get).toHaveBeenCalledTimes(2))
  expect(remove).not.toHaveBeenCalled()
  await user.click(clear)
  const dialog = screen.getByRole('dialog', {
    name: 'Confirm clearing cache for this rule',
  })
  expect(
    within(dialog).getByText(/Subsequent requests will select channels again/)
  ).toBeVisible()
  await user.click(within(dialog).getByRole('button', { name: 'Clear cache' }))
  await waitFor(() =>
    expect(remove).toHaveBeenCalledExactlyOnceWith(
      '/api/option/channel_affinity_cache',
      { params: { rule_name: 'saved-rule' } }
    )
  )
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
  expect(get).toHaveBeenCalledTimes(3)
})

test('failed cache clear keeps confirmation open and prevents duplicate requests while pending', async () => {
  let finish!: (value: unknown) => void
  const remove = vi.spyOn(api, 'delete').mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve
      })
  )
  const { user } = renderSettings()
  await user.click(screen.getByRole('button', { name: 'Clear All Cache' }))
  const dialog = screen.getByRole('dialog', {
    name: 'Confirm clearing all channel affinity cache',
  })
  const confirm = within(dialog).getByRole('button', {
    name: 'Clear All Cache',
  })
  await user.click(confirm)
  expect(confirm).toBeDisabled()
  expect(within(dialog).getByRole('button', { name: 'Cancel' })).toBeDisabled()
  await user.click(confirm)
  expect(remove).toHaveBeenCalledTimes(1)
  await act(() => finish({ data: { success: false } }))
  expect(dialog).toBeVisible()
  expect(confirm).toBeEnabled()
})

test('unavailable statistics disable per-rule clearing until a successful refresh', async () => {
  const { get, user } = renderSettings()
  const clear = await screen.findByRole('button', {
    name: 'Clear cache for saved-rule',
  })
  await waitFor(() => expect(clear).toBeEnabled())
  get.mockResolvedValueOnce({ data: { success: false } })
  await user.click(
    screen.getByRole('button', { name: 'Refresh cache statistics' })
  )
  await waitFor(() => expect(clear).toBeDisabled())
  await user.click(
    screen.getByRole('button', { name: 'Refresh cache statistics' })
  )
  await waitFor(() => expect(clear).toBeEnabled())
})
