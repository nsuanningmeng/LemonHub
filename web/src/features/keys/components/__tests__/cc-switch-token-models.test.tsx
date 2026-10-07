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
import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { apiKeySchema } from '../../types'
import { CCSwitchDialog } from '../dialogs/cc-switch-dialog'

const clients: QueryClient[] = []
const token = apiKeySchema.parse({
  id: 21,
  name: 'VIP key',
  key: 'masked',
  status: 1,
  remain_quota: 100,
  used_quota: 0,
  unlimited_quota: false,
  expired_time: -1,
  created_time: 0,
  accessed_time: 0,
  group: 'vip',
  model_limits_enabled: true,
  model_limits: 'vip-model',
})
afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
})
function setup() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  const viewFor = (key: string, group = 'vip') => (
    <QueryClientProvider client={client}>
      <CCSwitchDialog
        open
        onOpenChange={() => undefined}
        tokenKey={key}
        token={{ ...token, group }}
      />
    </QueryClientProvider>
  )
  return { viewFor, client }
}
async function openModels() {
  await userEvent.click(screen.getByRole('button', { name: 'Primary Model' }))
}

test('CCSwitch displays only models returned for the actual key and sends token authentication without sharing GET requests', async () => {
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data:
      url === '/v1/models'
        ? { data: [{ id: 'vip-model' }] }
        : { success: true, data: ['default-model', 'vip-model'] },
  }))
  const open = vi.spyOn(window, 'open').mockImplementation(() => null)
  const { viewFor, client } = setup()
  render(viewFor('sk-vip-secret'))
  await openModels()
  await screen.findByRole('option', { name: 'vip-model' })
  expect(
    screen.queryByRole('option', { name: 'default-model' })
  ).not.toBeInTheDocument()
  expect(get).toHaveBeenCalledWith(
    '/v1/models',
    expect.objectContaining({
      headers: { 'x-api-key': 'sk-vip-secret' },
      skipAuthRefresh: true,
      skipErrorHandler: true,
      disableDuplicate: true,
    })
  )
  await userEvent.click(screen.getByRole('option', { name: 'vip-model' }))
  await userEvent.click(screen.getByRole('button', { name: 'Open CC Switch' }))
  expect(open).toHaveBeenCalledTimes(1)
  const imported = new URL(String(open.mock.calls[0][0]))
  expect(imported.searchParams.get('model')).toBe('vip-model')
  expect(imported.searchParams.get('apiKey')).toBe('sk-vip-secret')
  expect(
    JSON.stringify(
      client
        .getQueryCache()
        .getAll()
        .map((query) => query.queryKey)
    )
  ).not.toContain('vip-secret')
})

test('rotating a key with the same token ID ignores the previous delayed response and clears the old model choice', async () => {
  let releaseOld:
    | ((value: { data: { data: { id: string }[] } }) => void)
    | undefined
  const old = new Promise<{ data: { data: { id: string }[] } }>((resolve) => {
    releaseOld = resolve
  })
  vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
    if (url !== '/v1/models') {
      return { data: { success: true, data: ['old-model'] } }
    }
    if (config?.headers?.['x-api-key'] === 'sk-old') return old
    return { data: { data: [{ id: 'new-model' }] } }
  })
  const { viewFor } = setup()
  const view = render(viewFor('sk-old'))
  view.rerender(viewFor('sk-new', 'auto'))
  await openModels()
  await screen.findByRole('option', { name: 'new-model' })
  await act(async () => releaseOld?.({ data: { data: [{ id: 'old-model' }] } }))
  expect(
    screen.queryByRole('option', { name: 'old-model' })
  ).not.toBeInTheDocument()
  await userEvent.click(screen.getByRole('option', { name: 'new-model' }))
  view.rerender(viewFor('sk-third', 'default'))
  await waitFor(() =>
    expect(screen.getByRole('combobox', { name: 'Primary Model' })).toHaveValue(
      ''
    )
  )
})

test('an inaccessible token shows a model-loading error and cannot export a previous selection', async () => {
  vi.spyOn(api, 'get').mockRejectedValue({ response: { status: 403 } })
  const open = vi.spyOn(window, 'open').mockImplementation(() => null)
  const { viewFor } = setup()
  render(viewFor('sk-denied'))
  await screen.findByText(
    'Unable to load models for this API key. Check its permissions and availability.'
  )
  expect(screen.getByRole('button', { name: 'Open CC Switch' })).toBeDisabled()
  expect(open).not.toHaveBeenCalled()
})
