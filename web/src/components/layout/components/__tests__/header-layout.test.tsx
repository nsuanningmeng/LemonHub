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
import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test } from 'vitest'

import { CommandMenu } from '@/components/command-menu'
import { Search } from '@/components/search'
import { SidebarProvider } from '@/components/ui/sidebar'
import { SearchProvider } from '@/context/search-provider'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { Header } from '../header'
import { SystemBrand } from '../system-brand'
import { TopNav } from '../top-nav'

const animationsDescriptor = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'getAnimations'
)

const originalConfig = useSystemConfigStore.getState()
let client: QueryClient
const siteName = 'LemonHub Enterprise API Gateway with a long name'

function HeaderFixture() {
  return (
    <SidebarProvider>
      <SearchProvider>
        <Header>
          <SystemBrand variant='inline' />
          <TopNav
            links={[
              { title: 'Model marketplace', href: '/pricing' },
              {
                title: 'Documentation',
                href: 'https://example.com/docs',
                external: true,
              },
            ]}
          />
          <Search />
        </Header>
        <CommandMenu />
      </SearchProvider>
    </SidebarProvider>
  )
}

beforeEach(() => {
  Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
  localStorage.clear()
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  client.setQueryData(['status'], { system_name: siteName })
})

afterEach(() => {
  cleanup()
  if (animationsDescriptor) {
    Object.defineProperty(
      HTMLElement.prototype,
      'getAnimations',
      animationsDescriptor
    )
  } else Reflect.deleteProperty(HTMLElement.prototype, 'getAnimations')
  client.clear()
  useSystemConfigStore.setState(originalConfig, true)
  localStorage.clear()
})

async function renderHeader() {
  const router = createRouter({
    routeTree: createRootRoute({ component: HeaderFixture }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  await screen.findByRole('banner')
}

test('a long inline site name can shrink while the logo keeps its size', async () => {
  await renderHeader()
  const brand = screen.getByRole('link', { name: 'Go to home' })
  expect(brand).toHaveClass('min-w-0')
  expect(within(brand).getByAltText('Logo').parentElement).toHaveClass(
    'shrink-0'
  )
  expect(screen.getByText(siteName)).toHaveClass('min-w-0', 'truncate')
})

test('mobile search keeps an accessible name while the text and keyboard hint wait for wider screens', async () => {
  await renderHeader()
  const search = screen.getByRole('button', { name: 'Search' })
  expect(search).toHaveClass('size-8', 'shrink-0')
  expect(within(search).getByText('Search')).toHaveClass('hidden', 'sm:inline')
  await userEvent.setup().click(search)
  expect(
    await screen.findByPlaceholderText('Type a command or search...')
  ).toBeVisible()
})

test('compact navigation exposes all links and desktop link titles stay on one line', async () => {
  await renderHeader()
  const menu = screen.getByRole('button', { name: 'Open menu' })
  expect(menu.parentElement).toHaveClass('2xl:hidden')
  expect(screen.getByRole('navigation')).toHaveClass(
    'whitespace-nowrap',
    '2xl:flex'
  )
  await userEvent.setup().click(menu)
  const items = await screen.findByRole('menu')
  expect(
    within(items).getByRole('menuitem', { name: 'Model marketplace' })
  ).toHaveAttribute('href', '/pricing')
  expect(
    within(items).getByRole('menuitem', { name: 'Documentation' })
  ).toHaveAttribute('href', 'https://example.com/docs')
})
