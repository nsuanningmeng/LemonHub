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
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { toast } from 'sonner'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { AudioPreviewDialog } from '../audio-preview-dialog'

const clipboardDescriptor = Object.getOwnPropertyDescriptor(
  navigator,
  'clipboard'
)
const execCommandDescriptor = Object.getOwnPropertyDescriptor(
  document,
  'execCommand'
)
const animationsDescriptor = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'getAnimations'
)
const audioUrl = 'https://example.com/audio.mp3'

beforeEach(() => {
  Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
  vi.spyOn(toast, 'success').mockImplementation(() => 'copy-success')
  vi.spyOn(toast, 'error').mockImplementation(() => 'copy-error')
  Object.defineProperty(document, 'execCommand', {
    configurable: true,
    value: vi.fn(() => false),
  })
})

afterEach(() => {
  if (animationsDescriptor) {
    Object.defineProperty(
      HTMLElement.prototype,
      'getAnimations',
      animationsDescriptor
    )
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, 'getAnimations')
  }
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

function renderFailedAudio(): void {
  render(
    <AudioPreviewDialog
      open
      onOpenChange={() => undefined}
      clips={[{ id: 'sample', title: 'Sample audio', audio_url: audioUrl }]}
    />
  )
  const audio = document.querySelector('audio')
  if (!audio) throw new Error('Audio player was not rendered')
  fireEvent.error(audio)
}

test('audio link copying shows success only after the clipboard write completes', async () => {
  const user = userEvent.setup()
  let completeCopy: () => void = () => undefined
  const pending = new Promise<void>((resolve) => {
    completeCopy = resolve
  })
  const writeText = vi
    .spyOn(navigator.clipboard, 'writeText')
    .mockReturnValue(pending)
  renderFailedAudio()

  await user.click(screen.getByRole('button', { name: 'Copy Link' }))

  expect(writeText).toHaveBeenCalledWith(audioUrl)
  expect(toast.success).not.toHaveBeenCalled()
  await act(async () => completeCopy())
  expect(toast.success).toHaveBeenCalledWith('Copied')
})

test('denied audio link copying gives actionable failure feedback without reporting success', async () => {
  const user = userEvent.setup()
  vi.spyOn(navigator.clipboard, 'writeText').mockRejectedValue(
    new DOMException('Denied', 'NotAllowedError')
  )
  vi.spyOn(console, 'warn').mockImplementation(() => undefined)
  renderFailedAudio()

  await user.click(screen.getByRole('button', { name: 'Copy Link' }))

  await waitFor(() =>
    expect(toast.error).toHaveBeenCalledWith(
      'Copy failed. Please copy manually.'
    )
  )
  expect(toast.success).not.toHaveBeenCalled()
})

test('audio link copying uses the compatibility fallback when the Clipboard API is unavailable', async () => {
  const user = userEvent.setup()
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: undefined,
  })
  const fallback = vi.spyOn(document, 'execCommand').mockReturnValue(true)
  renderFailedAudio()

  await user.click(screen.getByRole('button', { name: 'Copy Link' }))

  expect(fallback).toHaveBeenCalledWith('copy')
  expect(toast.success).toHaveBeenCalledWith('Copied')
  expect(toast.error).not.toHaveBeenCalled()
})
