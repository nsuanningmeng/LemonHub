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
import { act, renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { I18nextProvider } from 'react-i18next'
import { expect, test } from 'vitest'

import { createLocaleI18n } from '@/i18n/__tests__/helpers'

import { useCommonLogsColumns } from '../common-logs-columns'

test('the usage token header changes language and does not use the Vietnamese authorization-token label', async () => {
  const i18n = await createLocaleI18n('en')
  const { result } = renderHook(() => useCommonLogsColumns(false), {
    wrapper: ({ children }: { children: ReactNode }) => (
      <I18nextProvider i18n={i18n}>{children}</I18nextProvider>
    ),
  })
  expect(
    result.current.find(
      (column) =>
        'accessorKey' in column && column.accessorKey === 'prompt_tokens'
    )?.header
  ).toBe('Tokens')
  await act(async () => {
    await i18n.changeLanguage('vi')
  })
  expect(
    result.current.find(
      (column) =>
        'accessorKey' in column && column.accessorKey === 'prompt_tokens'
    )?.header
  ).toBe('Token sử dụng')
  expect(i18n.t('Usage tokens')).not.toBe(i18n.t('Tokens'))
})
