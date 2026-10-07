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
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { getApiKeys, searchApiKeys } from '../../api'
import { apiKeySchema, getApiKeyEffectiveStatus } from '../../types'

afterEach(() => vi.restoreAllMocks())
it('sends the same single status through list and combined paginated search; absence means all', async () => {
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValue({ data: { success: true } })
  await getApiKeys({ p: 2, size: 20, status: 4 })
  await searchApiKeys({
    keyword: 'team & A',
    token: 'secret+',
    p: 1,
    size: 20,
    status: 3,
  })
  await getApiKeys({ p: 1, size: 20 })
  expect(get.mock.calls[0][0]).toBe('/api/token/?p=2&size=20&status=4')
  const query = new URL(get.mock.calls[1][0], 'http://localhost').searchParams
  expect(Object.fromEntries(query)).toEqual({
    keyword: 'team & A',
    token: 'secret+',
    p: '1',
    size: '20',
    status: '3',
  })
  expect(get.mock.calls[2][0]).toBe('/api/token/?p=1&size=20')
})
it('uses server effective status without changing the stored toggle intent and supports older responses', () => {
  const stored = { status: 1, effective_status: 3 }
  expect(
    getApiKeyEffectiveStatus(
      stored as Parameters<typeof getApiKeyEffectiveStatus>[0]
    )
  ).toBe(3)
  expect(stored.status).toBe(1)
  expect(
    getApiKeyEffectiveStatus({ status: 2 } as Parameters<
      typeof getApiKeyEffectiveStatus
    >[0])
  ).toBe(2)
  expect(apiKeySchema.shape.effective_status.parse(undefined)).toBeUndefined()
})
