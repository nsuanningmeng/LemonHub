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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test } from 'vitest'

import { ChannelsProvider, useChannels } from '../../channels-provider'
import { EditTagDialog } from '../edit-tag-dialog'

let client: QueryClient
const originalGetAnimations = Element.prototype.getAnimations

function EditTagFixture() {
  const channels = useChannels()
  return (
    <>
      <button
        type='button'
        onClick={() => {
          channels.setCurrentTag('tagged')
          channels.setOpen('edit-tag')
        }}
      >
        Edit tagged models
      </button>
      <EditTagDialog
        open={channels.open === 'edit-tag'}
        onOpenChange={(open) => channels.setOpen(open ? 'edit-tag' : null)}
      />
    </>
  )
}

beforeEach(() => {
  Object.defineProperty(Element.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  client.setQueryData(['tag-models', 'tagged'], {
    success: true,
    data: 'base-model',
  })
  client.setQueryData(['all-models'], {
    success: true,
    data: [{ id: 'base-model' }],
  })
  client.setQueryData(['groups'], { success: true, data: ['default'] })
})

afterEach(() => {
  if (originalGetAnimations) {
    Object.defineProperty(Element.prototype, 'getAnimations', {
      configurable: true,
      value: originalGetAnimations,
    })
  } else {
    Reflect.deleteProperty(Element.prototype, 'getAnimations')
  }
  client.clear()
  localStorage.clear()
})

test.each([
  { isComposing: true, keyCode: 13 },
  { isComposing: false, keyCode: 229 },
])(
  'confirming a custom model candidate ($isComposing/$keyCode) does not add a partial model',
  async (event) => {
    const user = userEvent.setup()
    render(
      <QueryClientProvider client={client}>
        <ChannelsProvider>
          <EditTagFixture />
        </ChannelsProvider>
      </QueryClientProvider>
    )
    await user.click(screen.getByRole('button', { name: 'Edit tagged models' }))
    const input = await screen.findByPlaceholderText(
      'Custom model (comma-separated)'
    )
    fireEvent.change(input, { target: { value: 'zhong' } })
    fireEvent.keyDown(input, { key: 'Enter', ...event })
    expect(input).toHaveValue('zhong')
    expect(screen.queryByText('zhong ×')).not.toBeInTheDocument()
    fireEvent.change(input, { target: { value: '中文' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(input).toHaveValue('')
    expect(screen.getByText('中文 ×')).toBeInTheDocument()
  }
)
