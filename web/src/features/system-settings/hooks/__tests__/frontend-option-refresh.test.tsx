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
import {
  QueryClient,
  QueryClientProvider,
  useQuery,
} from '@tanstack/react-query'
import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import type { PropsWithChildren } from 'react'
import { toast } from 'sonner'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { useStatus } from '@/hooks/use-status'
import { api } from '@/lib/api'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { useSettingsForm } from '../use-settings-form'
import { useUpdateOption } from '../use-update-option'

let client: QueryClient
const originalAdapter = api.defaults.adapter
const originalConfig = useSystemConfigStore.getState()

function Wrapper(props: PropsWithChildren) {
  return (
    <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
  )
}

beforeEach(() => {
  localStorage.clear()
  client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Infinity },
      mutations: { retry: false },
    },
  })
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
})

afterEach(() => {
  cleanup()
  api.defaults.adapter = originalAdapter
  client.clear()
  localStorage.clear()
  useSystemConfigStore.setState(originalConfig, true)
})

test.each([
  ['SystemName', 'Updated workspace', 'system_name'],
  ['Logo', '/custom-logo.svg', 'logo'],
  ['Footer', 'Updated footer', 'footer_html'],
  ['ServerAddress', 'https://gateway.example.test', 'server_address'],
  ['DefaultCollapseSidebar', true, 'default_collapse_sidebar'],
  ['general_setting.docs_link', 'https://docs.example.test', 'docs_link'],
  ['Chats', '[{"Assistant":"https://chat.example.test"}]', 'chats'],
  ['console_setting.api_info_enabled', false, 'api_info_enabled'],
  ['console_setting.api_info', '[]', 'api_info'],
  ['console_setting.announcements_enabled', false, 'announcements_enabled'],
  ['console_setting.announcements', '[]', 'announcements'],
  ['console_setting.faq_enabled', false, 'faq_enabled'],
  ['console_setting.faq', '[]', 'faq'],
  ['console_setting.uptime_kuma_enabled', false, 'uptime_kuma_enabled'],
  ['PasswordLoginEnabled', false, 'password_login_enabled'],
  ['RegisterEnabled', false, 'register_enabled'],
  ['discord.enabled', false, 'discord_oauth'],
  ['oidc.enabled', false, 'oidc_enabled'],
  ['passkey.enabled', false, 'passkey_login'],
  ['legal.user_agreement', 'Updated agreement', 'user_agreement_enabled'],
  ['legal.privacy_policy', 'Updated policy', 'privacy_policy_enabled'],
] as const)(
  'saving %s refreshes mounted frontend status consumers and persisted status',
  async (key, value, field) => {
    const original = { system_name: 'Original workspace', [field]: 'old value' }
    const updated = { system_name: 'Original workspace', [field]: value }
    client.setQueryData(['status'], original)
    localStorage.setItem('status', JSON.stringify(original))
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: updated },
    })
    const { result } = renderHook(
      () => ({ status: useStatus().status, update: useUpdateOption() }),
      { wrapper: Wrapper }
    )

    await act(async () => {
      await result.current.update.mutateAsync({ key, value })
    })

    await waitFor(() => expect(result.current.status).toEqual(updated))
    expect(JSON.parse(localStorage.getItem('status') || 'null')).toEqual(
      updated
    )
  }
)

test.each([
  ['Notice', 'notice', undefined],
  ['HomePageContent', 'home-page-content', 'home_page_content'],
  ['About', 'about-content', undefined],
  ['Contact', 'contact-content', undefined],
  ['legal.user_agreement', 'user-agreement', undefined],
  ['legal.privacy_policy', 'privacy-policy', undefined],
  ['console_setting.uptime_kuma_groups', 'uptime-status', undefined],
] as const)(
  'saving %s refreshes its independent content query',
  async (key, queryKey, storageKey) => {
    client.setQueryData([queryKey], 'previous content')
    if (storageKey) localStorage.setItem(storageKey, 'previous content')
    const { result } = renderHook(
      () => ({
        content: useQuery({
          queryKey: [queryKey],
          queryFn: async () => 'updated content',
        }).data,
        update: useUpdateOption(),
      }),
      { wrapper: Wrapper }
    )

    await act(async () => {
      await result.current.update.mutateAsync({ key, value: 'updated content' })
    })

    await waitFor(() => expect(result.current.content).toBe('updated content'))
    if (storageKey) expect(localStorage.getItem(storageKey)).toBeNull()
  }
)

test('a rejected settings response preserves current frontend settings and caches', async () => {
  const status = { system_name: 'Existing workspace' }
  client.setQueryData(['status'], status)
  localStorage.setItem('status', JSON.stringify(status))
  vi.spyOn(api, 'put').mockResolvedValue({
    data: { success: false, message: 'Change rejected' },
  })
  const get = vi.spyOn(api, 'get')
  const { result } = renderHook(
    () => ({ status: useStatus().status, update: useUpdateOption() }),
    { wrapper: Wrapper }
  )

  await act(async () => {
    await expect(
      result.current.update.mutateAsync({
        key: 'SystemName',
        value: 'Rejected workspace',
      })
    ).rejects.toThrow('Change rejected')
  })

  expect(result.current.status).toEqual(status)
  expect(get).not.toHaveBeenCalled()
  expect(JSON.parse(localStorage.getItem('status') || 'null')).toEqual(status)
})

test('a business rejection keeps unsaved form changes, preserves the reset baseline, and shows one error', async () => {
  vi.mocked(api.put).mockRestore()
  api.defaults.adapter = async (config) => ({
    config,
    data: { success: false, message: 'Setting rejected' },
    status: 200,
    statusText: 'OK',
    headers: {},
  })
  const notify = vi.spyOn(toast, 'error')
  const { result } = renderHook(
    () => {
      const update = useUpdateOption()
      return useSettingsForm({
        defaultValues: { SystemName: 'Saved name' },
        onSubmit: async (data) => {
          await update.mutateAsync({
            key: 'SystemName',
            value: data.SystemName,
          })
        },
      })
    },
    { wrapper: Wrapper }
  )
  act(() =>
    result.current.form.setValue('SystemName', 'Unsaved name', {
      shouldDirty: true,
    })
  )

  await act(async () => {
    await expect(result.current.handleSubmit()).rejects.toThrow(
      'Setting rejected'
    )
  })

  expect(result.current.isDirty).toBe(true)
  expect(result.current.form.getValues('SystemName')).toBe('Unsaved name')
  expect(notify).toHaveBeenCalledTimes(1)
  expect(notify).toHaveBeenCalledWith('Setting rejected')
  act(() => result.current.handleReset())
  expect(result.current.form.getValues('SystemName')).toBe('Saved name')
})
