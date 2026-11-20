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
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import type { AxiosAdapter } from 'axios'
import { toast } from 'sonner'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { AnnouncementsSection } from '../announcements-section'
import { ApiInfoSection } from '../api-info-section'
import { FAQSection } from '../faq-section'
import { UptimeKumaSection } from '../uptime-kuma-section'

const originalAdapter = api.defaults.adapter
const clients: QueryClient[] = []
afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  api.defaults.adapter = originalAdapter
  vi.restoreAllMocks()
  localStorage.clear()
})
const sections = [
  {
    name: 'API info',
    Component: ApiInfoSection,
    key: 'api_info',
    add: 'Add API',
    fields: [
      ['API URL', 'https://gateway.example.test'],
      ['Route Description', 'Direct'],
      ['Description', 'Draft gateway'],
    ],
  },
  {
    name: 'announcements',
    Component: AnnouncementsSection,
    key: 'announcements',
    add: 'Add Announcement',
    fields: [['Content', 'Draft announcement']],
  },
  {
    name: 'FAQ',
    Component: FAQSection,
    key: 'faq',
    add: 'Add FAQ',
    fields: [
      ['Question', 'Draft question'],
      ['Answer', 'Draft answer'],
    ],
  },
  {
    name: 'uptime groups',
    Component: UptimeKumaSection,
    key: 'uptime_kuma',
    add: 'Add Group',
    fields: [
      ['Category Name', 'Draft monitors'],
      ['Uptime Kuma URL', 'https://status.example.test'],
      ['Status Page Slug', 'public'],
    ],
  },
] as const

function renderSection(Component: typeof ApiInfoSection) {
  const client = new QueryClient({
    defaultOptions: {
      queries: { enabled: false, retry: false },
      mutations: { retry: false },
    },
  })
  clients.push(client)
  const request = vi.fn<AxiosAdapter>(async (config) => ({
    config,
    status: 200,
    statusText: 'OK',
    headers: {},
    data: { success: false, message: 'Content update rejected' },
  }))
  api.defaults.adapter = request
  const notify = vi.spyOn(toast, 'error').mockImplementation(() => 0)
  render(
    <QueryClientProvider client={client}>
      <Component enabled={false} data='[]' />
    </QueryClientProvider>
  )
  return { request, notify }
}

test.each(sections)(
  'a rejected $name toggle reports once and keeps its saved state',
  async ({ Component, key }) => {
    const { request, notify } = renderSection(Component)
    const toggle = screen.getByRole('switch')
    fireEvent.click(toggle)
    await waitFor(() => expect(request).toHaveBeenCalledTimes(1))
    await waitFor(() =>
      expect(notify).toHaveBeenCalledWith('Content update rejected')
    )
    expect(notify).toHaveBeenCalledTimes(1)
    expect(JSON.parse(request.mock.calls[0][0].data)).toEqual({
      key: `console_setting.${key}_enabled`,
      value: true,
    })
    expect(toggle).not.toBeChecked()
  }
)

test.each(sections)(
  'a rejected $name save reports once and retains the draft for retry',
  async ({ Component, add, fields }) => {
    const { request, notify } = renderSection(Component)
    fireEvent.click(screen.getByRole('button', { name: add }))
    for (const [name, value] of fields) {
      fireEvent.change(await screen.findByRole('textbox', { name }), {
        target: { value },
      })
    }
    fireEvent.click(screen.getByRole('button', { name: /^Add$/ }))
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    const save = screen.getByRole('button', { name: 'Save Settings' })
    expect(save).toBeEnabled()
    fireEvent.click(save)
    await waitFor(() =>
      expect(notify).toHaveBeenCalledWith('Content update rejected')
    )
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Save Settings' })
      ).toBeEnabled()
    )
    expect(request).toHaveBeenCalledTimes(1)
    expect(notify).toHaveBeenCalledTimes(1)
    expect(screen.getByText(fields[0][1])).toBeInTheDocument()
  }
)
