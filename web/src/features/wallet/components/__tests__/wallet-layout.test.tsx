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
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { CreemProductsSection } from '../creem-products-section'
import { BillingHistoryDialog } from '../dialogs/billing-history-dialog'
import { SubscriptionPlansCard } from '../subscription-plans-card'
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

test('a loading balance announces its pending state without showing a misleading zero total', () => {
  const view = render(<WalletStatsCard user={null} loading />)

  expect(
    screen.getByRole('region', { name: 'Current Balance' })
  ).toHaveAttribute('aria-busy', 'true')
  expect(screen.queryByText('$0')).not.toBeInTheDocument()

  view.rerender(<WalletStatsCard user={null} loading={false} />)
  expect(
    screen.getByRole('region', { name: 'Current Balance' })
  ).toHaveAttribute('aria-busy', 'false')
  expect(screen.getAllByText('$0')).toHaveLength(2)
})

test('a Creem package exposes its price and quota and can be selected with the keyboard', async () => {
  const product = {
    productId: 'studio-credit',
    name: 'Studio credits',
    quota: 100,
    price: 20,
    currency: 'USD' as const,
  }
  const select = vi.fn()
  const user = userEvent.setup()
  render(<CreemProductsSection products={[product]} onProductSelect={select} />)

  const option = screen.getByRole('button', { name: /Studio credits/ })
  expect(option).toHaveTextContent('Quota: 100')
  expect(option).toHaveTextContent('$20.00')
  await user.tab()
  expect(option).toHaveFocus()
  await user.keyboard('{Enter}')
  expect(select).toHaveBeenCalledExactlyOnceWith(product)
})

test('subscription comparisons keep long plan names readable and retain purchase limits', async () => {
  const title = 'Production workspace plan with extended model access'
  const plan = {
    id: 3,
    title,
    price_amount: 10,
    duration_unit: 'month',
    duration_value: 1,
    quota_reset_period: 'monthly',
    max_purchase_per_user: 1,
    total_amount: 5000000,
  }
  vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data: {
      success: true,
      data:
        url === '/api/subscription/plans'
          ? [{ plan }, { plan: { ...plan, id: 4, title: 'Flexible plan' } }]
          : {
              billing_preference: 'wallet_only',
              subscriptions: [],
              all_subscriptions: [
                {
                  subscription: {
                    id: 1,
                    plan_id: 3,
                    end_time: 1,
                    status: 'expired',
                    amount_total: 5000000,
                    amount_used: 5000000,
                  },
                },
              ],
            },
    },
  }))
  render(<SubscriptionPlansCard topupInfo={null} />)

  const limited = await screen.findByRole('article', { name: title })
  expect(within(limited).getByRole('heading', { name: title })).toHaveClass(
    'wrap-anywhere'
  )
  expect(
    within(limited).getByRole('button', { name: 'Limit Reached' })
  ).toBeDisabled()
  expect(
    within(screen.getByRole('article', { name: 'Flexible plan' })).getByRole(
      'button',
      { name: 'Subscribe Now' }
    )
  ).toBeEnabled()
})
