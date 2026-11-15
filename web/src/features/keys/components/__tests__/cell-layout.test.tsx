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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { apiKeySchema } from '../../types'
import { ApiKeyCell } from '../api-keys-cells'
import { ApiKeysProvider } from '../api-keys-provider'

const key = apiKeySchema.parse({
  id: 1,
  name: 'Production',
  key: 'long-masked-key-value',
  status: 1,
  remain_quota: 100,
  used_quota: 0,
  unlimited_quota: false,
  expired_time: -1,
  created_time: 1,
  accessed_time: 1,
  model_limits_enabled: false,
})
afterEach(() => vi.restoreAllMocks())

it('shrinks the masked key to leave room for copying the resolved key', async () => {
  const user = userEvent.setup()
  vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, data: { key: 'resolved-workspace-key' } },
  })
  render(
    <ApiKeysProvider>
      <ApiKeyCell apiKey={key} />
    </ApiKeysProvider>
  )
  const value = screen.getByRole('button', { name: 'sk-long-masked-key-value' })
  expect(value).toHaveClass('shrink')
  expect(value).not.toHaveClass('shrink-0')
  await user.click(screen.getByRole('button', { name: 'Copy API key' }))
  await waitFor(async () =>
    expect(await navigator.clipboard.readText()).toBe(
      'sk-resolved-workspace-key'
    )
  )
})
