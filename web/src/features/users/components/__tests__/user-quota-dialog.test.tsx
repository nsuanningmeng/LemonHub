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
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { UserQuotaDialog } from '../user-quota-dialog'

const originalConfig = useSystemConfigStore.getState()

beforeEach(() => {
  localStorage.clear()
  useSystemConfigStore
    .getState()
    .setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG } })
})

afterEach(() => {
  useSystemConfigStore.setState(originalConfig, true)
  localStorage.clear()
})

it.each(['click', 'enter', 'mixed'] as const)(
  'allows only one quota request while rapid %s submissions are pending',
  async (method) => {
    let resolveResult: (value: { success: boolean }) => void = () => {}
    const result = new Promise<{ success: boolean }>((resolve) => {
      resolveResult = resolve
    })
    const post = vi
      .spyOn(api, 'post')
      .mockImplementation(async () => ({ data: await result }))
    const onSuccess = vi.fn()
    const onOpenChange = vi.fn()
    const user = userEvent.setup()
    render(
      <UserQuotaDialog
        open
        onOpenChange={onOpenChange}
        userId={7}
        currentQuota={5000000}
        onSuccess={onSuccess}
      />
    )
    const input = screen.getByRole('spinbutton')
    const confirm = screen.getByRole('button', { name: 'Confirm' })
    await user.type(input, '2')

    act(() => {
      if (method === 'enter') fireEvent.keyDown(input, { key: 'Enter' })
      else fireEvent.click(confirm)
      if (method === 'click') fireEvent.click(confirm)
      else fireEvent.keyDown(input, { key: 'Enter' })
    })

    await screen.findByRole('button', { name: 'Processing...' })
    expect(post).toHaveBeenCalledTimes(1)
    expect(post).toHaveBeenCalledWith('/api/user/manage', {
      id: 7,
      action: 'add_quota',
      mode: 'add',
      value: 1000000,
    })
    expect(input).toBeDisabled()
    for (const mode of ['Add', 'Subtract', 'Override']) {
      expect(screen.getByRole('button', { name: mode })).toBeDisabled()
    }
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(post).toHaveBeenCalledTimes(1)

    await act(async () => {
      resolveResult({ success: true })
    })
    expect(onSuccess).toHaveBeenCalledTimes(1)
    expect(onOpenChange).toHaveBeenCalledWith(false)
  }
)

it('retains the amount and mode after failure and allows a successful retry', async () => {
  let resolveResult: (value: {
    success: boolean
    message?: string
  }) => void = () => {}
  const result = new Promise<{ success: boolean; message?: string }>(
    (resolve) => {
      resolveResult = resolve
    }
  )
  const post = vi
    .spyOn(api, 'post')
    .mockImplementationOnce(async () => ({ data: await result }))
    .mockResolvedValueOnce({ data: { success: true } })
  const onSuccess = vi.fn()
  const user = userEvent.setup()
  render(
    <UserQuotaDialog
      open
      onOpenChange={vi.fn()}
      userId={7}
      currentQuota={5000000}
      onSuccess={onSuccess}
    />
  )
  await user.click(screen.getByRole('button', { name: 'Subtract' }))
  await user.type(screen.getByRole('spinbutton'), '3')
  await user.click(screen.getByRole('button', { name: 'Confirm' }))

  await act(async () => {
    resolveResult({ success: false, message: 'Please retry' })
  })
  expect(screen.getByRole('spinbutton')).toHaveValue(3)
  expect(screen.getByRole('spinbutton')).toBeEnabled()
  await user.click(screen.getByRole('button', { name: 'Confirm' }))

  await waitFor(() => expect(onSuccess).toHaveBeenCalledTimes(1))
  expect(post).toHaveBeenCalledTimes(2)
  expect(post).toHaveBeenLastCalledWith('/api/user/manage', {
    id: 7,
    action: 'add_quota',
    mode: 'subtract',
    value: 1500000,
  })
})
