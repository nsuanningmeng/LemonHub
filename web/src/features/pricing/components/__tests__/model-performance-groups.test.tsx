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
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import type { PerformanceGroup } from '@/features/performance-metrics/types'
import { api } from '@/lib/api'

import {
  ModelDetailsContent,
  type ModelDetailsContentProps,
} from '../model-details'

// Canvas rendering is outside jsdom; exercise the real chart data preparation.
vi.mock('@visactor/react-vchart', () => ({ VChart: () => null }))
vi.mock('@visactor/vchart', () => ({
  ThemeManager: { setCurrentTheme: () => undefined },
}))

const sampledGroups: PerformanceGroup[] = [
  {
    group: 'Gemini',
    avg_ttft_ms: 100,
    avg_latency_ms: 200,
    avg_tps: 20,
    success_rate: 80,
    series: [
      {
        ts: 1720000000,
        avg_ttft_ms: 100,
        avg_latency_ms: 200,
        avg_tps: 20,
        success_rate: 80,
      },
    ],
  },
  {
    group: 'Gemini特价',
    avg_ttft_ms: 200,
    avg_latency_ms: 400,
    avg_tps: 10,
    success_rate: 40,
    series: [
      {
        ts: 1720000000,
        avg_ttft_ms: 200,
        avg_latency_ms: 400,
        avg_tps: 10,
        success_rate: 40,
      },
    ],
  },
  {
    group: 'default',
    avg_ttft_ms: 1000,
    avg_latency_ms: 2000,
    avg_tps: 1,
    success_rate: 0,
    series: [],
  },
  {
    group: 'private',
    avg_ttft_ms: 1000,
    avg_latency_ms: 2000,
    avg_tps: 1,
    success_rate: 0,
    series: [],
  },
]

