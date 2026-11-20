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
  username: 'mobile-dialog-review',
  display_name: 'Mobile dialog review',
  role: 100,
  status: 1,
  group: 'default',
  quota: 500000,
  used_quota: 0,
  request_count: 0,
}
const token = {
  id: 1,
  name: 'A mobile API key with a descriptive application name '.repeat(5),
  key: 'fixture-masked-key',
  status: 1,
  remain_quota: 500000,
  used_quota: 0,
  unlimited_quota: false,
  expired_time: -1,
  created_time: 1,
  accessed_time: 1,
  group: 'default',
  model_limits_enabled: false,
}

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('i18nextLng', 'en'))
  await page.route('**/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    let data: unknown = []
    switch (path) {
      case '/api/status':
        data = {
          system_name: 'LemonHub',
          logo: '/lemonhub-mark.svg',
          quota_per_unit: 500000,
          quota_display_type: 'USD',
        }
        break
      case '/api/setup':
        data = { status: true, root_init: true, database_type: 'sqlite' }
        break
      case '/api/user/auth/refresh':
        data = {
          access_token: 'mobile-dialog-fixture',
          token_type: 'Bearer',
          access_expires_at: Math.floor(Date.now() / 1000) + 3600,
          user,
          session: {
            sid: 'mobile-dialog-fixture',
            current: true,
            login_method: 'password',
            ip: '127.0.0.1',
            user_agent: 'Mobile dialog regression',
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
        data = { items: [token], total: 1, page: 1, page_size: 10 }
        break
      case '/api/token/1/key':
        data = { key: 'mobile-dialog-fixture' }
        break
      case '/api/token/auto-groups':
        data = { groups: ['default'], max_count: 10 }
        break
      case '/api/user/self/groups':
        data = { default: { ratio: 1, desc: 'Default' } }
        break
      case '/api/rewards/summary':
        data = { pending_count: 0 }
        break
      case '/api/ticket/config':
        data = { enabled: true, types: [] }
        break
      case '/api/ticket/admin/':
        data = { items: [], total: 0, page: 1, page_size: 20 }
        break
    }
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ success: true, data }),
    })
  })
})

test('a tall settings dialog fits a short phone viewport and scrolls to its footer', async ({
  page,
}) => {
  await page.setViewportSize({ width: 375, height: 360 })
  await page.goto('/console/tickets-admin')
  await page.getByRole('button', { name: 'Cleanup attachments' }).click()
  const dialog = page.getByRole('dialog', { name: 'Cleanup attachments' })
  await expect(dialog).toBeVisible()
  const bounds = await dialog.boundingBox()
  if (!bounds) throw new Error('Dialog is not rendered')
  expect(bounds.y).toBeGreaterThan(0)
  expect(bounds.y + bounds.height).toBeLessThan(360)
  await dialog.evaluate((element) => {
    element.scrollTop = element.scrollHeight
  })
  await expect(
    dialog.getByRole('button', { name: 'Run cleanup', exact: true })
  ).toBeInViewport()
  await dialog
    .getByRole('button', { name: 'Close', exact: true })
    .first()
    .click()
  await expect(dialog).not.toBeVisible()
})

test('a long delete confirmation stays readable and cancellable on a narrow phone', async ({
  page,
}) => {
  await page.goto('/keys')
  await page
    .getByRole('button', { name: 'Open menu', exact: true })
    .last()
    .click()
  await page.getByRole('menuitem', { name: 'Delete', exact: true }).click()
  await page.setViewportSize({ width: 320, height: 400 })
  const dialog = page.getByRole('alertdialog', { name: 'Are you sure?' })
  await expect(dialog).toBeVisible()
  const bounds = await dialog.boundingBox()
  if (!bounds) throw new Error('Dialog is not rendered')
  expect(bounds.x).toBeGreaterThan(0)
  expect(bounds.x + bounds.width).toBeLessThan(320)
  expect(bounds.y).toBeGreaterThan(0)
  expect(bounds.y + bounds.height).toBeLessThan(400)
  await dialog.evaluate((element) => {
    element.scrollTop = element.scrollHeight
  })
  await expect(dialog.getByRole('button', { name: 'Cancel' })).toBeInViewport()
  await dialog.getByRole('button', { name: 'Cancel' }).click()
  await expect(dialog).not.toBeVisible()
})

test('workspace search fits a short landscape viewport and returns focus after reaching the last option', async ({
  page,
}) => {
  await page.setViewportSize({ width: 568, height: 320 })
  await page.goto('/keys')
  const trigger = page.getByRole('button', { name: 'Search', exact: true })
  await trigger.click()
  const dialog = page.getByRole('dialog', { name: 'Command palette' })
  await expect(dialog).toBeVisible()
  const bounds = await dialog.boundingBox()
  if (!bounds) throw new Error('Search dialog is not rendered')
  expect(bounds.y).toBeGreaterThanOrEqual(0)
  expect(bounds.y + bounds.height).toBeLessThanOrEqual(320)
  const last = dialog.getByRole('option').last()
  await last.scrollIntoViewIfNeeded()
  await expect(last).toBeInViewport()
  await page.keyboard.press('Escape')
  await expect(dialog).not.toBeVisible()
  await expect(trigger).toBeFocused()
})
