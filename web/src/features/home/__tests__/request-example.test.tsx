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

import { HeroTerminalDemo } from '../components/hero-terminal-demo'

test('choosing a protocol updates the request and exposes the selected protocol to assistive technology', async () => {
  const user = userEvent.setup()
  render(<HeroTerminalDemo />)
  expect(screen.getByRole('tab', { name: 'Chat' })).toHaveAttribute(
    'aria-selected',
    'true'
  )
  expect(
    screen.getByLabelText('Request', { selector: 'pre' })
  ).toHaveTextContent('/v1/chat/completions')

  const anthropic = screen.getByRole('tab', { name: 'Claude' })
  anthropic.focus()
  await user.keyboard('{Enter}')
  expect(anthropic).toHaveAttribute('aria-selected', 'true')
  expect(screen.getByRole('tab', { name: 'Chat' })).toHaveAttribute(
    'aria-selected',
    'false'
  )
  expect(
    screen.getByLabelText('Request', { selector: 'pre' })
  ).toHaveTextContent('/v1/messages')
  expect(
    screen.getByLabelText('Request', { selector: 'pre' })
  ).toHaveTextContent('anthropic-version: 2023-06-01')
})

test('copying a selected example uses this gateway origin and real shell line continuations', async () => {
  const user = userEvent.setup()
  const writeText = vi
    .spyOn(navigator.clipboard, 'writeText')
    .mockResolvedValue(undefined)
  render(<HeroTerminalDemo />)
  await user.click(screen.getByRole('tab', { name: 'Gemini' }))
  await user.click(screen.getByRole('button', { name: 'Copy code' }))
  expect(await screen.findByRole('button', { name: 'Copied' })).toBeVisible()
  const copied = writeText.mock.calls[0][0]
  expect(copied).toContain(
    `${window.location.origin}/v1beta/models/your-model:generateContent`
  )
  expect(copied).toContain('x-goog-api-key: $LEMONHUB_API_KEY')
  expect(copied).toContain(' \\' + '\n')
  expect(copied).not.toContain('\\n')
})

test('the active request and its copy action remain keyboard reachable', () => {
  render(<HeroTerminalDemo />)
  const region = screen.getByRole('region', { name: 'API examples' })
  const example = screen.getByLabelText('Request', { selector: 'pre' })
  expect(region).toContainElement(example)
  expect(region).toContainElement(
    screen.getByRole('button', { name: 'Copy code' })
  )
  example.focus()
  expect(example).toHaveFocus()
  expect(example).toHaveAttribute('tabindex', '0')
})
