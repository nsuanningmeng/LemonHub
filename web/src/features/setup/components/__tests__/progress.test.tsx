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
import { expect, test } from 'vitest'

import { SetupProgress } from '../setup-progress'

test('moves the current step announcement when the setup advances', () => {
  const { rerender } = render(<SetupProgress currentStep={0} />)
  const wizard = screen.getByRole('list', { name: 'System setup wizard' })
  expect(
    within(wizard).getByRole('listitem', { current: 'step' })
  ).toHaveTextContent('Database check')
  rerender(<SetupProgress currentStep={1} />)
  expect(
    within(wizard).getByRole('listitem', { current: 'step' })
  ).toHaveTextContent('Administrator account')
  expect(within(wizard).getAllByRole('listitem')).toHaveLength(4)
})
