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
import { render, screen } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { expect, test, vi } from 'vitest'

import { createLocaleI18n } from '@/i18n/__tests__/helpers'
import { api } from '@/lib/api'

import {
  ModelDetailsContent,
  type ModelDetailsContentProps,
} from '../model-details'

vi.mock('@visactor/react-vchart', () => ({ VChart: () => null }))
vi.mock('@visactor/vchart', () => ({
  ThemeManager: { setCurrentTheme: () => undefined },
}))

test.each(['static', 'dynamic'] as const)(
  '%s model pricing note translates the complete token unit sentence',
  async (mode) => {
    const i18n = await createLocaleI18n('zhCN')
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: {} },
    })
    const props: ModelDetailsContentProps = {
      model: {
        id: 1,
        model_name: 'local-model',
        quota_type: 0,
        model_ratio: 1,
        completion_ratio: 1,
        model_price: 0,
        enable_groups: ['default'],
        ...(mode === 'dynamic'
          ? { billing_mode: 'tiered_expr', billing_expr: 'p * 2 + c * 3' }
          : {}),
      },
      groupRatio: { default: 1 },
      usableGroup: { default: { desc: '', ratio: 1 } },
      endpointMap: {},
      autoGroups: [],
      priceRate: 1,
      usdExchangeRate: 1,
      tokenUnit: 'K',
    }
    try {
      render(
        <I18nextProvider i18n={i18n}>
          <QueryClientProvider client={client}>
            <ModelDetailsContent {...props} />
          </QueryClientProvider>
        </I18nextProvider>
      )
      expect(
        screen.getByText('价格按每 1K Token 显示', { exact: true })
      ).toBeInTheDocument()
    } finally {
      client.clear()
    }
  }
)
