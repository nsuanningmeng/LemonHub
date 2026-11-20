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
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { PanelWrapper } from '../panel-wrapper'

afterEach(cleanup)

test('a loading panel labels its busy region and keeps the refresh control operable', async () => {
  const refresh = vi.fn()
  const user = userEvent.setup()
  render(
    <PanelWrapper
      title='Service health'
      headerTone='teal'
      loading
      headerActions={
        <button type='button' onClick={refresh}>
          Refresh
        </button>
      }
    >
      Stale result
    </PanelWrapper>
  )
  expect(
    screen.getByRole('region', { name: 'Service health' })
  ).toHaveAttribute('aria-busy', 'true')
  expect(screen.getByRole('status', { name: 'Loading' })).toBeVisible()
  expect(screen.queryByText('Stale result')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Refresh' }))
  expect(refresh).toHaveBeenCalledOnce()
})

test('an empty panel announces its configured empty state before showing loaded content', () => {
  const { rerender } = render(
    <PanelWrapper
      title='API routes'
      headerTone='teal'
      empty
      emptyMessage='No configured routes'
    >
      Route list
    </PanelWrapper>
  )
  expect(screen.getByRole('status')).toHaveTextContent('No configured routes')
  expect(screen.queryByText('Route list')).not.toBeInTheDocument()
  rerender(
    <PanelWrapper title='API routes' headerTone='teal' empty={false}>
      Route list
    </PanelWrapper>
  )
  expect(screen.queryByRole('status')).not.toBeInTheDocument()
  expect(screen.getByRole('region', { name: 'API routes' })).toHaveAttribute(
    'aria-busy',
    'false'
  )
  expect(screen.getByText('Route list')).toBeVisible()
})

test('a long panel title can wrap while its refresh action stays available', () => {
  const title =
    'Regional service health for a production workspace with a long name'
  render(
    <PanelWrapper
      title={title}
      headerActions={<button type='button'>Refresh</button>}
    >
      Healthy
    </PanelWrapper>
  )
  expect(screen.getByRole('heading', { name: title })).toHaveClass(
    'break-words'
  )
  expect(screen.getByRole('button', { name: 'Refresh' })).toBeVisible()
  expect(screen.getByRole('region', { name: title })).toHaveTextContent(
    'Healthy'
  )
})
