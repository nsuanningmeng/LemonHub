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
  username: 'appearance-review',
  display_name: 'Appearance review',
  role: 100,
  status: 1,
  group: 'default',
  quota: 500000,
  used_quota: 0,
  request_count: 0,
}

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem('i18nextLng', 'en')
    if (!localStorage.getItem('newapi:theme:v1:mode')) {
      localStorage.setItem('newapi:theme:v1:mode', 'light')
    }
  })
  await page.route('**/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    let data: unknown = []
    switch (path) {
      case '/api/status':
        data = {
          system_name: 'Appearance review',
          logo: '/lemonhub-mark.svg',
          register_enabled: true,
          password_login_enabled: true,
          password_register_enabled: true,
          quota_per_unit: 500000,
          quota_display_type: 'USD',
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
            user_agent: 'Appearance regression',
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
        data = { items: [], total: 0, page: 1, page_size: 10 }
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
  await page.goto('/keys')
  await expect(
    page.getByRole('region', { name: 'Access overview' })
  ).toBeVisible()
})

test('choosing a color preset updates colored statistics as well as controls', async ({
  page,
}) => {
  const statistic = page
    .getByRole('region', { name: 'Access overview' })
    .locator('dd')
    .first()
  const action = page
    .getByRole('button', { name: 'Create API Key', exact: true })
    .first()
  const before = await statistic.evaluate((el) => getComputedStyle(el).color)
  const actionBefore = await action.evaluate(
    (el) => getComputedStyle(el).backgroundColor
  )
  await page.getByRole('button', { name: 'Open theme settings' }).click()
  const presets = page.getByRole('radiogroup', { name: 'Select color preset' })
  await presets.getByRole('radio', { name: 'Rose Garden' }).click()
  await page.keyboard.press('Escape')
  await expect(page.locator('body')).toHaveAttribute(
    'data-theme-preset',
    'rose-garden'
  )
  await expect
    .poll(() => statistic.evaluate((el) => getComputedStyle(el).color))
    .not.toBe(before)
  await expect
    .poll(() => action.evaluate((el) => getComputedStyle(el).backgroundColor))
    .not.toBe(actionBefore)
  await page.reload()
  await expect(page.locator('body')).toHaveAttribute(
    'data-theme-preset',
    'rose-garden'
  )
  await expect
    .poll(() => statistic.evaluate((el) => getComputedStyle(el).color))
    .not.toBe(before)
  await page.getByRole('button', { name: 'Open theme settings' }).click()
  await page
    .getByRole('button', { name: 'Reset all settings to default values' })
    .click()
  await page.keyboard.press('Escape')
  await expect
    .poll(() => statistic.evaluate((el) => getComputedStyle(el).color))
    .toBe(before)
  await expect
    .poll(() => action.evaluate((el) => getComputedStyle(el).backgroundColor))
    .toBe(actionBefore)
})

test('serif preference reaches Astryx headings after navigation and reload', async ({
  page,
}) => {
  await page.getByRole('button', { name: 'Open theme settings' }).click()
  await page
    .getByRole('radiogroup', { name: 'Select body font' })
    .getByRole('radio', { name: 'Serif', exact: true })
    .click()
  await page.goto('/')
  const heading = page.locator('#home-title')
  await expect(heading).toBeVisible()
  await expect(heading).toHaveCSS('font-family', /Lora/)
  await page.reload()
  await expect(heading).toHaveCSS('font-family', /Lora/)
})

test('density and radius preferences update the open workspace layout and controls', async ({
  page,
}) => {
  const panel = page.getByRole('region', { name: 'Access overview' })
  const action = page
    .getByRole('button', { name: 'Create API Key', exact: true })
    .first()
  const before = await panel.evaluate((el) => ({
    gap: Number.parseFloat(getComputedStyle(el).rowGap),
  }))
  const beforeHeight = await action.evaluate(
    (el) => el.getBoundingClientRect().height
  )
  await page.getByRole('button', { name: 'Open theme settings' }).click()
  await page
    .getByRole('radiogroup', { name: 'Select interface density' })
    .getByRole('radio', { name: 'Super Large', exact: true })
    .click()
  await page
    .getByRole('radiogroup', { name: 'Select border radius' })
    .getByRole('radio', { name: '0', exact: true })
    .click()
  await page.keyboard.press('Escape')
  await expect(action).toHaveCSS('border-top-left-radius', '0px')
  await expect
    .poll(() =>
      panel.evaluate((el) => Number.parseFloat(getComputedStyle(el).rowGap))
    )
    .toBeGreaterThan(before.gap)
  await expect
    .poll(() => action.evaluate((el) => el.getBoundingClientRect().height))
    .toBeGreaterThan(beforeHeight)
})

test('centered content keeps the workspace toolbar full width on large screens', async ({
  page,
}) => {
  await page.getByRole('button', { name: 'Open theme settings' }).click()
  await page
    .getByRole('radiogroup', { name: 'Select content width' })
    .getByRole('radio', { name: 'Centered', exact: true })
    .click()
  await page.keyboard.press('Escape')
  await expect(page.locator('body')).toHaveAttribute(
    'data-theme-content-layout',
    'centered'
  )
  const inset = page.locator('[data-slot="sidebar-inset"]')
  const widths = await inset.evaluate((el) => ({
    available: el.clientWidth,
    header: el.querySelector('header')?.getBoundingClientRect().width ?? 0,
    content: el.lastElementChild?.getBoundingClientRect().width ?? Infinity,
  }))
  expect(widths.available).toBeGreaterThan(1280)
  expect(widths.header).toBeCloseTo(widths.available, 0)
  expect(widths.content).toBeLessThanOrEqual(1280)
})

test('status text colors remain readable on light and dark cards', async ({
  page,
}) => {
  for (const mode of ['light', 'dark']) {
    await page.evaluate(
      (mode) => localStorage.setItem('newapi:theme:v1:mode', mode),
      mode
    )
    await page.reload()
    const contrasts = await page.evaluate(() => {
      const canvas = document.createElement('canvas')
      canvas.width = canvas.height = 1
      const context = canvas.getContext('2d')
      if (!context) {
        throw new Error('Canvas is required to measure color contrast')
      }
      const luminance = (color: string) => {
        context.fillStyle = color
        context.fillRect(0, 0, 1, 1)
        const channels = [...context.getImageData(0, 0, 1, 1).data]
          .slice(0, 3)
          .map((channel) => {
            const value = channel / 255
            return value <= 0.04045
              ? value / 12.92
              : ((value + 0.055) / 1.055) ** 2.4
          })
        return (
          channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722
        )
      }
      return ['success', 'warning', 'info'].map((role) => {
        const sample = document.createElement('span')
        sample.style.color = `var(--${role})`
        sample.style.backgroundColor = 'var(--card)'
        document.body.append(sample)
        const style = getComputedStyle(sample)
        const text = luminance(style.color),
          background = luminance(style.backgroundColor)
        sample.remove()
        return {
          role,
          ratio:
            (Math.max(text, background) + 0.05) /
            (Math.min(text, background) + 0.05),
        }
      })
    })
    for (const result of contrasts) {
      expect
        .soft(result.ratio, `${mode} ${result.role} body text`)
        .toBeGreaterThanOrEqual(4.5)
    }
  }
})
