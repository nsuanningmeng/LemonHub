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

test('missing account data keeps wallet totals unavailable until data arrives', () => {
  const { rerender } = render(<WalletStatsCard user={null} loading />)
  expect(screen.getByText('Current Balance').closest('dl')).toHaveAttribute(
    'aria-busy',
    'true'
  )
  rerender(<WalletStatsCard user={null} />)
  expect(screen.getAllByText('—')).toHaveLength(3)
  expect(screen.getByText('Current Balance').closest('dl')).toHaveAttribute(
    'aria-busy',
    'false'
  )
})

test('the wallet preview reuses its order rows when opening full history', async () => {
  const order = {
    id: 11,
    user_id: 42,
    amount: 90,
    money: 90,
    trade_no: 'QA-order-11',
    payment_method: 'alipay',
    create_time: 1700000000,
    status: 'success',
  }
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { items: [order], total: 1 } },
  })
  const onOpenChange = vi.fn()
  const user = userEvent.setup()
  const { rerender } = render(
    <BillingHistoryDialog
      showPreview
      open={false}
      onOpenChange={onOpenChange}
    />
  )
  expect(await screen.findByText('Success')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'View orders' }))
  expect(onOpenChange).toHaveBeenCalledWith(true)
  rerender(
    <BillingHistoryDialog showPreview open onOpenChange={onOpenChange} />
  )
  expect(await screen.findByText('QA-order-11')).toBeVisible()
  expect(get).toHaveBeenCalledTimes(1)
})

test('an unavailable order preview offers a retry before showing an empty history', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: false, message: 'Unavailable' },
  })
  render(
    <BillingHistoryDialog open={false} showPreview onOpenChange={vi.fn()} />
  )

  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Failed to load billing history'
  )
  expect(screen.queryByText('No orders yet')).not.toBeInTheDocument()
  get.mockResolvedValue({
    data: { success: true, data: { items: [], total: 0 } },
  })
  await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
  expect(await screen.findByText('No orders yet')).toBeVisible()
})
