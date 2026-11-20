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
import { render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import { formatQuota } from '@/lib/format'

import { ReferralAdminSummaryCards } from '../referral-admin-summary-cards'
import { ReferralStatsCards } from '../referral-stats-cards'

const largeAmount = 8_798_765_432_123_456

test('referral statistics preserve every digit of long balances and invite counts', () => {
  render(
    <ReferralStatsCards
      affiliateLink='https://example.test/r/fixture'
      loading={false}
      onTransfer={() => {}}
      stats={{
        aff_code: 'fixture',
        pending_quota: largeAmount,
        total_earned_quota: 0,
        total_invited: largeAmount,
        activated_count: 0,
        commission_percent: 5,
        month_commission_quota: 0,
      }}
    />
  )

  for (const value of [
    formatQuota(largeAmount),
    largeAmount.toLocaleString(),
  ]) {
    expect(screen.getByTitle(value)).toHaveTextContent(value)
    expect(screen.getByTitle(value)).toHaveClass('wrap-anywhere')
  }
})

test('administrator commission labels and their complete values can wrap on small screens', () => {
  const client = new QueryClient({
    defaultOptions: { queries: { staleTime: Infinity } },
  })
  client.setQueryData(['referral', 'admin', 'summary'], {
    total_commission_paid: largeAmount,
  })
  const view = render(
    <QueryClientProvider client={client}>
      <ReferralAdminSummaryCards />
    </QueryClientProvider>
  )

  const value = formatQuota(largeAmount)
  expect(screen.getByText('Total Commission Paid')).toHaveClass('wrap-anywhere')
  expect(screen.getByTitle(value)).toHaveTextContent(value)
  expect(screen.getByTitle(value)).toHaveClass('wrap-anywhere')
  view.unmount()
  client.clear()
})
