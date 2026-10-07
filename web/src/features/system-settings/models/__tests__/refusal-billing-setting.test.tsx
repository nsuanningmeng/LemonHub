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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { GlobalSettingsCard } from '../global-settings-card'

const refusalLabel = 'Do not charge for Claude refusals before output'

describe('Claude refusal billing setting', () => {
  const clients: QueryClient[] = []
  const actionContainers: HTMLDivElement[] = []

  afterEach(() => {
    for (const client of clients.splice(0)) client.clear()
    for (const actions of actionContainers.splice(0)) actions.remove()
  })

  test.each([false, true])(
    'loads the saved value %s and saves a keyboard toggle without changing other settings',
    async (enabled) => {
      const user = userEvent.setup()
      const client = new QueryClient({
        defaultOptions: { mutations: { retry: false } },
      })
      clients.push(client)
      const actions = document.createElement('div')
      document.body.append(actions)
      actionContainers.push(actions)
      const put = vi.spyOn(api, 'put').mockResolvedValue({
        data: { success: true },
      })
      render(
        <QueryClientProvider client={client}>
          <SettingsPageProvider actionsContainer={actions}>
            <GlobalSettingsCard
              defaultValues={{
                global: {
                  pass_through_request_enabled: false,
                  claude_pre_output_refusal_free_enabled: enabled,
                  thinking_model_blacklist: '[]',
                  chat_completions_to_responses_policy: '{}',
                },
                general_setting: {
                  ping_interval_enabled: false,
                  ping_interval_seconds: 60,
                },
              }}
            />
          </SettingsPageProvider>
        </QueryClientProvider>
      )

      const toggle = screen.getByRole('switch', { name: refusalLabel })
      expect(toggle).toHaveAttribute('aria-checked', String(enabled))
      toggle.focus()
      await user.keyboard(' ')
      expect(toggle).toHaveAttribute('aria-checked', String(!enabled))
      await user.click(screen.getByRole('button', { name: 'Save Changes' }))

      await waitFor(() =>
        expect(put).toHaveBeenCalledExactlyOnceWith('/api/option/', {
          key: 'global.claude_pre_output_refusal_free_enabled',
          value: !enabled,
        })
      )
    }
  )
})
