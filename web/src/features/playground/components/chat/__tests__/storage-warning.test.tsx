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
import { afterEach, expect, test, vi } from 'vitest'

import type { Message } from '../../../types'
import { PlaygroundStorageWarning } from '../playground-storage-warning'

const messages: Message[] = [
  {
    key: 'account-a',
    from: 'assistant',
    versions: [
      { id: 'a-current', content: 'Account A full response' },
      { id: 'a-old', content: 'Earlier revision' },
    ],
    reasoning: { content: 'Saved reasoning', duration: 1 },
  },
]

afterEach(() => {
  vi.unstubAllGlobals()
})

test('the save-failure alert provides a keyboard-accessible retry action', async () => {
  const user = userEvent.setup()
  const onRetry = vi.fn()
  render(<PlaygroundStorageWarning messages={messages} onRetry={onRetry} />)

  expect(screen.getByRole('alert')).toHaveTextContent(
    'Conversation could not be saved in this browser. Download a copy before leaving.'
  )
  await user.tab()
  expect(
    screen.getByRole('button', { name: 'Download conversation' })
  ).toHaveFocus()
  await user.tab()
  expect(screen.getByRole('button', { name: 'Retry saving' })).toHaveFocus()
  await user.keyboard('{Enter}')

  expect(onRetry).toHaveBeenCalledOnce()
})

test('downloading after switching conversations uses only the current full message snapshot', async () => {
  const user = userEvent.setup()
  const createObjectURL = vi.fn<(blob: Blob | MediaSource) => string>(
    () => 'blob:conversation-copy'
  )
  const revokeObjectURL = vi.fn<(url: string) => void>()
  class DownloadURL extends URL {
    static createObjectURL = createObjectURL
    static revokeObjectURL = revokeObjectURL
  }
  vi.stubGlobal('URL', DownloadURL)
  const download = vi
    .spyOn(HTMLAnchorElement.prototype, 'click')
    .mockImplementation(() => undefined)
  const view = render(
    <PlaygroundStorageWarning messages={messages} onRetry={() => undefined} />
  )
  const accountB: Message[] = [
    {
      key: 'account-b',
      from: 'assistant',
      versions: [
        { id: 'b-current', content: 'Account B full response' },
        { id: 'b-old', content: 'Account B earlier revision' },
      ],
      reasoning: { content: 'Account B reasoning', duration: 2 },
    },
  ]
  view.rerender(
    <PlaygroundStorageWarning messages={accountB} onRetry={() => undefined} />
  )

  await user.click(
    screen.getByRole('button', { name: 'Download conversation' })
  )

  expect(createObjectURL).toHaveBeenCalledOnce()
  const blob = createObjectURL.mock.calls[0][0] as Blob
  expect(blob.type).toBe('application/json')
  const text = await new Promise<string>((resolve, reject) => {
    const reader = new FileReader()
    reader.addEventListener('load', () => resolve(String(reader.result)))
    reader.addEventListener('error', () => reject(reader.error))
    reader.readAsText(blob)
  })
  expect(JSON.parse(text)).toEqual(accountB)
  expect(download).toHaveBeenCalledOnce()
  expect(download.mock.contexts[0]).toHaveAttribute(
    'download',
    'playground-conversation.json'
  )
  expect(download.mock.contexts[0]).toHaveAttribute(
    'href',
    'blob:conversation-copy'
  )
  await waitFor(() =>
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:conversation-copy')
  )
})