describe('model performance groups', () => {
  let queryClient: QueryClient
  let props: ModelDetailsContentProps

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    })
    queryClient.setQueryData(['status'], { perf_no_data_as_full: true })
    queryClient.setQueryData(['perf-metrics', 'gemini-test'], {
      success: true,
      data: { model_name: 'gemini-test', groups: sampledGroups },
    })
    vi.spyOn(api, 'get').mockResolvedValue({
      data: queryClient.getQueryData(['perf-metrics', 'gemini-test']),
    })
    props = {
      model: {
        id: 1,
        model_name: 'gemini-test',
        quota_type: 1,
        model_ratio: 1,
        completion_ratio: 1,
        model_price: 0.1,
        enable_groups: [
          'Gemini',
          'Gemini混合',
          'Gemini特价',
          'private',
          'auto',
        ],
      },
      usableGroup: {
        Gemini: { desc: '', ratio: 1 },
        Gemini混合: { desc: '', ratio: 3 },
        Gemini特价: { desc: '', ratio: 0.6 },
        default: { desc: '', ratio: 1 },
        auto: { desc: '', ratio: 1 },
      },
      groupRatio: { Gemini: 1, Gemini混合: 3, Gemini特价: 0.6, default: 1 },
      endpointMap: {},
      autoGroups: [],
      priceRate: 1,
      usdExchangeRate: 1,
      tokenUnit: 'M',
    }
  })

  afterEach(() => {
    queryClient.clear()
  })

  test('current pricing groups include unsampled groups and exclude historical or inaccessible groups', async () => {
    const user = userEvent.setup()
    render(
      <QueryClientProvider client={queryClient}>
        <ModelDetailsContent {...props} />
      </QueryClientProvider>
    )
    await user.click(screen.getByRole('tab', { name: 'Performance' }))

    const table = screen.getByRole('table')
    expect(
      within(table)
        .getAllByRole('row')
        .slice(1)
        .map((row) => within(row).getAllByRole('cell')[0].textContent)
    ).toEqual(['Gemini', 'Gemini混合', 'Gemini特价'])
    const emptyGroup = within(table).getByRole('row', { name: /Gemini混合/ })
    expect(within(emptyGroup).getAllByText('—')).toHaveLength(4)
    expect(screen.getByText('60.00%')).toBeInTheDocument()
    expect(screen.queryByRole('note')).not.toBeInTheDocument()
  })

  test.each([
    { name: 'TPS', avg_tps: 0, avg_ttft_ms: 100, missingCells: 1 },
    { name: 'TTFT', avg_tps: 20, avg_ttft_ms: 0, missingCells: 1 },
    { name: 'TPS and TTFT', avg_tps: 0, avg_ttft_ms: 0, missingCells: 2 },
  ])(
    'groups with success-rate samples but no $name explain the missing measurements',
    async ({ avg_tps, avg_ttft_ms, missingCells }) => {
      const user = userEvent.setup()
      const group = {
        ...sampledGroups[0],
        avg_tps,
        avg_ttft_ms,
        series: [{ ...sampledGroups[0].series[0], avg_tps, avg_ttft_ms }],
      }
      queryClient.setQueryData(['perf-metrics', 'gemini-test'], {
        success: true,
        data: { model_name: 'gemini-test', groups: [group] },
      })
      vi.mocked(api.get).mockResolvedValue({
        data: queryClient.getQueryData(['perf-metrics', 'gemini-test']),
      })
      render(
        <QueryClientProvider client={queryClient}>
          <ModelDetailsContent {...props} />
        </QueryClientProvider>
      )
      await user.click(screen.getByRole('tab', { name: 'Performance' }))

      expect(screen.getByRole('note')).toHaveTextContent(
        'TPS and TTFT require samples from this model. TTFT is available only for streaming responses.'
      )
      const row = within(screen.getByRole('table')).getByRole('row', {
        name: /^Gemini\s/,
      })
      expect(within(row).getAllByText('—')).toHaveLength(missingCells)
    }
  )

  test('removing a model group immediately removes its row and contribution despite cached metrics', async () => {
    const user = userEvent.setup()
    const view = render(
      <QueryClientProvider client={queryClient}>
        <ModelDetailsContent {...props} />
      </QueryClientProvider>
    )
    await user.click(screen.getByRole('tab', { name: 'Performance' }))
    props = {
      ...props,
      model: { ...props.model, enable_groups: ['Gemini', 'Gemini混合'] },
    }
    view.rerender(
      <QueryClientProvider client={queryClient}>
        <ModelDetailsContent {...props} />
      </QueryClientProvider>
    )

    expect(
      within(screen.getByRole('table')).queryByText('Gemini特价')
    ).not.toBeInTheDocument()
    expect(screen.getByText('80.00%')).toBeInTheDocument()
    expect(screen.getAllByText('200ms')).toHaveLength(2)
  })

  test('overview summary uses the same currently available model groups', () => {
    render(
      <QueryClientProvider client={queryClient}>
        <ModelDetailsContent {...props} />
      </QueryClientProvider>
    )
    expect(screen.getByText('60.00%')).toBeInTheDocument()
    expect(screen.getByText('300ms')).toBeInTheDocument()
  })

  test('deleting a configured group ratio removes the group while preserving a zero-ratio group', async () => {
    const user = userEvent.setup()
    props = { ...props, groupRatio: { Gemini: 0, Gemini混合: 3 } }
    render(
      <QueryClientProvider client={queryClient}>
        <ModelDetailsContent {...props} />
      </QueryClientProvider>
    )
    await user.click(screen.getByRole('tab', { name: 'Performance' }))

    const table = screen.getByRole('table')
    expect(
      within(table)
        .getAllByRole('row')
        .slice(1)
        .map((row) => within(row).getAllByRole('cell')[0].textContent)
    ).toEqual(['Gemini', 'Gemini混合'])
    expect(screen.getByText('80.00%')).toBeInTheDocument()
  })

  test.each([true, false])(
    'models with no recent samples show every current group with empty metrics when noDataAsFull is %s',
    async (noDataAsFull) => {
      const user = userEvent.setup()
      queryClient.setQueryData(['status'], {
        perf_no_data_as_full: noDataAsFull,
      })
      queryClient.setQueryData(['perf-metrics', 'gemini-test'], {
        success: true,
        data: { model_name: 'gemini-test', groups: [] },
      })
      vi.mocked(api.get).mockResolvedValue({
        data: queryClient.getQueryData(['perf-metrics', 'gemini-test']),
      })
      render(
        <QueryClientProvider client={queryClient}>
          <ModelDetailsContent {...props} />
        </QueryClientProvider>
      )
      await user.click(screen.getByRole('tab', { name: 'Performance' }))

      const table = screen.getByRole('table')
      expect(within(table).getAllByRole('row')).toHaveLength(4)
      expect(within(table).getAllByText('—')).toHaveLength(12)
      expect(
        screen.getByText('No requests in the last 24 hours')
      ).toBeInTheDocument()
      expect(screen.queryByText('100.00%') !== null).toBe(noDataAsFull)
      expect(screen.queryByRole('note')).not.toBeInTheDocument()
    }
  )
  test('time-dependent group prices retain tier labels without exposing raw predicates', () => {
    const condition = 'hour("Asia/Shanghai") >= 9 && hour("Asia/Shanghai") < 18'
    props.model.billing_mode = 'tiered_expr'
    props.model.billing_expr = `${condition} ? tier("peak", p * 3 + c * 15) : tier("off", p * 1 + c * 5)`
    render(
      <QueryClientProvider client={queryClient}>
        <ModelDetailsContent {...props} />
      </QueryClientProvider>
    )
    const groups = screen.getByText('Pricing by Group').closest('section')
    expect(groups).not.toBeNull()
    if (!groups) throw new Error('Missing group pricing section')
    expect(within(groups).queryAllByText(/hour\(/)).toHaveLength(0)
    expect(within(groups).getAllByText('peak')).toHaveLength(3)
    expect(within(groups).getAllByText('off')).toHaveLength(3)
  })

  test('long-context pricing hides base formulas and describes the complementary group tier', () => {
    props.model.billing_mode = 'tiered_expr'
    props.model.billing_expr =
      '(len <= 272000 ? tier("standard", p * 5 + c * 30) : tier("long_context", p * 10 + c * 45)) * (param("service_tier") == "fast" ? 2 : 1)'
    render(
      <QueryClientProvider client={queryClient}>
        <ModelDetailsContent {...props} />
      </QueryClientProvider>
    )

    const base = screen
      .getByRole('heading', { name: 'Base Price' })
      .closest('section')
    if (!base) throw new Error('Missing base pricing section')
    expect(
      within(base).queryByText(/len <=|service_tier/)
    ).not.toBeInTheDocument()
    expect(within(base).getByText('$5')).toBeInTheDocument()
    expect(within(base).getByText('$30')).toBeInTheDocument()

    const groups = screen
      .getByRole('heading', { name: 'Pricing by Group' })
      .closest('section')
    if (!groups) throw new Error('Missing group pricing section')
    expect(within(groups).queryAllByText(/len <=|!\(/)).toHaveLength(0)
    const table = within(groups).getAllByRole('table')[0]
    expect(
      within(table).getByRole('row', { name: /standard/ })
    ).toHaveTextContent('$5')
    const longContext = within(table).getByRole('row', { name: /long_context/ })
    expect(longContext).toHaveTextContent(/Full input length > 272(?:K|000)/)
    expect(longContext).toHaveTextContent('$10')
    expect(longContext).toHaveTextContent('$45')
  })

  test('unsupported pricing retains base and group explanations without raw expressions', () => {
    props.model.billing_mode = 'tiered_expr'
    props.model.billing_expr = 'max(p * 3, 5)'
    render(
      <QueryClientProvider client={queryClient}>
        <ModelDetailsContent {...props} />
      </QueryClientProvider>
    )

    for (const title of ['Base Price', 'Pricing by Group']) {
      const section = screen
        .getByRole('heading', { name: title })
        .closest('section')
      if (!section) throw new Error(`Missing ${title} section`)
      expect(
        within(section).getByText('Special billing expression')
      ).toBeInTheDocument()
      expect(
        within(section).queryByText('Raw expression')
      ).not.toBeInTheDocument()
      expect(
        within(section).queryByText(props.model.billing_expr)
      ).not.toBeInTheDocument()
    }
  })
})
