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
import { expect, it } from 'vitest'

import { chatLinkRequiresApiKey, resolveChatUrl } from '../chat-links'

it('requires an API key for Chatbox presets and embeds the selected key in the configuration', () => {
  const template = 'chatbox://api/providers/import?config={chatboxConfig}'
  expect(chatLinkRequiresApiKey(template)).toBe(true)
  const url = new URL(
    resolveChatUrl({
      template,
      apiKey: 'chosen-key',
      serverAddress: 'https://gateway.example',
    })
  )
  const encoded = url.searchParams.get('config')
  expect(encoded).not.toBeNull()
  expect(JSON.parse(atob(encoded ?? ''))).toMatchObject({
    settings: { apiKey: 'sk-chosen-key', apiHost: 'https://gateway.example' },
  })
})

it('does not request credentials for a preset containing only the server address', () => {
  expect(chatLinkRequiresApiKey('https://chat.example/?server={address}')).toBe(
    false
  )
})
