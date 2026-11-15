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
import { expect, test } from '@playwright/test'

const user = {
  id: 1,
  username: 'mobile-review',
  display_name: 'Mobile review',
  role: 100,
  status: 1,
  group: 'default',
  quota: 500000,
  used_quota: 0,
  request_count: 0,
}

const keys = Array.from({ length: 24 }, (_, index) => ({
  id: index + 1,
  name:
    index === 0
      ? 'Production application with a very long credential name for narrow screens'
      : `Application key ${index + 1}`,
  key: `mobile-fixture-${index + 1}-masked`,
  status: index === 1 ? 2 : 1,
  remain_quota: 500000,
  used_quota: 50000,
  unlimited_quota: false,
  expired_time: -1,
  created_time: 1,
  accessed_time: 1,
  group: 'default',
  auto_groups: [],
  cross_group_retry: false,
  model_limits_enabled: false,
  model_limits: '',
  allow_ips: '',
}))

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem('i18nextLng', 'en')
    localStorage.setItem('newapi:theme:v1:mode', 'light')
  })
  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url())
    let data: unknown = []
    switch (url.pathname) {
      case '/api/status':
        data = {
          system_name: 'Mobile review',
          logo: '/lemonhub-mark.svg',
          register_enabled: true,
          password_login_enabled: true,
          password_register_enabled: true,
          quota_per_unit: 500000,
          quota_display_type: 'USD',
          announcements_enabled: true,
          announcements: [
            {
              id: 'mobile-announcement',
              content:
                'Service announcement with enough text to exercise the narrow banner layout.',
              publishDate: '2026-11-15',
            },
          ],
        }
        break
      case '/api/setup':
        data = { status: true, root_init: true, database_type: 'sqlite' }
        break
      case '/api/user/auth/refresh':
        data = {
          access_token: 'browser-fixture',
          token_type: 'Bearer',
          access_expires_at: Math.floor(Date.now() / 1000) + 3600,
          user,
          session: {
            sid: 'browser-fixture',
            current: true,
            login_method: 'password',
            ip: '127.0.0.1',
            user_agent: 'Mobile regression',
            created_at: 1,
            last_active_at: 1,
            expires_at: Math.floor(Date.now() / 1000) + 3600,
          },
        }
        break
      case '/api/user/self':
        data = user
        break
      case '/api/notice':
      case '/api/home_page_content':
        data = ''
        break
      case '/api/token/':
      case '/api/token/search': {
        const pageIndex = Number(url.searchParams.get('p') ?? 1) - 1
        const size = Number(url.searchParams.get('size') ?? 20)
        const keyword = url.searchParams.get('keyword') ?? ''
        const filtered = keys.filter((key) => key.name.includes(keyword))
        data = {
          items: filtered.slice(pageIndex * size, (pageIndex + 1) * size),
          total: filtered.length,
          page: pageIndex + 1,
          page_size: size,
        }
        break
      }
      case '/api/channel':
        data = {
          items: keys.slice(0, 20).map((key) => ({
            id: key.id,
            name: `Upstream channel ${key.id}`,
            type: 1,
            key: '',
            status: 1,
            created_time: 1,
            test_time: 1,
            response_time: 100,
            balance_updated_time: 1,
            models: 'gpt-4o',
            group: 'default',
            priority: 1,
            weight: 1,
          })),
          total: 20,
        }
        break
      case '/api/channel/priority/order':
        data = { version: 'browser-fixture', items: [] }
        break
      case '/api/user/':
        data = {
          items: keys.slice(0, 20).map((key) => ({
            ...user,
            id: key.id,
            username: `member-${key.id}`,
            display_name: `Member ${key.id}`,
            role: 1,
          })),
          total: 20,
        }
        break
      case '/api/group/':
        data = ['default']
        break
      case '/api/models/':
        data = {
          items: keys.slice(0, 20).map((key) => ({
            id: key.id,
            model_name: `text-model-${key.id}`,
            status: 1,
            sync_official: 0,
            created_time: 1,
            updated_time: 1,
            name_rule: 0,
          })),
          total: 20,
        }
        break
      case '/api/vendors/':
        data = { items: [], total: 0 }
        break
      case '/api/log':
        data = {
          items: keys.slice(0, 20).map((key) => ({
            id: key.id,
            user_id: 1,
            username: 'mobile-review',
            created_at: 1700000000,
            type: 2,
            content: '',
            token_name: `Usage credential ${key.id}`,
            model_name: `request-model-${key.id}`,
            quota: 100,
            prompt_tokens: 10,
            completion_tokens: 5,
            use_time: 1,
            is_stream: false,
            channel: 1,
            channel_name: 'Upstream channel',
            token_id: key.id,
            group: 'default',
            ip: '',
            other: '{}',
            request_id: `fixture-request-${key.id}`,
            upstream_request_id: '',
          })),
          total: 20,
        }
        break
      case '/api/log/stat':
        data = { quota: 2000, rpm: 1, tpm: 15 }
        break
      case '/api/token/1/key':
        data = { key: 'mobile-browser-fixture-only-not-a-real-credential' }
        break
      case '/api/token/auto-groups':
        data = { groups: ['default'], max_count: 10 }
        break
      case '/api/user/self/groups':
        data = { default: { ratio: 1, desc: 'Default' } }
        break
      case '/api/rewards/summary':
        data = {
          pending_count: 0,
          fixed_count: 0,
          random_count: 0,
          fixed_quota: 0,
        }
        break
    }
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ success: true, data }),
    })
  })
})

