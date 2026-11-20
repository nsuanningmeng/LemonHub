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
import { act, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { expect, test, vi } from 'vitest'

import { createLocaleI18n } from '@/i18n/__tests__/helpers'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { BillingHistoryDialog } from '../dialogs/billing-history-dialog'
import { RechargeFormCard } from '../recharge-form-card'

test('billing history translates all provider statuses when the language changes', async () => {
  const i18n = await createLocaleI18n('en')
  const previousUser = useAuthStore.getState().auth.user
  useAuthStore.getState().auth.setUser(null)
  const records = ['success', 'pending', 'expired'].map((status, index) => ({
    id: index + 1,
    user_id: 1,
    amount: 10,
    money: 10,
    trade_no: `local-${status}`,
    payment_method: 'stripe',
    create_time: 1700000000,
    status,
  }))
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { items: records, total: 3 } },
  })
  try {
    render(
      <I18nextProvider i18n={i18n}>
        <BillingHistoryDialog open onOpenChange={vi.fn()} />
      </I18nextProvider>
    )
    expect(
      await screen.findByText('Success', { exact: true })
    ).toBeInTheDocument()
    expect(screen.getByText('Pending', { exact: true })).toBeInTheDocument()
    expect(screen.getByText('Expired', { exact: true })).toBeInTheDocument()
    await act(async () => {
      await i18n.changeLanguage('zhCN')
    })
    for (const status of ['Success', 'Pending', 'Expired']) {
      expect(
        screen.getByText(i18n.t(status), { exact: true })
      ).toBeInTheDocument()
      expect(
        screen.queryByText(status, { exact: true })
      ).not.toBeInTheDocument()
    }
    expect(records.map((record) => record.status)).toEqual([
      'success',
      'pending',
      'expired',
    ])
  } finally {
    useAuthStore.getState().auth.setUser(previousUser)
  }
})

test('recharge language changes preserve preset amounts, savings and selection callbacks', async () => {
  const i18n = await createLocaleI18n('en')
  const select = vi.fn()
  const user = userEvent.setup()
  render(
    <I18nextProvider i18n={i18n}>
      <RechargeFormCard
        topupInfo={{
          enable_online_topup: true,
          enable_stripe_topup: false,
          pay_methods: [],
          min_topup: 5,
          stripe_min_topup: 5,
          amount_options: [730, 100],
          discount: {},
        }}
        presetAmounts={[{ value: 730, discount: 0.8 }, { value: 100 }]}
        selectedPreset={null}
        onSelectPreset={select}
        topupAmount={5}
        onTopupAmountChange={vi.fn()}
        paymentAmount={5}
        calculating={false}
        onPaymentMethodSelect={vi.fn()}
        paymentLoading={null}
        redemptionCode=''
        onRedemptionCodeChange={vi.fn()}
        onRedeem={vi.fn()}
        redeeming={false}
      />
    </I18nextProvider>
  )
  expect(
    screen.getByRole('button', { name: '730 20% OFF Pay 584 • Save 146' })
  ).toBeInTheDocument()
  expect(
    screen.getByRole('button', { name: '100 Pay 100' })
  ).toBeInTheDocument()
  expect(screen.getByPlaceholderText('Minimum 5')).toBeInTheDocument()
  await act(async () => {
    await i18n.changeLanguage('zhCN')
  })
  const discounted = screen.getByRole('button', {
    name: '730 优惠 20% 支付 584 • 节省 146',
  })
  expect(
    within(discounted).getByText('• 节省 146', { selector: 'span' })
  ).toBeInTheDocument()
  expect(
    screen.getByRole('button', { name: '100 支付 100' })
  ).toBeInTheDocument()
  expect(screen.getByPlaceholderText('最低 5')).toBeInTheDocument()
  await user.click(discounted)
  expect(select).toHaveBeenCalledExactlyOnceWith({ value: 730, discount: 0.8 })
})
