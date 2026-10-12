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
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { BillingHistoryDialog } from '../dialogs/billing-history-dialog'
import { WalletStatsCard } from '../wallet-stats-card'

const { copyToClipboard } = vi.hoisted(() => ({ copyToClipboard: vi.fn() }))

vi.mock('@/hooks/use-copy-to-clipboard', () => ({
  useCopyToClipboard: () => ({ copiedText: '', copyToClipboard }),
}))

afterEach(() => {
  vi.restoreAllMocks()
  copyToClipboard.mockClear()
})

test('wallet statistics preserve complete large amounts and expose them as accessible titles', () => {
  render(
    <WalletStatsCard
      user={{
        id: 1,
        username: 'layout-fixture',
        group: 'default',
        quota: 987654321000,
        used_quota: 234567890,
        request_count: 1234567,
        aff_quota: 0,
        aff_history_quota: 0,
        aff_count: 0,
      }}
    />
  )

  expect(screen.getByText('$1,975,308.64')).toHaveAttribute(
    'title',
    '$1,975,308.64'
  )
  expect(screen.getByText('1,234,567')).toHaveAttribute('title', '1,234,567')
})

test('a shortened billing order remains readable in full and copies the original identifier', async () => {
  const tradeNo =
    'fixture-order-production-20261012-abcdefghijklmnop-1234567890'
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        total: 1,
        items: [
          {
            id: 1,
            user_id: 1,
            amount: 20,
            money: 20,
            trade_no: tradeNo,
            payment_method: 'alipay',
            create_time: 1700000000,
            status: 'success',
          },
        ],
      },
    },
  })
  const user = userEvent.setup()
  render(<BillingHistoryDialog open onOpenChange={vi.fn()} />)

  expect(await screen.findByText(tradeNo)).toHaveAttribute('title', tradeNo)
  expect(screen.getByText('Success', { exact: true })).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Copy to clipboard' }))
  expect(copyToClipboard).toHaveBeenCalledWith(tradeNo)
})
