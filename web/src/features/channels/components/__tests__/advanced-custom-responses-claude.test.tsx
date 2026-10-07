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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'

import {
  getAdvancedCustomConverterOptions,
  validateAdvancedCustomConfig,
} from '../../lib/advanced-custom'
import type { AdvancedCustomConfig } from '../../types'
import { AdvancedCustomEditorDialog } from '../dialogs/advanced-custom-editor-dialog'

const converter = 'openai_responses_to_claude_messages'

test('selecting Responses to Claude saves the matching path and authentication while retaining model restrictions', async () => {
  const onSave = vi.fn()
  const user = userEvent.setup()
  render(
    <AdvancedCustomEditorDialog
      open
      value={JSON.stringify({
        advanced_routes: [
          {
            incoming_path: '/v1/responses',
            upstream_path: '/v1/responses',
            converter: 'none',
            models: ['restricted-model'],
          },
        ],
      })}
      onOpenChange={vi.fn()}
      onSave={onSave}
    />
  )
  const select = screen
    .getAllByRole('combobox')
    .find((element) => element.textContent === 'Native forwarding')
  expect(select).toBeDefined()
  select?.focus()
  await user.keyboard('{Enter}')
  await user.click(
    await screen.findByRole('option', {
      name: 'OpenAI Responses to Anthropic Messages',
    })
  )
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  await waitFor(() => expect(onSave).toHaveBeenCalledOnce())
  const saved = JSON.parse(onSave.mock.calls[0][0]) as AdvancedCustomConfig
  expect(saved.advanced_routes?.[0]).toMatchObject({
    incoming_path: '/v1/responses',
    upstream_path: '/v1/messages',
    converter,
    models: ['restricted-model'],
    auth: { type: 'header', name: 'x-api-key', value: '{api_key}' },
  })
})

test('reopening and saving a Responses to Claude route preserves explicit endpoint and authentication values', async () => {
  const route = {
    incoming_path: '/v1/responses',
    upstream_path: '/provider/claude/messages',
    converter,
    models: ['model-a'],
    auth: { type: 'header', name: 'provider-auth', value: 'prefix {api_key}' },
  }
  const onSave = vi.fn()
  const user = userEvent.setup()
  render(
    <AdvancedCustomEditorDialog
      open
      value={JSON.stringify({ advanced_routes: [route] })}
      onOpenChange={vi.fn()}
      onSave={onSave}
    />
  )
  expect(await screen.findByDisplayValue(route.upstream_path)).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  await waitFor(() => expect(onSave).toHaveBeenCalledOnce())
  const saved = JSON.parse(onSave.mock.calls[0][0]) as AdvancedCustomConfig
  expect(saved.advanced_routes?.[0]).toMatchObject(route)
})

test.each([
  '/v1/chat/completions',
  '/v1/responses/compact',
  '/v1/models',
  '/v1/alpha/search',
])(
  'does not offer or accept the Responses to Claude converter for %s',
  (incomingPath) => {
    expect(
      getAdvancedCustomConverterOptions(incomingPath).map(
        (option) => option.value
      )
    ).not.toContain(converter)
    expect(
      validateAdvancedCustomConfig({
        advanced_routes: [
          {
            incoming_path: incomingPath,
            upstream_path: '/v1/messages',
            converter,
          },
        ],
      })
    ).not.toBeNull()
  }
)
