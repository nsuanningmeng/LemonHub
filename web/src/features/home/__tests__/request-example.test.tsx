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
  expect(screen.getByRole('button', { name: 'OpenAI' })).toHaveAttribute(
    'aria-pressed',
    'true'
  )
  expect(screen.getByLabelText('Request example')).toHaveTextContent(
    '/v1/chat/completions'
  )

  const anthropic = screen.getByRole('button', { name: 'Anthropic' })
  anthropic.focus()
  await user.keyboard('{Enter}')
  expect(anthropic).toHaveAttribute('aria-pressed', 'true')
  expect(screen.getByRole('button', { name: 'OpenAI' })).toHaveAttribute(
    'aria-pressed',
    'false'
  )
  expect(screen.getByLabelText('Request example')).toHaveTextContent(
    '/v1/messages'
  )
  expect(screen.getByLabelText('Request example')).toHaveTextContent(
    'anthropic-version: 2023-06-01'
  )
})

test('copying a selected example uses this gateway origin and real shell line continuations', async () => {
  const user = userEvent.setup()
  const writeText = vi
    .spyOn(navigator.clipboard, 'writeText')
    .mockResolvedValue(undefined)
  render(<HeroTerminalDemo />)
  await user.click(screen.getByRole('button', { name: 'Gemini' }))
  await user.click(screen.getByRole('button', { name: 'Copy request' }))
  expect(await screen.findByRole('button', { name: 'Copied' })).toBeVisible()
  const copied = writeText.mock.calls[0][0]
  expect(copied).toContain(
    `${window.location.origin}/v1beta/models/your-model:generateContent`
  )
  expect(copied).toContain('x-goog-api-key: $LEMONHUB_API_KEY')
  expect(copied).toContain(' \\' + '\n')
  expect(copied).not.toContain('\\n')
})

test('long request examples scroll locally and remain keyboard reachable', () => {
  render(<HeroTerminalDemo />)
  const example = screen.getByLabelText('Request example')
  expect(example).toHaveClass('max-w-full', 'overflow-x-auto')
  expect(example).toHaveAttribute('tabindex', '0')
})
