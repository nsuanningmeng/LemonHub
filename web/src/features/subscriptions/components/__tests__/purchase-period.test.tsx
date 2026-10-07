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
import { expect, test, vi } from 'vitest'

import { SubscriptionPlansCard } from '@/features/wallet/components/subscription-plans-card'
import { api } from '@/lib/api'

import { formatTimestamp } from '../../lib/format'
import { subscriptionPlanSchema } from '../../types'
import { SubscriptionPurchaseDialog } from '../dialogs/subscription-purchase-dialog'

const plan = {
  plan: subscriptionPlanSchema.parse({
    id: 11,
    title: 'Monthly',
    price_amount: 1,
    duration_unit: 'month',
    duration_value: 1,
    quota_reset_period: 'monthly',
    enabled: true,
    sort_order: 0,
    max_purchase_per_user: 0,
    total_amount: 100,
    stripe_price_id: 'price-monthly',
  }),
}
const start = Math.floor(Date.now() / 1000) - 86400
const end = start + 86400 * 30
const current = {
  subscription: {
    id: 31,
    user_id: 1,
    plan_id: 11,
    status: 'active',
    start_time: start,
    end_time: end,
    amount_total: 100,
    amount_used: 7,
  },
}
const notice =
  'After payment succeeds, a new subscription starts immediately with its own quota. Subscription periods may overlap; this purchase does not extend an existing subscription.'

test.each(['Pay with Balance', 'Stripe'])(
  'the %s confirmation explains parallel periods and shows the existing plan dates without changing payment requests',
  async (payment) => {
    const post = vi
      .spyOn(api, 'post')
      .mockResolvedValue({ data: { success: true, data: {} } })
    render(
      <SubscriptionPurchaseDialog
        open
        onOpenChange={() => undefined}
        plan={plan}
        enableStripe
        userQuota={1000000}
        activeSubscriptions={[current]}
      />
    )
    expect(screen.getByText(notice)).toBeInTheDocument()
    expect(
      screen.getByText('You already have an active subscription to this plan.')
    ).toBeInTheDocument()
    expect(
      screen.getByText(
        `Existing period: ${formatTimestamp(start)} – ${formatTimestamp(end)}`
      )
    ).toBeInTheDocument()
    expect(
      screen.getByText(
        'New period: starts after successful payment and lasts 1 months. Exact dates are recorded when the subscription is activated.'
      )
    ).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: payment }))
    expect(post).toHaveBeenCalledWith(
      payment === 'Stripe'
        ? '/api/subscription/stripe/pay'
        : '/api/subscription/balance/pay',
      { plan_id: 11 }
    )
  }
)

test('an active different plan or an expired same plan does not trigger a same-plan warning', () => {
  render(
    <SubscriptionPurchaseDialog
      open
      onOpenChange={() => undefined}
      plan={plan}
      activeSubscriptions={[
        { subscription: { ...current.subscription, plan_id: 22 } },
        {
          subscription: {
            ...current.subscription,
            status: 'expired',
            end_time: start,
          },
        },
      ]}
    />
  )
  expect(screen.getByText(notice)).toBeInTheDocument()
  expect(
    screen.queryByText('You already have an active subscription to this plan.')
  ).not.toBeInTheDocument()
})

test('the wallet plan list carries the actual active subscription into the purchase confirmation', async () => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data: {
      success: true,
      data:
        url === '/api/subscription/self'
          ? {
              billing_preference: 'subscription_first',
              subscriptions: [current],
              all_subscriptions: [current],
            }
          : [plan],
    },
  }))
  render(<SubscriptionPlansCard topupInfo={null} />)
  await userEvent.click(
    await screen.findByRole('button', { name: 'Subscribe Now' })
  )
  expect(
    screen.getByText('You already have an active subscription to this plan.')
  ).toBeInTheDocument()
  expect(
    screen.getByText(
      `Existing period: ${formatTimestamp(start)} – ${formatTimestamp(end)}`
    )
  ).toBeInTheDocument()
})
