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
import userEvent from '@testing-library/user-event'
import { toast } from 'sonner'
import { expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import type { UpdateOptionRequest } from '../../types'
import { RatioSettingsCard } from '../ratio-settings-card'

test('a rejected group save retains the editable value, reports one error, and can be retried', async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const actions = document.createElement('div')
  document.body.append(actions)
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValueOnce({
      data: { success: false, message: 'Group limit rejected' },
    })
    .mockResolvedValue({ data: { success: true } })
  const error = vi.spyOn(toast, 'error')
  const user = userEvent.setup()
  const view = render(
    <QueryClientProvider client={client}>
      <SettingsPageProvider actionsContainer={actions}>
        <RatioSettingsCard
          modelDefaults={{
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
          }}
          groupDefaults={{
            GroupRatio: '{}',
            TopupGroupRatio: '{}',
            UserUsableGroups: '{}',
            GroupGroupRatio: '{}',
            AutoGroups: '[]',
            MaxTokenAutoGroups: 5,
            DefaultUseAutoGroup: false,
            GroupSpecialUsableGroup: '{}',
          }}
          toolPricesDefault='{}'
          visibleTabs={['groups']}
        />
      </SettingsPageProvider>
    </QueryClientProvider>
  )
  const input = screen.getByRole('spinbutton', {
    name: 'Maximum custom groups per token',
  })
  fireEvent.change(input, { target: { value: '8' } })
  await user.click(screen.getByRole('button', { name: 'Save group ratios' }))

  await waitFor(() =>
    expect(error).toHaveBeenCalledExactlyOnceWith('Group limit rejected')
  )
  const save = await screen.findByRole('button', { name: 'Save group ratios' })
  expect(save).toBeEnabled()
  expect(input).toHaveValue(8)
  expect(input).toBeEnabled()

  await user.click(save)
  await waitFor(() => expect(put).toHaveBeenCalledTimes(2))
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Save group ratios' })
    ).toBeEnabled()
  )
  expect(put.mock.calls[1][1] as UpdateOptionRequest).toEqual({
    key: 'MaxTokenAutoGroups',
    value: 8,
  })
  expect(error).toHaveBeenCalledTimes(1)
  view.unmount()
  client.clear()
  actions.remove()
})
