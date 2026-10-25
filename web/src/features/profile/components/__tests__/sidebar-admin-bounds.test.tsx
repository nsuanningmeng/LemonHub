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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { useSidebarConfig } from '@/hooks/use-sidebar-config'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { SidebarModulesCard } from '../sidebar-modules-card'

afterEach(() => {
  useAuthStore.getState().auth.reset()
})

function Navigation() {
  const groups = useSidebarConfig([
    { title: 'Chat', items: [{ title: 'Chat', url: '/playground' }] },
  ])
  return (
    <div data-testid='navigation'>
      {groups
        .flatMap((g) => g.items)
        .map((item) => item.title)
        .join(',')}
    </div>
  )
}
test.each([false, true])(
  'admin chat section enabled=%s bounds personal reset/save and preserves hidden preferences',
  async (sectionEnabled) => {
    const client = new QueryClient()
    client.setQueryData(['status'], {
      SidebarModulesAdmin: JSON.stringify({
        chat: {
          enabled: sectionEnabled,
          chat: false,
          playground: sectionEnabled,
        },
      }),
    })
    const config = {
      chat: { enabled: true, chat: false, playground: true },
      console: { enabled: true, detail: true },
    }
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: { sidebar_modules: JSON.stringify(config) },
      },
    })
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    const rendered = render(
      <QueryClientProvider client={client}>
        <SidebarModulesCard />
        <Navigation />
      </QueryClientProvider>
    )
    await screen.findByText('Console Area')
    await waitFor(() => expect(api.get).toHaveBeenCalledWith('/api/user/self'))
    if (!sectionEnabled) expect(screen.queryByText('Chat Area')).toBeNull()
    expect(screen.queryByText('Chat session management')).toBeNull()
    if (!sectionEnabled) {
      expect(screen.getByTestId('navigation')).toBeEmptyDOMElement()
    }
    fireEvent.click(screen.getByRole('button', { name: 'Reset to Default' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
    await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
    const payload = put.mock.calls[0]?.[1] as { sidebar_modules: string }
    expect(JSON.parse(payload.sidebar_modules).chat).toEqual(config.chat)
    if (!sectionEnabled) {
      expect(screen.getByTestId('navigation')).toBeEmptyDOMElement()
    }
    rendered.unmount()
    client.clear()
  }
)

function RewardsNavigation() {
  const groups = useSidebarConfig([
    {
      title: 'Personal',
      items: [{ title: 'My Rewards', url: '/rewards' }],
    },
  ])
  return (
    <nav>
      {groups.flatMap((group) =>
        group.items.map((item) => (
          <a key={item.url} href={item.url}>
            {item.title}
          </a>
        ))
      )}
    </nav>
  )
}

test('saving the personal rewards preference updates navigation and preserves other modules', async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  client.setQueryData(['status'], {})
  const config = { personal: { enabled: true, topup: true } }
  useAuthStore.getState().auth.setUser({
    id: 1,
    role: ROLE.USER,
    username: 'member',
    sidebar_modules: JSON.stringify(config),
  })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { sidebar_modules: JSON.stringify(config) } },
  })
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true } })
  const rendered = render(
    <QueryClientProvider client={client}>
      <SidebarModulesCard />
      <RewardsNavigation />
    </QueryClientProvider>
  )
  const rewardsSwitch = await screen.findByRole('switch', {
    name: 'My Rewards',
  })
  expect(rewardsSwitch).toBeChecked()
  expect(screen.getByRole('link', { name: 'My Rewards' })).toBeVisible()

  fireEvent.click(rewardsSwitch)
  expect(rewardsSwitch).not.toBeChecked()
  fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }))

  await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
  const payload = put.mock.calls[0]?.[1] as { sidebar_modules: string }
  expect(JSON.parse(payload.sidebar_modules).personal).toEqual({
    enabled: true,
    topup: true,
    rewards: false,
  })
  await waitFor(() =>
    expect(screen.queryByRole('link', { name: 'My Rewards' })).toBeNull()
  )
  rendered.unmount()
  client.clear()
})

test('an administrator-disabled rewards module cannot be restored in personal sidebar settings', async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  client.setQueryData(['status'], {
    SidebarModulesAdmin: JSON.stringify({
      personal: { enabled: true, rewards: false },
    }),
  })
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: {} } })
  const rendered = render(
    <QueryClientProvider client={client}>
      <SidebarModulesCard />
    </QueryClientProvider>
  )
  await screen.findByRole('switch', { name: 'Wallet Management' })
  expect(screen.queryByRole('switch', { name: 'My Rewards' })).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: 'Reset to Default' }))
  expect(screen.queryByRole('switch', { name: 'My Rewards' })).toBeNull()
  rendered.unmount()
  client.clear()
})
