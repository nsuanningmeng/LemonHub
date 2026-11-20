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
import { act, render, screen } from '@testing-library/react'
import { expect, test, vi } from 'vitest'

import { getUserInfo } from '../../../api'
import { UserInfoDialog } from '../user-info-dialog'

vi.mock('../../../api', () => ({ getUserInfo: vi.fn() }))

test('a delayed response from the previously selected user cannot replace the current details', async () => {
  type UserResult = Awaited<ReturnType<typeof getUserInfo>>
  let finishFirst!: (value: UserResult) => void
  const first = new Promise<UserResult>((resolve) => {
    finishFirst = resolve
  })
  const second = {
    id: 2,
    username: 'current-user',
    quota: 0,
    used_quota: 0,
    request_count: 0,
  }
  vi.mocked(getUserInfo)
    .mockReturnValueOnce(first)
    .mockResolvedValueOnce({ success: true, data: second })
  const props = { open: true, onOpenChange: vi.fn() }
  const { rerender } = render(<UserInfoDialog userId={1} {...props} />)
  rerender(<UserInfoDialog userId={2} {...props} />)
  expect(await screen.findByText('current-user')).toBeVisible()
  await act(async () => {
    finishFirst({
      success: true,
      data: { ...second, id: 1, username: 'previous-user' },
    })
    await first
  })
  expect(screen.getByText('current-user')).toBeVisible()
  expect(screen.queryByText('previous-user')).not.toBeInTheDocument()
})
