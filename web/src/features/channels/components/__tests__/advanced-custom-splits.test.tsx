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

import type { AdvancedCustomConfig } from '../../types'
import { AdvancedCustomEditorDialog } from '../dialogs/advanced-custom-editor-dialog'

test('expanded forwarding routes expose Add split and save the new fallback without losing existing routes', async () => {
  const config = {
    advanced_routes: [
      {
        incoming_path: '/v1/chat/completions',
        upstream_path: '/provider/chat',
        converter: 'none',
        models: ['special-model'],
        auth: { type: 'header', name: 'x-api-key', value: '{{key}}' },
      },
      {
        incoming_path: '/v1/models',
        upstream_path: '/provider/models',
        converter: 'none',
      },
    ],
  } satisfies AdvancedCustomConfig
  const onSave = vi.fn()
  const user = userEvent.setup()
  render(
    <AdvancedCustomEditorDialog
      open
      value={JSON.stringify(config)}
      onOpenChange={vi.fn()}
      onSave={onSave}
    />
  )
  await user.click(await screen.findByRole('button', { name: 'Add split' }))
  expect(
    screen.getByRole('tab', { name: /Forwarding Routes/ })
  ).toHaveTextContent('2')
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  await waitFor(() => expect(onSave).toHaveBeenCalledOnce())
  const saved = JSON.parse(onSave.mock.calls[0][0]) as AdvancedCustomConfig
  expect(saved.advanced_routes).toHaveLength(3)
  expect(saved.advanced_routes?.[0]).toMatchObject(config.advanced_routes[0])
  expect(saved.advanced_routes?.[1]).toMatchObject(config.advanced_routes[1])
  expect(saved.advanced_routes?.[2]).toMatchObject({
    incoming_path: '/v1/chat/completions',
    upstream_path: '/v1/chat/completions',
    converter: 'none',
  })
})

test('model discovery routes do not offer splitting and preserve their saved configuration', async () => {
  const config = {
    advanced_routes: [
      {
        incoming_path: '/v1/models',
        upstream_path: '/provider/models',
        converter: 'none',
      },
    ],
  } satisfies AdvancedCustomConfig
  const onSave = vi.fn()
  const user = userEvent.setup()
  render(
    <AdvancedCustomEditorDialog
      open
      value={JSON.stringify(config)}
      onOpenChange={vi.fn()}
      onSave={onSave}
    />
  )
  await user.click(screen.getByRole('tab', { name: /Model List/ }))
  expect(
    screen.queryByRole('button', { name: 'Add split' })
  ).not.toBeInTheDocument()
  expect(screen.getByDisplayValue('/provider/models')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  await waitFor(() => expect(onSave).toHaveBeenCalledOnce())
  const saved = JSON.parse(onSave.mock.calls[0][0]) as AdvancedCustomConfig
  expect(saved.advanced_routes).toHaveLength(1)
  expect(saved.advanced_routes?.[0]).toMatchObject(config.advanced_routes[0])
})

test('adding a split after an existing fallback blocks saving duplicate fallbacks', async () => {
  const config = {
    advanced_routes: [
      {
        incoming_path: '/v1/chat/completions',
        upstream_path: '/existing/fallback',
        converter: 'none',
      },
    ],
  } satisfies AdvancedCustomConfig
  const onSave = vi.fn()
  const user = userEvent.setup()
  render(
    <AdvancedCustomEditorDialog
      open
      value={JSON.stringify(config)}
      onOpenChange={vi.fn()}
      onSave={onSave}
    />
  )
  await user.click(await screen.findByRole('button', { name: 'Add split' }))
  expect(screen.getByRole('alert')).toHaveTextContent(
    'Only one catch-all route is allowed for the same incoming path'
  )
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  expect(onSave).not.toHaveBeenCalled()
  expect(screen.getByDisplayValue('/existing/fallback')).toBeVisible()
})
