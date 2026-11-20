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
import { getCoreRowModel, useReactTable } from '@tanstack/react-table'
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { DataTableView } from '@/components/data-table'
import { api } from '@/lib/api'

import { apiKeySchema } from '../../types'
import { useApiKeysColumns } from '../api-keys-columns'
import { ApiKeysProvider, useApiKeys } from '../api-keys-provider'

const keys = [
  apiKeySchema.parse({
    id: 1,
    name: 'Production',
    key: 'masked-production-key',
    status: 1,
    remain_quota: 100,
    used_quota: 0,
    unlimited_quota: false,
    expired_time: -1,
    created_time: 1,
    accessed_time: 1,
    model_limits_enabled: false,
  }),
]
const clients: QueryClient[] = []

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
})

function KeyTable(props: { now: number }) {
  // The production table subscribes to the same provider for its refresh state.
  useApiKeys()
  const columns = useApiKeysColumns(props.now)
  const table = useReactTable({
    columns,
    data: keys,
    getCoreRowModel: getCoreRowModel(),
  })
  return <DataTableView table={table} />
}

function KeyTablePage(props: { client: QueryClient; now: number }) {
  return (
    <QueryClientProvider client={props.client}>
      <ApiKeysProvider>
        <KeyTable now={props.now} />
      </ApiKeysProvider>
    </QueryClientProvider>
  )
}

function prepareKeyRequest() {
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: {} } })
  type KeyResponse = {
    data: { success: boolean; data: { key: string } }
  }
  let resolve!: (response: KeyResponse) => void
  const promise = new Promise<KeyResponse>((resolveResponse) => {
    resolve = resolveResponse
  })
  vi.spyOn(api, 'post').mockReturnValue(promise)
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  client.setQueryData(['status'], { success: true, data: {} })
  clients.push(client)
  return { client, response: { promise, resolve } }
}

it('keeps the first key popover open while resolving the key and updating the table clock', async () => {
  const user = userEvent.setup()
  const { client, response } = prepareKeyRequest()
  const now = 1_800_000_000_000
  const view = render(<KeyTablePage client={client} now={now} />)

  await user.click(
    screen.getByRole('button', { name: 'sk-masked-production-key' })
  )
  expect(screen.getByText('Full API Key')).toBeVisible()
  expect(screen.getByText('Loading...')).toBeVisible()
  await act(async () => {
    response.resolve({
      data: { success: true, data: { key: 'resolved-production-key' } },
    })
    await response.promise
  })

  const key = await screen.findByRole('textbox', { name: 'Full API Key' })
  expect(key).toHaveValue('sk-resolved-production-key')
  view.rerender(<KeyTablePage client={client} now={now + 30_000} />)
  expect(screen.getByRole('textbox', { name: 'Full API Key' })).toBeVisible()
  expect(screen.getByRole('textbox', { name: 'Full API Key' })).toHaveValue(
    'sk-resolved-production-key'
  )
})

it('keeps the first row menu open through key loading and table clock updates so its key can be copied', async () => {
  const user = userEvent.setup()
  const { client, response } = prepareKeyRequest()
  const now = 1_800_000_000_000
  const view = render(<KeyTablePage client={client} now={now} />)

  await user.click(screen.getByRole('button', { name: 'Open menu' }))
  expect(screen.getByRole('menuitem', { name: 'Copy Key' })).toBeVisible()
  await act(async () => {
    response.resolve({
      data: { success: true, data: { key: 'resolved-production-key' } },
    })
    await response.promise
  })
  view.rerender(<KeyTablePage client={client} now={now + 30_000} />)
  await user.click(screen.getByRole('menuitem', { name: 'Copy Key' }))
  expect(await navigator.clipboard.readText()).toBe(
    'sk-resolved-production-key'
  )
})
