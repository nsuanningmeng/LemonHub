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
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { copyToClipboard } from '../copy-to-clipboard'

const clipboardDescriptor = Object.getOwnPropertyDescriptor(
  navigator,
  'clipboard'
)
const execCommandDescriptor = Object.getOwnPropertyDescriptor(
  document,
  'execCommand'
)

beforeEach(() => {
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: undefined,
  })
  Object.defineProperty(document, 'execCommand', {
    configurable: true,
    value: vi.fn(() => true),
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
  document.body.replaceChildren()
})

test.each([true, false])(
  'restores the copy control focus when the fallback returns %s',
  async (result) => {
    const button = document.createElement('button')
    button.textContent = 'Copy link'
    document.body.append(button)
    button.focus()
    vi.spyOn(document, 'execCommand').mockReturnValue(result)

    expect(await copyToClipboard('https://example.com/audio.mp3')).toBe(result)

    expect(button).toHaveFocus()
    expect(document.querySelector('textarea')).not.toBeInTheDocument()
  }
)

test('restores the copy control focus when the fallback throws', async () => {
  const button = document.createElement('button')
  document.body.append(button)
  button.focus()
  vi.spyOn(document, 'execCommand').mockImplementation(() => {
    throw new Error('Copy denied')
  })
  vi.spyOn(console, 'error').mockImplementation(() => undefined)

  expect(await copyToClipboard('sample')).toBe(false)

  expect(button).toHaveFocus()
  expect(document.querySelector('textarea')).not.toBeInTheDocument()
})

test('does not restore focus to a control removed during fallback copying', async () => {
  const button = document.createElement('button')
  document.body.append(button)
  button.focus()
  const focus = vi.spyOn(button, 'focus')
  vi.spyOn(document, 'execCommand').mockImplementation(() => {
    button.remove()
    return true
  })

  expect(await copyToClipboard('sample')).toBe(true)

  expect(focus).not.toHaveBeenCalled()
  expect(document.querySelector('textarea')).not.toBeInTheDocument()
})

test('does not change focus when the native clipboard API succeeds', async () => {
  const button = document.createElement('button')
  document.body.append(button)
  button.focus()
  const focus = vi.spyOn(button, 'focus')
  const writeText = vi.fn().mockResolvedValue(undefined)
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText },
  })

  expect(await copyToClipboard('sample')).toBe(true)

  expect(writeText).toHaveBeenCalledWith('sample')
  expect(button).toHaveFocus()
  expect(focus).not.toHaveBeenCalled()
  expect(document.execCommand).not.toHaveBeenCalled()
})
