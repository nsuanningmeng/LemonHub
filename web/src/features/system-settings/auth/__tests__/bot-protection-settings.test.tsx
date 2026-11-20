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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance, type i18n } from 'i18next'
import { useState } from 'react'
import { I18nextProvider } from 'react-i18next'
import { toast } from 'sonner'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import zh from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import {
  getOptionValue,
  useSystemOptions,
} from '../../hooks/use-system-options'
import type { UpdateOptionRequest } from '../../types'
import { BotProtectionSection } from '../bot-protection-section'

const defaults = {
  TurnstileCheckEnabled: false,
  CaptchaProvider: 'turnstile' as const,
  TurnstileSiteKey: '',
  TurnstileSecretKey: '',
  GeetestCaptchaId: '',
  GeetestCaptchaKey: '',
}

let testI18n: i18n
const clients: QueryClient[] = []
const originalAdapter = api.defaults.adapter
afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  api.defaults.adapter = originalAdapter
})

function ConnectedSettings() {
  const options = useSystemOptions()
  return (
    <BotProtectionSection
      defaultValues={getOptionValue(options.data?.data, defaults)}
    />
  )
}

function Fixture({ syncWithServer = false }: { syncWithServer?: boolean }) {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  const [client] = useState(() => {
    const client = new QueryClient({
      defaultOptions: {
        mutations: { retry: false },
        queries: { retry: false },
      },
    })
    clients.push(client)
    return client
  })

  return (
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={client}>
        <div ref={setContainer} />
        <SettingsPageProvider actionsContainer={container}>
          {syncWithServer ? (
            <ConnectedSettings />
          ) : (
            <BotProtectionSection defaultValues={defaults} />
          )}
        </SettingsPageProvider>
      </QueryClientProvider>
    </I18nextProvider>
  )
}

async function renderSettings(syncWithServer = false) {
  const route = createRootRoute({
    component: () => <Fixture syncWithServer={syncWithServer} />,
  })
  const router = createRouter({
    routeTree: route,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(<RouterProvider router={router} />)
}

beforeEach(async () => {
  testI18n = createInstance()
  await testI18n.init({
    lng: 'en',
    fallbackLng: 'en',
    resources: { en: { translation: {} }, zh },
    interpolation: { escapeValue: false },
  })
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
})

describe('Bot Protection settings saves', () => {
  it('saves both keys before enabling Turnstile in one save', async () => {
    const savedOptions: Record<string, string | number | boolean> = {}
    const put = vi.spyOn(api, 'put').mockImplementation(async (_url, body) => {
      const request = body as UpdateOptionRequest
      savedOptions[request.key] = request.value
      return { data: { success: true, message: '' } } as never
    })
    const user = userEvent.setup()
    await renderSettings()
    await user.type(
      screen.getByRole('textbox', { name: 'Site Key' }),
      'site-key'
    )
    await user.type(screen.getByLabelText('Secret Key'), 'secret-key')
    await user.click(
      screen.getByRole('switch', { name: 'Enable human verification' })
    )
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))

    await waitFor(() => expect(put).toHaveBeenCalledTimes(3))
    expect(
      put.mock.calls.map(([, body]) => (body as UpdateOptionRequest).key)
    ).toEqual([
      'TurnstileSiteKey',
      'TurnstileSecretKey',
      'TurnstileCheckEnabled',
    ])
    expect(savedOptions).toEqual({
      TurnstileSiteKey: 'site-key',
      TurnstileSecretKey: 'secret-key',
      TurnstileCheckEnabled: true,
    })
  })
})

