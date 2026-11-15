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
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { toast } from 'sonner'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { CodeBlock, CodeBlockCopyButton } from '../code-block'

const clipboardDescriptor = Object.getOwnPropertyDescriptor(
  navigator,
  'clipboard'
)
const execCommandDescriptor = Object.getOwnPropertyDescriptor(
  document,
  'execCommand'
)

beforeEach(() => {
  vi.spyOn(toast, 'error').mockImplementation(() => 'copy-error')
  Object.defineProperty(document, 'execCommand', {
    configurable: true,
    value: vi.fn(() => false),
  })
})

afterEach(() => {
  if (clipboardDescriptor) {
    Object.defineProperty(navigator, 'clipboard', clipboardDescriptor)
  } else {
    Reflect.deleteProperty(navigator, 'clipboard')
  }
  if (execCommandDescriptor) {
    Object.defineProperty(document, 'execCommand', execCommandDescriptor)
  } else {
    Reflect.deleteProperty(document, 'execCommand')
  }
})

test('copying code waits for clipboard completion before showing success or notifying callers', async () => {
  const user = userEvent.setup()
  let completeCopy: () => void = () => undefined
  const pending = new Promise<void>((resolve) => {
    completeCopy = resolve
  })
  const writeText = vi
    .spyOn(navigator.clipboard, 'writeText')
    .mockReturnValue(pending)
  const onCopy = vi.fn()
  render(
    <CodeBlock code='const answer = 42' language='plaintext'>
      <CodeBlockCopyButton onCopy={onCopy} />
    </CodeBlock>
  )

  await user.click(screen.getByRole('button', { name: 'Copy code' }))
  expect(writeText).toHaveBeenCalledWith('const answer = 42')
  expect(screen.getByRole('button', { name: 'Copy code' })).toBeVisible()
  expect(onCopy).not.toHaveBeenCalled()

  await act(async () => completeCopy())
  expect(screen.getByRole('button', { name: 'Copied!' })).toBeVisible()
  expect(onCopy).toHaveBeenCalledOnce()
})

test('denied code copying shows actionable feedback without requiring an onError callback', async () => {
  const user = userEvent.setup()
  vi.spyOn(navigator.clipboard, 'writeText').mockRejectedValue(
    new DOMException('Denied', 'NotAllowedError')
  )
  vi.spyOn(console, 'warn').mockImplementation(() => undefined)
  render(
    <CodeBlock code='const answer = 42' language='plaintext'>
      <CodeBlockCopyButton />
    </CodeBlock>
  )

  await user.click(screen.getByRole('button', { name: 'Copy code' }))

  await waitFor(() =>
    expect(toast.error).toHaveBeenCalledWith(
      'Copy failed. Please copy manually.'
    )
  )
  expect(screen.getByRole('button', { name: 'Copy code' })).toBeVisible()
})

test('code copying uses the compatibility fallback when the Clipboard API is unavailable', async () => {
  const user = userEvent.setup()
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: undefined,
  })
  const fallback = vi.spyOn(document, 'execCommand').mockReturnValue(true)
  render(
    <CodeBlock code='const answer = 42' language='plaintext'>
      <CodeBlockCopyButton />
    </CodeBlock>
  )

  await user.click(screen.getByRole('button', { name: 'Copy code' }))

  expect(fallback).toHaveBeenCalledWith('copy')
  expect(screen.getByRole('button', { name: 'Copied!' })).toBeVisible()
  expect(toast.error).not.toHaveBeenCalled()
})

test('code copying reports failure to the caller and user when all clipboard methods fail', async () => {
  const user = userEvent.setup()
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: undefined,
  })
  const onError = vi.fn()
  render(
    <CodeBlock code='const answer = 42' language='plaintext'>
      <CodeBlockCopyButton onError={onError} />
    </CodeBlock>
  )

  await user.click(screen.getByRole('button', { name: 'Copy code' }))

  await waitFor(() =>
    expect(toast.error).toHaveBeenCalledWith(
      'Copy failed. Please copy manually.'
    )
  )
  expect(onError).toHaveBeenCalledWith(expect.any(Error))
  expect(screen.getByRole('button', { name: 'Copy code' })).toBeVisible()
})