for (const viewport of [
  { width: 320, height: 568 },
  { width: 390, height: 664 },
  { width: 430, height: 740 },
  { width: 844, height: 390 },
  { width: 768, height: 600 },
]) {
  test(`keys at ${viewport.width}x${viewport.height} scroll with their overview and filters instead of a collapsed list`, async ({
    page,
  }) => {
    await page.setViewportSize(viewport)
    await page.goto('/keys')
    const firstKey = page.getByText(keys[0].name, { exact: true })
    await expect(firstKey).toBeAttached()
    await expect(
      page.getByRole('region', { name: 'Latest announcements' })
    ).toBeVisible()

    // A phone must scroll the whole page content, not a tiny list squeezed
    // between the non-scrolling overview, filters and footer.
    const scrollArea = await firstKey.evaluate((element) => {
      let ancestor = element.parentElement
      while (ancestor) {
        const style = getComputedStyle(ancestor)
        if (
          /(auto|scroll)/.test(style.overflowY) &&
          ancestor.scrollHeight > ancestor.clientHeight
        ) {
          return {
            includesOverview: Boolean(
              ancestor.querySelector('[aria-label="Access overview"]')
            ),
            height: ancestor.clientHeight,
          }
        }
        ancestor = ancestor.parentElement
      }
      return null
    })
    expect(scrollArea?.includesOverview).toBe(true)
    expect(scrollArea?.height).toBeGreaterThan(viewport.height / 2)

    await firstKey.scrollIntoViewIfNeeded()
    await expect(firstKey).toBeInViewport()
    const lastKey = page.getByText('Application key 20', { exact: true })
    await lastKey.scrollIntoViewIfNeeded()
    await expect(lastKey).toBeInViewport()
    const nextPage = page.getByRole('button', {
      name: 'Go to next page',
      exact: true,
    })
    await nextPage.click()
    await expect(
      page.getByText('Application key 24', { exact: true })
    ).toBeAttached()
    await page
      .getByText('Application key 24', { exact: true })
      .scrollIntoViewIfNeeded()
    await expect(
      page.getByText('Application key 24', { exact: true })
    ).toBeInViewport()
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth)
    ).toBeLessThanOrEqual(viewport.width)
  })
}

