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

import { ErrorState } from '../error-state'
import { LoadingState } from '../loading-state'

test('announces loading when an inline spinner has no visible message', () => {
  render(<LoadingState inline />)
  expect(screen.getByRole('status')).toHaveTextContent('Loading...')
})

test('announces the supplied operation when showing a loading message', () => {
  render(<LoadingState message='Fetching usage' />)
  expect(screen.getByRole('status')).toHaveTextContent('Fetching usage')
})

test('announces a load error and lets the keyboard retry the request', async () => {
  const retry = vi.fn()
  const user = userEvent.setup()
  render(<ErrorState title='Could not load usage' onRetry={retry} />)
  expect(screen.getByRole('alert')).toHaveTextContent('Could not load usage')
  await user.tab()
  expect(screen.getByRole('button', { name: 'Retry' })).toHaveFocus()
  await user.keyboard('{Enter}')
  expect(retry).toHaveBeenCalledOnce()
})
