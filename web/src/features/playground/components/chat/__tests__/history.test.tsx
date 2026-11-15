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

import type { Message } from '../../../types'
import { PlaygroundChat } from '../playground-chat'

const history: Message[] = Array.from({ length: 26 }, (_, index) => ({
  key: `message-${index}`,
  from: 'user',
  versions: [{ id: `version-${index}`, content: `History message ${index}` }],
}))

test('loading earlier history makes the oldest messages and their edit and delete actions available', async () => {
  const user = userEvent.setup()
  const onEditMessage = vi.fn()
  const onDeleteMessage = vi.fn()
  render(
    <PlaygroundChat
      messages={history}
      onEditMessage={onEditMessage}
      onDeleteMessage={onDeleteMessage}
    />
  )

  expect(screen.queryByText('History message 0')).not.toBeInTheDocument()
  await user.click(
    screen.getByRole('button', { name: 'Load earlier messages' })
  )

  expect(screen.getByText('History message 0')).toBeVisible()
  expect(screen.getByText('History message 1')).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'Load earlier messages' })
  ).not.toBeInTheDocument()
  await user.click(screen.getAllByLabelText('Edit')[0])
  expect(onEditMessage).toHaveBeenCalledWith(history[0])
  await user.click(screen.getAllByLabelText('Delete')[0])
  expect(onDeleteMessage).toHaveBeenCalledWith(history[0])
})

test('loading earlier history preserves the scroll distance from the existing messages', async () => {
  const user = userEvent.setup()
  const scrollTop = vi.spyOn(HTMLElement.prototype, 'scrollTop', 'set')
  vi.spyOn(HTMLElement.prototype, 'scrollTop', 'get').mockReturnValue(120)
  vi.spyOn(HTMLElement.prototype, 'scrollHeight', 'get').mockImplementation(
    () => (screen.queryByText('History message 0') ? 2800 : 2400)
  )
  render(<PlaygroundChat messages={history} />)

  await user.click(
    screen.getByRole('button', { name: 'Load earlier messages' })
  )

  expect(scrollTop).toHaveBeenCalledWith(520)
})

test('short and loading conversations do not offer nonexistent earlier messages', () => {
  const view = render(<PlaygroundChat messages={history.slice(0, 1)} />)
  expect(
    screen.queryByRole('button', { name: 'Load earlier messages' })
  ).not.toBeInTheDocument()

  view.rerender(<PlaygroundChat messages={history} isLoadingMessages />)
  expect(screen.getByText('Loading conversation...')).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'Load earlier messages' })
  ).not.toBeInTheDocument()
})

test('appending new messages after loading history keeps the older visible messages available', async () => {
  const user = userEvent.setup()
  const longerHistory: Message[] = Array.from({ length: 52 }, (_, index) => ({
    key: `message-${index}`,
    from: 'user',
    versions: [{ id: `version-${index}`, content: `History message ${index}` }],
  }))
  const view = render(<PlaygroundChat messages={longerHistory.slice(0, 50)} />)
  await user.click(screen.getByText('Load earlier messages'))
  expect(screen.getByText('History message 2')).toBeVisible()

  view.rerender(<PlaygroundChat messages={longerHistory} />)

  expect(screen.getByText('History message 2')).toBeVisible()
  expect(screen.getByText('History message 3')).toBeVisible()
  expect(screen.getByText('History message 51')).toBeVisible()
}, 15_000)
