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
import { expect, test } from 'vitest'

import { formatDeploymentEstimate } from '../deployments-utils'

test('uses the server estimate, including a zero price and older response shapes', () => {
  expect(
    formatDeploymentEstimate({
      price_breakdown: { TotalCost: '2.75' },
      currency: 'usdc',
    })
  ).toBe('2.75 USDC')
  expect(formatDeploymentEstimate({ total_cost: 0 }, 'usd')).toBe('0 USD')
  expect(formatDeploymentEstimate({ total: 16, currency: 'usd' })).toBe(
    '16 USD'
  )
  expect(formatDeploymentEstimate({ currency: 'usd' })).toBe('')
})
