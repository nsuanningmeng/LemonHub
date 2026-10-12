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
import { render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import type { UserProfile } from '../../types'
import { ProfileHeader } from '../profile-header'

const profile: UserProfile = {
  id: 2147483647,
  username: 'long_user_name_for_small_mobile_screens',
  display_name: 'A long display name for a small screen',
  role: 1,
  email: 'very.long.account.address@example.invalid',
  group: 'long-group-identifier',
  quota: 15000000,
  used_quota: 2400000,
  request_count: 842,
  status: 1,
  aff_count: 0,
  aff_quota: 0,
  aff_history_quota: 0,
  created_time: 1735689600,
}

test('a long display name and account badges can occupy separate rows on a narrow screen', () => {
  render(<ProfileHeader profile={profile} loading={false} />)
  const heading = screen.getByRole('heading', { name: profile.display_name })
  expect(heading).toHaveAttribute('title', profile.display_name)
  expect(heading.parentElement).toHaveClass('flex-wrap')
  expect(
    screen.getByText('User ID 2147483647').closest('[data-slot="status-badge"]')
  ).toHaveClass('shrink-0')
})

test('mobile account statistics keep labels readable and expose exact truncated balances', () => {
  render(<ProfileHeader profile={profile} loading={false} />)
  const label = screen.getByText('Current Balance')
  expect(label).not.toHaveClass('truncate')
  expect(label).toHaveClass('sm:truncate')
  const balance = screen.getByText('$30')
  expect(balance).toHaveAttribute('title', '$30')
})
