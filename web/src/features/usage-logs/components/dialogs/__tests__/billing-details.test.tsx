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
import { describe, expect, test } from 'vitest'

import { formatLogQuota } from '@/lib/format'

import { usageLogSchema } from '../../../data/schema'
import type { LogOtherData } from '../../../types'
import { DetailsDialog } from '../details-dialog'

function renderBillingDetails(other: LogOtherData): void {
  const log = usageLogSchema.parse({
    id: 1,
    user_id: 1,
    created_at: 1728000000,
    type: 2,
    content: '',
    model_name: 'billing-test',
    quota: 800000,
    other: JSON.stringify(other),
  })
  render(
    <DetailsDialog
      log={log}
      isAdmin={false}
      open
      onOpenChange={() => undefined}
    />
  )
}

describe('subscription billing details', () => {
  test.each([300000, 0, 800000])(
    'keeps total cost and reports actual source amounts when subscription pays %i',
    (subscriptionQuota) => {
      renderBillingDetails({
        billing_source: 'subscription',
        subscription_consumed: subscriptionQuota,
        wallet_quota_deducted: 800000 - subscriptionQuota,
      })

      expect(screen.getByText('Total Cost').parentElement).toHaveTextContent(
        formatLogQuota(800000)
      )
      expect(
        screen.getByText('Deducted by subscription').parentElement
      ).toHaveTextContent(formatLogQuota(subscriptionQuota))
      expect(
        screen.getByText('Deducted from wallet').parentElement
      ).toHaveTextContent(formatLogQuota(800000 - subscriptionQuota))
    }
  )

  test('preserves legacy subscription details when the wallet split is absent', () => {
    renderBillingDetails({
      billing_source: 'subscription',
      subscription_consumed: 800000,
    })

    expect(screen.getByText('Final Consumed').parentElement).toHaveTextContent(
      formatLogQuota(800000)
    )
    expect(screen.queryByText('Deducted from wallet')).not.toBeInTheDocument()
  })
})

test('recorded time tier displays its exact conditions and all configured rates', () => {
  const expression =
    'hour("Asia/Shanghai") >= 9 && hour("Asia/Shanghai") < 18 ? tier("peak", p * 3 + c * 15 + cr * 0.3) : tier("off-peak", p * 1.5 + c * 7.5 + cr * 0.15)'
  renderBillingDetails({
    billing_mode: 'tiered_expr',
    expr_b64: btoa(expression),
    matched_tier: 'off-peak',
    cache_tokens: 100,
  })
  expect(
    screen.getAllByText(/09:00.*18:00.*Asia\/Shanghai/).length
  ).toBeGreaterThan(0)
  expect(screen.getAllByText(/!.*hour.*Asia\/Shanghai/).length).toBeGreaterThan(
    0
  )
  expect(screen.getAllByText('Matched').length).toBeGreaterThan(0)
})
