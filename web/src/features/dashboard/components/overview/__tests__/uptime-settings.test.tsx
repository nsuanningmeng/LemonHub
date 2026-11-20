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
import { act, cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AxiosError } from 'axios'
import { toast } from 'sonner'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { UptimePanel } from '../uptime-panel'

const originalAdapter = api.defaults.adapter
let client: QueryClient
const getAnimationsDescriptor = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'getAnimations'
)

beforeEach(() => {
  Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
})

afterEach(() => {
  cleanup()
  client.clear()
  api.defaults.adapter = originalAdapter
  if (getAnimationsDescriptor) {
    Object.defineProperty(
      HTMLElement.prototype,
      'getAnimations',
      getAnimationsDescriptor
    )
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, 'getAnimations')
  }
})

test('saving uptime group settings refreshes the mounted monitor list through query invalidation', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValueOnce({
    data: {
      success: true,
      data: [
        {
          categoryName: 'Primary',
          monitors: [{ name: 'Old monitor', uptime: 0.99, status: 1 }],
        },
      ],
    },
  })
  render(
    <QueryClientProvider client={client}>
      <UptimePanel />
    </QueryClientProvider>
  )
  expect(await screen.findByText('Old monitor')).toBeVisible()
  get.mockResolvedValue({
    data: {
      success: true,
      data: [
        {
          categoryName: 'Secondary',
          monitors: [{ name: 'New monitor', uptime: 1, status: 1 }],
        },
      ],
    },
  })
  await act(async () => {
    await client.invalidateQueries({ queryKey: ['uptime-status'] })
  })
  expect(await screen.findByText('New monitor')).toBeVisible()
  expect(screen.queryByText('Old monitor')).not.toBeInTheDocument()
})

test('failed service-status requests report the error and allow retry instead of claiming no configuration', async () => {
  const get = vi.spyOn(api, 'get').mockRejectedValue(new Error('Unavailable'))
  const user = userEvent.setup()
  render(
    <QueryClientProvider client={client}>
      <UptimePanel />
    </QueryClientProvider>
  )
  expect(await screen.findByText('Failed to load')).toBeVisible()
  expect(
    screen.queryByText('No uptime monitoring configured')
  ).not.toBeInTheDocument()
  get.mockResolvedValue({ data: { success: true, data: [] } })
  await user.click(screen.getByRole('button', { name: 'Refresh' }))
  expect(
    await screen.findByText('No uptime monitoring configured')
  ).toBeVisible()
})

test('an HTTP 500 remains in the uptime panel and retries without global navigation or duplicate notifications', async () => {
  const notify = vi.spyOn(toast, 'error')
  let recovered = false
  let failure: AxiosError | undefined
  api.defaults.adapter = async (config) => {
    if (!recovered) {
      failure = new AxiosError(
        'Unavailable',
        'ERR_BAD_RESPONSE',
        config,
        undefined,
        {
          status: 500,
          statusText: 'Server error',
          headers: {},
          config,
          data: { message: 'Unavailable' },
        }
      )
      throw failure
    }
    return {
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
      data: { success: true, data: [] },
    }
  }
  const user = userEvent.setup()
  render(
    <QueryClientProvider client={client}>
      <UptimePanel />
    </QueryClientProvider>
  )
  expect(await screen.findByText('Failed to load')).toBeVisible()
  const queryError = client.getQueryState(['uptime-status'])?.error
  expect(queryError).not.toBeInstanceOf(AxiosError)
  expect(queryError?.cause).toBe(failure)
  expect(notify).not.toHaveBeenCalled()
  recovered = true
  await user.click(screen.getByRole('button', { name: 'Refresh' }))
  expect(
    await screen.findByText('No uptime monitoring configured')
  ).toBeVisible()
  expect(notify).not.toHaveBeenCalled()
})
