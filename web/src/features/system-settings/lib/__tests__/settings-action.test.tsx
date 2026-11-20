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
import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import { AxiosError, CanceledError } from 'axios'
import type { PropsWithChildren } from 'react'
import { toast } from 'sonner'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { useUpdateOption } from '../../hooks/use-update-option'
import {
  markSettingsErrorReported,
  runSettingsAction,
} from '../settings-action'

let client: QueryClient

function Wrapper({ children }: PropsWithChildren) {
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
})

afterEach(() => {
  cleanup()
  client.clear()
})

test.each(['business', 'transport'])(
  'a reported %s rejection is consumed at the UI boundary with one error',
  async (kind) => {
    const put = vi.spyOn(api, 'put')
    if (kind === 'business') {
      put.mockResolvedValue({ data: { success: false, message: 'Rejected' } })
    } else {
      put.mockRejectedValue(new AxiosError('Connection lost'))
    }
    const notify = vi.spyOn(toast, 'error')
    const { result } = renderHook(useUpdateOption, { wrapper: Wrapper })

    await act(async () => {
      await runSettingsAction(() =>
        result.current.mutateAsync({ key: 'SystemName', value: 'Draft' })
      )
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(notify).toHaveBeenCalledTimes(1)
    expect(notify).toHaveBeenCalledWith(
      kind === 'business' ? 'Rejected' : 'Connection lost'
    )
  }
)

test('a cancelled settings request stays rejected without displaying an error on the next session', async () => {
  const cancelled = new CanceledError('Authentication session changed')
  vi.spyOn(api, 'put').mockRejectedValue(cancelled)
  const notify = vi.spyOn(toast, 'error')
  const { result } = renderHook(useUpdateOption, { wrapper: Wrapper })

  await act(async () => {
    await runSettingsAction(() =>
      result.current.mutateAsync({ key: 'SystemName', value: 'Draft' })
    )
  })

  await waitFor(() => expect(result.current.error).toBe(cancelled))
  expect(notify).not.toHaveBeenCalled()
})

test('an unexpected programming error remains observable even if reported, and handlers run synchronously', async () => {
  const error = new TypeError('Unexpected form state')
  markSettingsErrorReported(error)
  let called = false
  const result = runSettingsAction(() => {
    called = true
    throw error
  })
  expect(called).toBe(true)
  await expect(result).rejects.toBe(error)
})
