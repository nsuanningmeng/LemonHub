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
import { afterAll, beforeAll, expect, test, vi } from 'vitest'

import { PaymentConfirmDialog } from '../payment-confirm-dialog'

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

const paymentDetails = {
  open: true,
  topupAmount: 1000,
  paymentAmount: 120,
  paymentMethod: { type: 'alipay', name: 'Alipay' },
  calculating: false,
  processing: false,
}

test('a successful positive quote shows the payment amount and allows confirmation', async () => {
  const confirm = vi.fn()
  const user = userEvent.setup()
  render(
    <PaymentConfirmDialog
      {...paymentDetails}
      onOpenChange={vi.fn()}
      onConfirm={confirm}
    />
  )

  const button = screen.getByRole('button', { name: 'Confirm Payment' })
  expect(screen.getByText('120')).toBeInTheDocument()
  expect(button).toBeEnabled()
  await user.click(button)
  expect(confirm).toHaveBeenCalledOnce()
})

test('a failed quote displays the server reason and prevents paying a stale amount', async () => {
  const confirm = vi.fn()
  const user = userEvent.setup()
  render(
    <PaymentConfirmDialog
      {...paymentDetails}
      paymentError='Payment provider is unavailable'
      onOpenChange={vi.fn()}
      onConfirm={confirm}
    />
  )

  expect(screen.getByRole('alert')).toHaveTextContent(
    'Payment provider is unavailable'
  )
  expect(screen.queryByText('120')).not.toBeInTheDocument()
  const button = screen.getByRole('button', { name: 'Confirm Payment' })
  expect(button).toBeDisabled()
  await user.click(button)
  expect(confirm).not.toHaveBeenCalled()
})

test.each([0, -1, Number.NaN, Number.POSITIVE_INFINITY])(
  'an invalid quote of %s shows a failure instead of a payable amount and prevents confirmation',
  async (paymentAmount) => {
    const confirm = vi.fn()
    const user = userEvent.setup()
    render(
      <PaymentConfirmDialog
        {...paymentDetails}
        paymentAmount={paymentAmount}
        onOpenChange={vi.fn()}
        onConfirm={confirm}
      />
    )

    expect(screen.getByRole('alert')).toHaveTextContent(
      'Payment request failed'
    )
    expect(screen.queryByText('0')).not.toBeInTheDocument()
    const button = screen.getByRole('button', { name: 'Confirm Payment' })
    expect(button).toBeDisabled()
    await user.click(button)
    expect(confirm).not.toHaveBeenCalled()
  }
)

test.each([
  { calculating: true, processing: false },
  { calculating: false, processing: true },
])(
  'calculating=$calculating and processing=$processing prevents confirmation',
  async (state) => {
    const confirm = vi.fn()
    const user = userEvent.setup()
    render(
      <PaymentConfirmDialog
        {...paymentDetails}
        {...state}
        onOpenChange={vi.fn()}
        onConfirm={confirm}
      />
    )

    const button = screen.getByRole('button', { name: 'Confirm Payment' })
    expect(button).toBeDisabled()
    await user.click(button)
    expect(confirm).not.toHaveBeenCalled()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  }
)
