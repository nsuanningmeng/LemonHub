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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import {
  PromptInput,
  PromptInputAttachment,
  PromptInputAttachments,
  PromptInputProvider,
  PromptInputTextarea,
} from '../prompt-input'

afterEach(() => {
  vi.unstubAllGlobals()
})

test('a long unbroken prompt can wrap and is submitted unchanged with Enter', async () => {
  const text = 'long_prompt_without_spaces_'.repeat(20)
  const onSubmit = vi.fn()
  const user = userEvent.setup()
  render(
    <PromptInput onSubmit={onSubmit}>
      <PromptInputTextarea aria-label='Prompt' />
    </PromptInput>
  )
  const input = screen.getByRole('textbox', { name: 'Prompt' })
  fireEvent.change(input, { target: { value: text } })
  expect(input).toHaveValue(text)
  expect(input).toHaveClass('break-all')
  await user.click(input)
  await user.keyboard('{Enter}')
  await waitFor(() =>
    expect(onSubmit).toHaveBeenCalledWith(
      { text, files: [] },
      expect.anything()
    )
  )
})

test('provider text keeps Shift+Enter and composition separate from submission', async () => {
  const onSubmit = vi.fn()
  const user = userEvent.setup()
  render(
    <PromptInputProvider initialInput='first'>
      <PromptInput onSubmit={onSubmit}>
        <PromptInputTextarea aria-label='Prompt' />
      </PromptInput>
    </PromptInputProvider>
  )
  const input = screen.getByRole('textbox', { name: 'Prompt' })
  await user.click(input)
  await user.keyboard('{End}{Shift>}{Enter}{/Shift}second')
  expect(input).toHaveValue('first\nsecond')
  expect(onSubmit).not.toHaveBeenCalled()
  fireEvent.compositionStart(input)
  fireEvent.keyDown(input, { key: 'Enter', isComposing: true })
  expect(onSubmit).not.toHaveBeenCalled()
  fireEvent.compositionEnd(input)
  await user.keyboard('{Enter}')
  await waitFor(() =>
    expect(onSubmit).toHaveBeenCalledWith(
      { text: 'first\nsecond', files: [] },
      expect.anything()
    )
  )
  await waitFor(() => expect(input).toHaveValue(''))
})

test('an attachment conversion failure preserves the provider prompt and attachment for retry', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockRejectedValue(new Error('blob unavailable'))
  )
  vi.stubGlobal(
    'URL',
    class extends URL {
      static createObjectURL() {
        return 'blob:attachment'
      }
      static revokeObjectURL() {}
    }
  )
  const onSubmit = vi.fn()
  const user = userEvent.setup()
  render(
    <PromptInputProvider initialInput='retry me'>
      <PromptInput onSubmit={onSubmit}>
        <PromptInputTextarea aria-label='Prompt' />
        <PromptInputAttachments>
          {(file) => <PromptInputAttachment data={file} />}
        </PromptInputAttachments>
      </PromptInput>
    </PromptInputProvider>
  )
  await user.upload(
    screen.getByLabelText('Upload files'),
    new File(['note'], 'note.txt', { type: 'text/plain' })
  )
  await user.click(screen.getByRole('textbox', { name: 'Prompt' }))
  await user.keyboard('{Enter}')
  await waitFor(() => expect(fetch).toHaveBeenCalledWith('blob:attachment'))
  expect(onSubmit).not.toHaveBeenCalled()
  expect(screen.getByRole('textbox', { name: 'Prompt' })).toHaveValue(
    'retry me'
  )
  expect(screen.getByText('note.txt')).toBeInTheDocument()
})