test('a narrow phone can filter keys and open the full credential without horizontal overflow', async ({
  page,
}) => {
  await page.setViewportSize({ width: 320, height: 568 })
  await page.goto('/keys')
  const search = page.getByPlaceholder('Filter by name...')
  await search.fill('Production')
  await expect(
    page.getByText('Application key 20', { exact: true })
  ).not.toBeAttached()
  await page
    .getByRole('button', { name: 'sk-mobile-fixture-1-masked', exact: true })
    .click()
  await expect(page.locator('input[readonly]')).toHaveValue(
    'sk-mobile-browser-fixture-only-not-a-real-credential'
  )
  const inputBounds = await page.locator('input[readonly]').boundingBox()
  if (!inputBounds) throw new Error('Full credential input is not rendered')
  expect(inputBounds.x).toBeGreaterThanOrEqual(0)
  expect(inputBounds.x + inputBounds.width).toBeLessThanOrEqual(320)
  const inputFitsPopover = await page
    .locator('input[readonly]')
    .evaluate((input) => {
      const popup = input.closest('[data-slot="popover-content"]')
      if (!popup) throw new Error('Credential popover is not rendered')
      const style = getComputedStyle(popup)
      const availableWidth =
        popup.clientWidth -
        Number.parseFloat(style.paddingLeft) -
        Number.parseFloat(style.paddingRight)
      // Layout widths stay stable while the popup's opening animation scales
      // its bounding rectangle; a minimum-width overflow still fails here.
      return (input as HTMLInputElement).offsetWidth <= availableWidth
    })
  expect(inputFitsPopover).toBe(true)
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth)
  ).toBeLessThanOrEqual(320)
})

for (const viewport of [
  { width: 768, height: 640 },
  { width: 1440, height: 900 },
]) {
  test(`keys at ${viewport.width}x${viewport.height} retain usable table scrolling and pagination`, async ({
    page,
  }) => {
    await page.setViewportSize(viewport)
    await page.goto('/keys')
    await expect(page.getByText(keys[0].name, { exact: true })).toBeAttached()
    const lastKey = page.getByText('Application key 20', { exact: true })
    await lastKey.scrollIntoViewIfNeeded()
    await expect(lastKey).toBeInViewport()
    await expect(
      page.getByRole('button', { name: 'Go to next page', exact: true })
    ).toBeInViewport()
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth)
    ).toBeLessThanOrEqual(viewport.width)
  })
}

for (const pageCase of [
  { route: '/channels', lastRecord: 'Upstream channel 20' },
  { route: '/users', lastRecord: 'member-20' },
  { route: '/usage-logs', lastRecord: 'request-model-20' },
  { route: '/models', lastRecord: 'text-model-20' },
]) {
  test(`${pageCase.route} keeps its last record and pagination reachable on a phone`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 664 })
    await page.goto(pageCase.route)
    const lastRecord = page
      .getByText(pageCase.lastRecord, { exact: true })
      .first()
    await expect(lastRecord).toBeAttached()
    await lastRecord.scrollIntoViewIfNeeded()
    await expect(lastRecord).toBeInViewport()
    const pagination = page.getByRole('navigation', {
      name: 'Table pagination',
    })
    await pagination.scrollIntoViewIfNeeded()
    await expect(pagination).toBeInViewport()
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth)
    ).toBeLessThanOrEqual(390)
  })
}

test('keys loading on a short phone keep the search controls reachable', async ({
  page,
}) => {
  await page.setViewportSize({ width: 320, height: 568 })
  let finishLoading = () => {}
  const responseReady = new Promise<void>((resolve) => {
    finishLoading = resolve
  })
  await page.route('**/api/token/?*', async (route) => {
    await responseReady
    await route.fallback()
  })
  try {
    await page.goto('/keys')
    await expect(
      page.getByRole('region', { name: 'Access overview' })
    ).toHaveAttribute('aria-busy', 'true')
    const search = page.getByPlaceholder('Filter by name...')
    await search.scrollIntoViewIfNeeded()
    await expect(search).toBeInViewport()
    finishLoading()
    await expect(page.getByText(keys[0].name, { exact: true })).toBeAttached()
  } finally {
    finishLoading()
  }
})

