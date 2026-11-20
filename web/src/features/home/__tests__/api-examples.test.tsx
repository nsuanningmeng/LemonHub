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
import { expect, test } from 'vitest'

import { HeroTerminalDemo } from '../components/hero-terminal-demo'

test('choosing a different protocol by keyboard shows its endpoint and authentication header', async () => {
  const user = userEvent.setup()
  render(<HeroTerminalDemo serverUrl='https://gateway.test' />)
  const chat = screen.getByRole('tab', { name: 'Chat' })
  await user.click(chat)
  expect(screen.getByRole('tabpanel')).toHaveTextContent('/v1/chat/completions')
  await user.keyboard('{ArrowRight}{ArrowRight}{Enter}')
  expect(screen.getByRole('tab', { name: 'Claude' })).toHaveAttribute(
    'aria-selected',
    'true'
  )
  expect(screen.getByRole('tabpanel')).toHaveTextContent('/v1/messages')
  expect(screen.getByRole('tabpanel')).toHaveTextContent(
    'anthropic-version: 2023-06-01'
  )
  expect(screen.getByRole('tabpanel')).not.toHaveTextContent('Bearer')
})

test('a developer can edit the model, copy its request, and inspect the response', async () => {
  const user = userEvent.setup()
  render(<HeroTerminalDemo serverUrl='https://gateway.test' />)
  await user.type(
    screen.getByRole('textbox', { name: 'Model name' }),
    "custom'model"
  )
  await user.click(screen.getByRole('button', { name: 'Copy code' }))
  const copied = await navigator.clipboard.readText()
  expect(copied).toContain(
    "curl -X POST 'https://gateway.test/v1/chat/completions'"
  )
  expect(copied).toContain("custom'\\''model")
  await user.click(screen.getByRole('button', { name: 'Response' }))
  expect(screen.getByRole('button', { name: 'Response' })).toHaveAttribute(
    'aria-pressed',
    'true'
  )
  expect(screen.getByRole('tabpanel')).toHaveTextContent('choices')
  await user.click(screen.getByRole('button', { name: 'Copy URL' }))
  expect(await navigator.clipboard.readText()).toBe('https://gateway.test/v1')
})
