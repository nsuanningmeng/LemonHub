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
import type { PropsWithChildren } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import type { UpdateOptionRequest } from '../../types'
import { useSettingsForm } from '../use-settings-form'
import { getOptionValue, useSystemOptions } from '../use-system-options'
import { useUpdateOption } from '../use-update-option'

const savedValues = { SystemName: 'Saved workspace', Footer: 'Saved footer' }
let client: QueryClient

function Wrapper({ children }: PropsWithChildren) {
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

beforeEach(() => {
  client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Infinity },
      mutations: { retry: false },
    },
  })
  client.setQueryData(['system-options'], {
    success: true,
    data: Object.entries(savedValues).map(([key, value]) => ({ key, value })),
  })
})

afterEach(() => {
  cleanup()
  client.clear()
})

test.each(['retry', 'reset'] as const)(
  'after a partially saved form fails, %s uses the latest saved baseline and retains pending edits',
  async (nextAction) => {
    const serverValues = { ...savedValues }
    vi.spyOn(api, 'get').mockImplementation(async () => ({
      data: {
        success: true,
        data: Object.entries(serverValues).map(([key, value]) => ({
          key,
          value,
        })),
      },
    }))
    let rejectFooter: () => void = () => {}
    let shouldRejectFooter = true
    const put = vi
      .spyOn(api, 'put')
      .mockImplementation(async (_url, payload) => {
        const { key, value } = payload as {
          key: keyof typeof serverValues
          value: string
        }
        if (key === 'Footer' && shouldRejectFooter) {
          await new Promise<void>((resolve) => {
            rejectFooter = resolve
          })
          return { data: { success: false, message: 'Footer rejected' } }
        }
        serverValues[key] = value
        return { data: { success: true } }
      })
    const { result } = renderHook(
      () => {
        const options = useSystemOptions()
        const update = useUpdateOption()
        return useSettingsForm({
          defaultValues: getOptionValue(options.data?.data, savedValues),
          onSubmit: async (_data, changes) => {
            for (const [key, value] of Object.entries(changes)) {
              await update.mutateAsync({ key, value: value as string })
            }
          },
        })
      },
      { wrapper: Wrapper }
    )
    act(() => {
      result.current.form.setValue('SystemName', 'Updated workspace', {
        shouldDirty: true,
      })
      result.current.form.setValue('Footer', 'Pending footer', {
        shouldDirty: true,
      })
    })
    let submission: Promise<unknown> = Promise.resolve()
    act(() => {
      submission = result.current.handleSubmit().catch((error) => error)
    })
    await waitFor(() => expect(put).toHaveBeenCalledTimes(2))
    await waitFor(() =>
      expect(result.current.form.formState.defaultValues?.SystemName).toBe(
        'Updated workspace'
      )
    )

    expect(result.current.form.getValues('Footer')).toBe('Pending footer')
    expect(result.current.isDirty).toBe(true)

    await act(async () => {
      rejectFooter()
      expect(await submission).toEqual(new Error('Footer rejected'))
    })
    expect(result.current.form.getValues()).toEqual({
      SystemName: 'Updated workspace',
      Footer: 'Pending footer',
    })
    expect(result.current.isDirty).toBe(true)

    if (nextAction === 'retry') {
      shouldRejectFooter = false
      await act(async () => result.current.handleSubmit())
      expect(
        put.mock.calls.map((call) => (call[1] as UpdateOptionRequest).key)
      ).toEqual(['SystemName', 'Footer', 'Footer'])
      expect(serverValues.Footer).toBe('Pending footer')
    } else {
      act(() => result.current.handleReset())
      expect(result.current.form.getValues()).toEqual({
        SystemName: 'Updated workspace',
        Footer: 'Saved footer',
      })
    }
    expect(result.current.isDirty).toBe(false)
  }
)
