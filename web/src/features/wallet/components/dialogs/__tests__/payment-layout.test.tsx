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

import { CreemConfirmDialog } from '../creem-confirm-dialog'
import { PaymentConfirmDialog } from '../payment-confirm-dialog'

test('a long payment method wraps within a narrow confirmation dialog and remains payable', async () => {
  const name = 'InternationalPaymentProviderWithoutSpacesForMobileCheckout'
  const confirm = vi.fn()
  render(
    <PaymentConfirmDialog
      open
      onOpenChange={vi.fn()}
      onConfirm={confirm}
      topupAmount={1000}
      paymentAmount={120}
      paymentMethod={{ type: 'alipay', name }}
      calculating={false}
      processing={false}
    />
  )

  const method = screen.getByText(name)
  expect(method).toHaveClass('min-w-0', 'wrap-anywhere')
  expect(method.parentElement).toHaveClass('min-w-0', 'max-w-full')
  expect(method.parentElement?.parentElement).toHaveClass(
    'flex-col',
    'sm:flex-row'
  )
  await userEvent.click(screen.getByRole('button', { name: 'Confirm Payment' }))
  expect(confirm).toHaveBeenCalledOnce()
})

test('a long Creem product name wraps below its label on phones without hiding purchase controls', async () => {
  const name = 'ProfessionalSubscriptionProductWithoutSpacesForMobileCheckout'
  const confirm = vi.fn()
  render(
    <CreemConfirmDialog
      open
      onOpenChange={vi.fn()}
      onConfirm={confirm}
      product={{
        name,
        productId: 'mobile-product',
        price: 25,
        currency: 'USD',
        quota: 1000,
      }}
      processing={false}
    />
  )

  const product = screen.getByText(name)
  expect(product).toHaveClass('min-w-0', 'wrap-anywhere')
  expect(product.parentElement).toHaveClass('flex-col', 'sm:flex-row')
  await userEvent.click(screen.getByRole('button', { name: 'Confirm Payment' }))
  expect(confirm).toHaveBeenCalledOnce()
})
