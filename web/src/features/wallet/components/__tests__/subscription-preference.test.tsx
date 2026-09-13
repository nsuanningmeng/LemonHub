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
import { afterAll, beforeAll, expect, test, vi } from 'vitest'

import type { SelfSubscriptionData } from '@/features/subscriptions/types'
import { api } from '@/lib/api'

import { SubscriptionPlansCard } from '../subscription-plans-card'

const originalGetAnimations = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'getAnimations'
)

beforeAll(() => {
  Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
})

afterAll(() => {
  if (originalGetAnimations) {
    Object.defineProperty(
      HTMLElement.prototype,
      'getAnimations',
      originalGetAnimations
    )
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, 'getAnimations')
  }
})

function mockSubscription(
  preference: string,
  status: 'none' | 'expired' | 'active'
) {
  const records =
    status === 'none'
      ? []
      : [
          {
            subscription: {
              id: 1,
              user_id: 7,
              plan_id: 3,
              status,
              start_time: 1,
              end_time: 2,
              amount_total: 1000,
              amount_used: 100,
            },
          },
        ]
  const self: SelfSubscriptionData = {
    billing_preference: preference,
    subscriptions: status === 'active' ? records : [],
    all_subscriptions: records,
  }
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/subscription/self') {
      return { data: { success: true, data: self } }
    }
    if (url === '/api/subscription/plans') {
      return {
        data: {
          success: true,
          data: [
            {
              plan: {
                id: 3,
                title: 'Monthly plan',
                price_amount: 10,
                currency: 'USD',
                duration_unit: 'month',
                duration_value: 1,
                quota_reset_period: 'never',
                enabled: true,
                sort_order: 0,
                max_purchase_per_user: 1,
                total_amount: 1000,
                allow_balance_pay: true,
                allow_wallet_overflow: true,
              },
            },
          ],
        },
      }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
  return vi.spyOn(api, 'put')
}

test.each([
  ['subscription_only', 'Subscription Only'],
  ['subscription_first', 'Subscription First'],
  ['wallet_first', 'Wallet First'],
  ['wallet_only', 'Wallet Only'],
])(
  'no active subscription still shows saved %s without writing a new preference',
  async (preference, label) => {
    const update = mockSubscription(preference, 'none')
    render(<SubscriptionPlansCard topupInfo={null} />)
    expect(await screen.findByRole('combobox')).toHaveTextContent(label)
    expect(update).not.toHaveBeenCalled()
    if (preference === 'subscription_only') {
      expect(screen.getByText(/Requests will be rejected/)).toBeInTheDocument()
      expect(
        screen.queryByText(/Wallet will be used automatically/)
      ).not.toBeInTheDocument()
    } else if (preference === 'subscription_first') {
      expect(
        screen.getByText(/Wallet will be used automatically/)
      ).toBeInTheDocument()
      expect(
        screen.queryByText(/Requests will be rejected/)
      ).not.toBeInTheDocument()
    } else {
      expect(
        screen.queryByText(/no active subscription/)
      ).not.toBeInTheDocument()
    }
  }
)

test('expired subscription-only preference warns of rejection rather than wallet fallback', async () => {
  mockSubscription('subscription_only', 'expired')
  render(<SubscriptionPlansCard topupInfo={null} />)
  expect(await screen.findByRole('combobox')).toHaveTextContent(
    'Subscription Only'
  )
  expect(screen.getByText(/Requests will be rejected/)).toBeInTheDocument()
})

test('active subscription-only preference shows no unavailable-subscription warning', async () => {
  mockSubscription('subscription_only', 'active')
  render(<SubscriptionPlansCard topupInfo={null} />)
  expect(await screen.findByRole('combobox')).toHaveTextContent(
    'Subscription Only'
  )
  expect(screen.queryByText(/no active subscription/)).not.toBeInTheDocument()
})

test('failed preference update restores subscription-only selection and warning', async () => {
  const update = mockSubscription('subscription_only', 'none')
  update.mockResolvedValue({
    data: { success: false, message: 'Update failed' },
  })
  const user = userEvent.setup()
  render(<SubscriptionPlansCard topupInfo={null} />)
  const selector = await screen.findByRole('combobox')
  await user.click(selector)
  await user.click(await screen.findByRole('option', { name: 'Wallet Only' }))
  await waitFor(() =>
    expect(update).toHaveBeenCalledWith('/api/subscription/self/preference', {
      billing_preference: 'wallet_only',
    })
  )
  await waitFor(() => expect(selector).toHaveTextContent('Subscription Only'))
  expect(screen.getByText(/Requests will be rejected/)).toBeInTheDocument()
})