it('does not enable verification when saving its credentials is rejected', async () => {
  const put = vi.spyOn(api, 'put').mockImplementation(async (_url, body) => {
    const request = body as UpdateOptionRequest
    return {
      data: {
        success: request.key !== 'TurnstileSecretKey',
        message: 'rejected',
      },
    } as never
  })
  const user = userEvent.setup()
  await renderSettings()
  await user.type(screen.getByRole('textbox', { name: 'Site Key' }), 'new-site')
  await user.type(screen.getByLabelText('Secret Key'), 'new-secret')
  await user.click(
    screen.getByRole('switch', { name: 'Enable human verification' })
  )
  await user.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith(
      '/api/option/',
      {
        key: 'TurnstileSecretKey',
        value: 'new-secret',
      },
      { skipBusinessError: true, skipErrorHandler: true }
    )
  )
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Save Changes' })
    ).not.toBeDisabled()
  )
  expect(
    put.mock.calls.map(([, body]) => (body as UpdateOptionRequest).key)
  ).toEqual(['TurnstileSiteKey', 'TurnstileSecretKey'])
})

it.each(['button', 'form'] as const)(
  'a rejected %s submission retains pending credentials through partial refresh and can retry safely',
  async (entry) => {
    vi.mocked(api.put).mockRestore()
    const saved: Record<string, string | number | boolean> = { ...defaults }
    const requests: UpdateOptionRequest[] = []
    let rejectSecret = true
    api.defaults.adapter = async (config) => {
      let data: unknown
      if (config.method === 'put') {
        const request = JSON.parse(config.data) as UpdateOptionRequest
        requests.push(request)
        if (request.key === 'GeetestCaptchaKey' && rejectSecret) {
          data = { success: false, message: 'Secret key rejected' }
        } else {
          saved[request.key] = request.value
          data = { success: true }
        }
      } else {
        data = {
          success: true,
          data: Object.entries(saved)
            .filter(([key]) => !key.includes('Secret') && !key.endsWith('Key'))
            .map(([key, value]) => ({ key, value: String(value) })),
        }
      }
      return { config, data, status: 200, statusText: 'OK', headers: {} }
    }
    const notify = vi.spyOn(toast, 'error')
    const user = userEvent.setup()
    await renderSettings(true)
    await user.click(
      screen.getByRole('combobox', { name: 'Verification channel' })
    )
    await user.click(screen.getByRole('option', { name: 'GeeTest v4' }))
    const captchaId = screen.getByRole('textbox', { name: 'Captcha ID' })
    await user.type(captchaId, 'new-captcha-id')
    await user.type(screen.getByLabelText('Captcha Key'), 'new-secret')
    await user.click(
      screen.getByRole('switch', { name: 'Enable human verification' })
    )
    if (entry === 'button') {
      await user.click(screen.getByRole('button', { name: 'Save Changes' }))
    } else {
      const form = captchaId.closest('form')
      if (!form) throw new Error('The captcha field must be inside its form')
      fireEvent.submit(form)
    }

    await waitFor(() =>
      expect(notify).toHaveBeenCalledWith('Secret key rejected')
    )
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Save Changes' })).toBeEnabled()
    )
    expect(requests.map(({ key }) => key)).toEqual([
      'CaptchaProvider',
      'GeetestCaptchaId',
      'GeetestCaptchaKey',
    ])
    expect(saved.TurnstileCheckEnabled).toBe(false)
    expect(captchaId).toHaveValue('new-captcha-id')
    expect(screen.getByLabelText('Captcha Key')).toHaveValue('new-secret')
    expect(
      screen.getByRole('switch', { name: 'Enable human verification' })
    ).toBeChecked()
    expect(notify).toHaveBeenCalledTimes(1)

    rejectSecret = false
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))
    await waitFor(() => expect(saved.TurnstileCheckEnabled).toBe(true))
    expect(requests.map(({ key }) => key)).toEqual([
      'CaptchaProvider',
      'GeetestCaptchaId',
      'GeetestCaptchaKey',
      'GeetestCaptchaKey',
      'TurnstileCheckEnabled',
    ])
    expect(saved.GeetestCaptchaKey).toBe('new-secret')
    expect(notify).toHaveBeenCalledTimes(1)
  }
)
