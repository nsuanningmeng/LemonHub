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
import { expect, it, vi } from 'vitest'

import {
  WaffoSettingsSection,
  type WaffoSettingsValues,
} from '../waffo-settings-section'

const values: WaffoSettingsValues = {
  WaffoEnabled: true,
  WaffoApiKey: '',
  WaffoPrivateKey: '',
  WaffoPublicCert: '',
  WaffoSandboxPublicCert: '',
  WaffoSandboxApiKey: '',
  WaffoSandboxPrivateKey: '',
  WaffoSandbox: false,
  WaffoMerchantId: '',
  WaffoCurrency: 'USD',
  WaffoUnitPrice: 1,
  WaffoMinTopUp: 1,
  WaffoNotifyUrl: '',
  WaffoReturnUrl: '',
  WaffoPayMethods: '[]',
}

it('stacks payment credentials and amounts on phones while retaining desktop columns', () => {
  render(
    <WaffoSettingsSection
      values={values}
      onValueChange={vi.fn()}
      payMethods={[]}
      onPayMethodsChange={vi.fn()}
    />
  )

  for (const label of [
    'API Key (Production)',
    'RSA Private Key (Production)',
    'Waffo Public Key (Production)',
    'Callback notification URL',
  ]) {
    const fields = screen.getByText(label).parentElement?.parentElement
    expect(fields).toHaveClass('grid-cols-1', 'sm:grid-cols-2')
  }
  expect(screen.getByText('Currency').parentElement?.parentElement).toHaveClass(
    'grid-cols-1',
    'sm:grid-cols-3'
  )
})

it('allows the payment-method heading and add action to wrap on narrow screens', () => {
  render(
    <WaffoSettingsSection
      values={values}
      onValueChange={vi.fn()}
      payMethods={[]}
      onPayMethodsChange={vi.fn()}
    />
  )

  expect(
    screen.getByRole('button', { name: 'Add payment method' }).parentElement
  ).toHaveClass('flex-wrap', 'gap-2')
})
