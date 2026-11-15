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
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { toast } from 'sonner'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { SystemInfoSection } from '../system-info-section'

const clients: QueryClient[] = []
const containers: HTMLElement[] = []
afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  containers.splice(0).forEach((container) => container.remove())
  vi.restoreAllMocks()
  localStorage.clear()
})
async function renderSettings(logo: string) {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  clients.push(client)
  const actions = document.createElement('div')
  document.body.append(actions)
  containers.push(actions)
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true } })
  const router = createRouter({
    routeTree: createRootRoute({
      component: () => (
        <QueryClientProvider client={client}>
          <SettingsPageProvider actionsContainer={actions}>
            <SystemInfoSection
              defaultValues={{ SystemName: 'Research', Logo: logo, legal: {} }}
            />
          </SettingsPageProvider>
        </QueryClientProvider>
      ),
    }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(<RouterProvider router={router} />)
  return put
}

test.each(['/lemonhub-mark.svg', '/logo.png'])(
  'the existing relative logo %s allows a system-name change to save',
  async (logo) => {
    const put = await renderSettings(logo)
    fireEvent.change(screen.getByRole('textbox', { name: 'System Name' }), {
      target: { value: 'Research Studio' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
    await waitFor(() =>
      expect(put).toHaveBeenCalledWith(
        '/api/option/',
        {
          key: 'SystemName',
          value: 'Research Studio',
        },
        expect.any(Object)
      )
    )
    expect(put).toHaveBeenCalledTimes(1)
  }
)

test('an absolute custom logo saves without changing unrelated settings', async () => {
  const put = await renderSettings('')
  fireEvent.change(screen.getByRole('textbox', { name: 'Logo URL' }), {
    target: { value: 'https://cdn.example.test/team.svg' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith(
      '/api/option/',
      {
        key: 'Logo',
        value: 'https://cdn.example.test/team.svg',
      },
      expect.any(Object)
    )
  )
})

test('a malformed logo value shows a field error and prevents a save', async () => {
  const put = await renderSettings('')
  const input = screen.getByRole('textbox', { name: 'Logo URL' })
  fireEvent.change(input, { target: { value: 'not a URL' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() => expect(input).toHaveAttribute('aria-invalid', 'true'))
  expect(put).not.toHaveBeenCalled()
})

test.each(['save button', 'form submission'] as const)(
  'a rejected setting from %s retains edits, stops later updates and allows a successful retry',
  async (submission) => {
    const put = await renderSettings('/lemonhub-mark.svg')
    put.mockResolvedValueOnce({
      data: { success: false, message: 'System name rejected' },
    })
    const notify = vi.spyOn(toast, 'error').mockImplementation(() => 0)
    const name = screen.getByRole('textbox', { name: 'System Name' })
    const footer = screen.getByRole('textbox', { name: 'Footer' })
    fireEvent.change(name, { target: { value: 'Research Studio' } })
    fireEvent.change(footer, { target: { value: 'Pending footer' } })
    const form = name.closest('form')
    if (!form) throw new Error('System information form is missing')

    if (submission === 'save button') {
      fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
    } else {
      fireEvent.submit(form)
    }

    await waitFor(() =>
      expect(notify).toHaveBeenCalledWith('System name rejected')
    )
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Save Changes' })).toBeEnabled()
    )
    expect(notify).toHaveBeenCalledTimes(1)
    expect(put).toHaveBeenCalledTimes(1)
    expect(name).toHaveValue('Research Studio')
    expect(footer).toHaveValue('Pending footer')
    expect(screen.getByRole('button', { name: 'Reset' })).toBeEnabled()

    if (submission === 'save button') {
      fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
    } else {
      fireEvent.submit(form)
    }

    await waitFor(() => expect(put).toHaveBeenCalledTimes(3))
    expect(put.mock.calls.map(([, payload]) => payload)).toEqual([
      { key: 'SystemName', value: 'Research Studio' },
      { key: 'SystemName', value: 'Research Studio' },
      { key: 'Footer', value: 'Pending footer' },
    ])
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Reset' })).toBeDisabled()
    )
    expect(notify).toHaveBeenCalledTimes(1)
  }
)
