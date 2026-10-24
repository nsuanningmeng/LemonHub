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
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import type { Model } from '../../../types'
import { ModelMutateDrawer } from '../model-mutate-drawer'

const clients: QueryClient[] = []
afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
})

test.each([undefined, 0])(
  'switching from a model with a vendor to vendor_id %s clears the selection and preserves pricing',
  async (vendorId) => {
    const first: Model = {
      id: 7,
      model_name: 'vendor-model',
      vendor_id: 3,
      status: 1,
      sync_official: 1,
      created_time: 1,
      updated_time: 1,
      name_rule: 0,
    }
    const second = {
      ...first,
      id: 8,
      model_name: 'unassigned-model',
      vendor_id: vendorId,
    }
    let resolveDetail!: (value: {
      data: { success: boolean; data: Model }
    }) => void
    const detail = new Promise<{ data: { success: boolean; data: Model } }>(
      (resolve) => {
        resolveDetail = resolve
      }
    )
    const get = vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (url === '/api/models/7') {
        return { data: { success: true, data: first } }
      }
      if (url === '/api/models/8') return detail
      if (url === '/api/vendors/') {
        return {
          data: {
            success: true,
            data: { items: [{ id: 3, name: 'Existing vendor' }] },
          },
        }
      }
      if (url === '/api/option/') {
        return {
          data: {
            success: true,
            data: [
              {
                key: 'ModelRatio',
                value: '{"unassigned-model":2,"other-model":8}',
              },
              { key: 'CompletionRatio', value: '{"unassigned-model":3}' },
            ],
          },
        }
      }
      throw new Error(`Unexpected GET ${url}`)
    })
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })
    clients.push(client)
    const close = vi.fn()
    const tree = (currentRow: Model, open: boolean) => (
      <QueryClientProvider client={client}>
        <ModelMutateDrawer
          open={open}
          currentRow={currentRow}
          onOpenChange={close}
        />
      </QueryClientProvider>
    )
    const view = render(tree(first, true))
    await screen.findByDisplayValue('vendor-model')
    await waitFor(() =>
      expect(
        screen.getByRole('combobox', { name: 'Vendor' })
      ).toHaveTextContent('Existing vendor')
    )
    view.rerender(tree(first, false))
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    view.rerender(tree(second, true))
    await waitFor(() => expect(get).toHaveBeenCalledWith('/api/models/8'))
    expect
      .soft(screen.getByRole('button', { name: 'Update Model' }))
      .toBeDisabled()
    await act(async () =>
      resolveDetail({ data: { success: true, data: second } })
    )
    await screen.findByDisplayValue('unassigned-model')
    expect(screen.getByRole('combobox', { name: 'Vendor' })).toHaveTextContent(
      'Select vendor'
    )
    fireEvent.click(screen.getByRole('button', { name: 'Update Model' }))
    await waitFor(() =>
      expect(put).toHaveBeenCalledWith(
        '/api/models/',
        expect.objectContaining({ id: 8, vendor_id: 0 })
      )
    )
    await waitFor(() => expect(close).toHaveBeenCalledWith(false))
    for (const [url, payload] of put.mock.calls) {
      if (url !== '/api/option/') continue
      const update = payload as { key: string; value: string }
      if (update.key === 'ModelRatio') {
        expect(JSON.parse(update.value)).toEqual({
          'unassigned-model': 2,
          'other-model': 8,
        })
      }
      if (update.key === 'CompletionRatio') {
        expect(JSON.parse(update.value)).toEqual({ 'unassigned-model': 3 })
      }
    }
  },
  30000
)