test('a failed keys request on a phone shows a reachable retry that restores the list', async ({
  page,
}) => {
  await page.setViewportSize({ width: 320, height: 568 })
  let failing = true
  await page.route('**/api/token/?*', async (route) => {
    if (failing) {
      await route.fulfill({
        json: { success: false, message: 'Fixture service unavailable' },
      })
    } else {
      await route.fallback()
    }
  })
  await page.goto('/keys')
  const error = page
    .getByRole('alert')
    .filter({ hasText: 'Failed to load API keys' })
  // Production queries retry before exposing the recoverable error state.
  await expect(error).toBeVisible({ timeout: 30_000 })
  const retry = error.getByRole('button', { name: 'Retry', exact: true })
  await retry.scrollIntoViewIfNeeded()
  await expect(retry).toBeInViewport()
  failing = false
  await retry.click()
  const firstKey = page.getByText(keys[0].name, { exact: true })
  await firstKey.scrollIntoViewIfNeeded()
  await expect(firstKey).toBeInViewport()
})

test('an empty filtered keys list on a phone keeps its message and reset search reachable', async ({
  page,
}) => {
  await page.setViewportSize({ width: 320, height: 568 })
  await page.goto('/keys')
  const search = page.getByPlaceholder('Filter by name...')
  await search.fill('No matching credential')
  const empty = page.getByText('No API Keys Found', { exact: true })
  await empty.scrollIntoViewIfNeeded()
  await expect(empty).toBeInViewport()
  await search.fill('')
  const firstKey = page.getByText(keys[0].name, { exact: true })
  await firstKey.scrollIntoViewIfNeeded()
  await expect(firstKey).toBeInViewport()
})

test('large density on a small desktop keeps the keys list and paging controls usable', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1024, height: 768 })
  await page.goto('/keys')
  await page.getByRole('button', { name: 'Open theme settings' }).click()
  await page
    .getByRole('radiogroup', { name: 'Select interface density' })
    .getByRole('radio', { name: 'Super Large', exact: true })
    .click()
  await page.keyboard.press('Escape')
  const lastKey = page.getByText('Application key 20', { exact: true })
  await lastKey.scrollIntoViewIfNeeded()
  await expect(lastKey).toBeInViewport()
  const next = page.getByRole('button', {
    name: 'Go to next page',
    exact: true,
  })
  await next.click()
  const nextKey = page.getByText('Application key 24', { exact: true })
  await nextKey.scrollIntoViewIfNeeded()
  await expect(nextKey).toBeInViewport()
})

test('the mobile navigation can reach administration and return to the selected page', async ({
  page,
}) => {
  await page.setViewportSize({ width: 320, height: 568 })
  await page.goto('/keys')
  await page
    .getByRole('button', { name: 'Toggle Sidebar', exact: true })
    .click()
  const sidebar = page.getByRole('dialog', { name: 'Sidebar', exact: true })
  const usersLink = sidebar.getByRole('link', { name: 'Users', exact: true })
  await usersLink.scrollIntoViewIfNeeded()
  await expect(usersLink).toBeInViewport()
  await usersLink.click()
  await expect(sidebar).not.toBeVisible()
  await expect(page).toHaveURL(/\/users/)
  await expect(
    page.getByRole('heading', { name: 'Users', exact: true })
  ).toBeVisible()
})

test('a phone shows the first credential immediately and keeps its status readable beside a long name', async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 664 })
  await page.goto('/keys')
  const firstKey = page.getByText(keys[0].name, { exact: true })
  await expect.soft(firstKey).toBeInViewport()
  const status = page.getByText('Enabled', { exact: true }).first()
  const statusWidth = await status.evaluate((element) => ({
    available: element.clientWidth,
    needed: element.scrollWidth,
  }))
  expect(statusWidth.available).toBeGreaterThanOrEqual(statusWidth.needed)
})
