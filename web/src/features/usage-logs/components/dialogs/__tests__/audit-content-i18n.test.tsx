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
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'

import { usageLogSchema } from '../../../data/schema'
import { DetailsDialog } from '../details-dialog'

test('labels and copies the original audit content while the operation summary follows the selected language', async () => {
  const user = userEvent.setup()
  const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
  const language = createInstance()
  await language.use(initReactI18next).init({
    lng: 'zh',
    fallbackLng: 'en',
    resources: { en, zh },
  })
  const raw = 'Updated user customer_timeout (ID: 42)'
  const log = usageLogSchema.parse({
    id: 1,
    user_id: 42,
    created_at: 1,
    type: 3,
    content: raw,
    other: JSON.stringify({
      op: {
        action: 'user.update',
        params: { username: 'customer_timeout', id: '42' },
      },
    }),
  })
  render(
    <I18nextProvider i18n={language}>
      <DetailsDialog log={log} isAdmin open onOpenChange={() => undefined} />
    </I18nextProvider>
  )
  expect(screen.getByText('原始内容')).toBeVisible()
  expect(screen.getByText(raw)).toBeVisible()
  await user.click(screen.getByRole('button', { name: '复制到剪贴板' }))
  expect(copy).toHaveBeenCalledWith(raw)
  await act(async () => {
    await language.changeLanguage('en')
  })
  expect(screen.getByText('Original content')).toBeVisible()
  expect(screen.getAllByText(raw).length).toBeGreaterThan(0)
})
