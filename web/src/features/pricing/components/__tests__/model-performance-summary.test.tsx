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
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import {
  ModelDetailsContent,
  type ModelDetailsContentProps,
} from '../model-details'
import { ModelDetailsPerformance } from '../model-details-performance'

vi.mock('@visactor/react-vchart', () => ({ VChart: () => null }))
vi.mock('@visactor/vchart', () => ({
  ThemeManager: { setCurrentTheme: () => undefined },
}))

const props: ModelDetailsContentProps = {
  model: {
    id: 1,
    model_name: 'weighted-model',
    quota_type: 1,
    model_ratio: 1,
    completion_ratio: 1,
    model_price: 0.1,
    enable_groups: ['default', 'premium'],
  },
  usableGroup: {
    default: { desc: '', ratio: 1 },
    premium: { desc: '', ratio: 1 },
  },
  groupRatio: { default: 1, premium: 1 },
  endpointMap: {},
  autoGroups: [],
  priceRate: 1,
  usdExchangeRate: 1,
  tokenUnit: 'M',
}
const summary = {
  success: true,
  data: {
    models: [
      {
        model_name: 'weighted-model',
        success_rate: 99.01,
        avg_latency_ms: 210,
        avg_tps: 19.8,
      },
    ],
  },
}
const metrics = {
  success: true,
  data: {
    model_name: 'weighted-model',
    groups: [
      {
        group: 'default',
        success_rate: 100,
        avg_latency_ms: 200,
        avg_ttft_ms: 100,
        avg_tps: 20,
        series: [],
      },
      {
        group: 'premium',
        success_rate: 0,
        avg_latency_ms: 1200,
        avg_ttft_ms: 200,
        avg_tps: 2,
        series: [],
      },
    ],
  },
}

describe('model detail performance summary', () => {
  let client: QueryClient
  beforeEach(() => {
    client = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    })
    client.setQueryData(['status'], { perf_no_data_as_full: true })
    vi.spyOn(api, 'get').mockImplementation(async (url) => ({
      data: url === '/api/perf-metrics/summary' ? summary : metrics,
    }))
  })
  afterEach(() => {
    client.clear()
  })

  test.each(['Overview', 'Performance'])(
    '%s displays the request-weighted model summary for groups with unequal traffic',
    async (tab) => {
      const user = userEvent.setup()
      render(
        <QueryClientProvider client={client}>
          <ModelDetailsContent {...props} />
        </QueryClientProvider>
      )
      if (tab === 'Performance') {
        await user.click(screen.getByRole('tab', { name: tab }))
      }
      // 100 successful requests in default, one failed request in premium.
      // The model summary is 99.01%, while averaging group rates gives 50%.
      expect(await screen.findByText('99.01%')).toBeInTheDocument()
      expect(screen.queryByText('50.00%')).not.toBeInTheDocument()
      expect(screen.getByText('210ms')).toBeInTheDocument()
      expect(screen.getByText('19.8 t/s')).toBeInTheDocument()
      expect(api.get).toHaveBeenCalledWith('/api/perf-metrics/summary', {
        params: { hours: 24 },
      })
    }
  )

  test.each([
    { view: 'overview', failedEndpoint: '/api/perf-metrics/summary' },
    { view: 'performance', failedEndpoint: '/api/perf-metrics/summary' },
    { view: 'performance', failedEndpoint: '/api/perf-metrics' },
  ])(
    '$view reports a $failedEndpoint HTTP 500 without claiming healthy or idle traffic',
    async ({ view, failedEndpoint }) => {
      vi.mocked(api.get).mockImplementation(async (url) => {
        if (url === failedEndpoint) throw new Error('HTTP 500')
        return { data: url === '/api/perf-metrics/summary' ? summary : metrics }
      })
      render(
        <QueryClientProvider client={client}>
          {view === 'overview' ? (
            <ModelDetailsContent {...props} />
          ) : (
            <ModelDetailsPerformance
              model={props.model}
              availableGroups={['default', 'premium']}
            />
          )}
        </QueryClientProvider>
      )
      expect(await screen.findByRole('alert')).toHaveTextContent(
        'Failed to load performance data'
      )
      expect(screen.queryByText('100.00%')).not.toBeInTheDocument()
      expect(
        screen.queryByText('No requests in the last 24 hours')
      ).not.toBeInTheDocument()
    }
  )

  test('an unsuccessful API envelope is displayed as an error, not no samples', async () => {
    vi.mocked(api.get).mockResolvedValue({
      data: {
        success: false,
        message: 'database unavailable',
        data: { models: [] },
      },
    })
    render(
      <QueryClientProvider client={client}>
        <ModelDetailsContent {...props} />
      </QueryClientProvider>
    )
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Failed to load performance data'
    )
    expect(screen.queryByText('100.00%')).not.toBeInTheDocument()
  })

  test('a failed refresh replaces cached healthy summary data with an error', async () => {
    client.setQueryData(['perf-metrics-summary', 24], {
      ...summary,
      data: { models: [{ ...summary.data.models[0], success_rate: 100 }] },
    })
    vi.mocked(api.get).mockRejectedValue(new Error('HTTP 500'))
    render(
      <QueryClientProvider client={client}>
        <ModelDetailsContent {...props} />
      </QueryClientProvider>
    )
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Failed to load performance data'
    )
    expect(screen.queryByText('100.00%')).not.toBeInTheDocument()
  })

  test.each(['overview', 'performance'])(
    '%s shows loading until the first metrics request resolves',
    (view) => {
      vi.mocked(api.get).mockImplementation(() => new Promise(() => {}))
      render(
        <QueryClientProvider client={client}>
          {view === 'overview' ? (
            <ModelDetailsContent {...props} />
          ) : (
            <ModelDetailsPerformance
              model={props.model}
              availableGroups={['default', 'premium']}
            />
          )}
        </QueryClientProvider>
      )
      expect(screen.getByRole('status')).toHaveTextContent('Loading...')
      expect(screen.queryByText('100.00%')).not.toBeInTheDocument()
      expect(
        screen.queryByText('No requests in the last 24 hours')
      ).not.toBeInTheDocument()
    }
  )

  test.each([true, false])(
    'overview applies noDataAsFull=%s only after a successful empty response',
    async (noDataAsFull) => {
      client.setQueryData(['status'], { perf_no_data_as_full: noDataAsFull })
      vi.mocked(api.get).mockResolvedValue({
        data: { success: true, data: { models: [] } },
      })
      render(
        <QueryClientProvider client={client}>
          <ModelDetailsContent {...props} />
        </QueryClientProvider>
      )
      await screen.findByText('Success rate')
      expect(screen.queryByText('100.00%') !== null).toBe(noDataAsFull)
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    }
  )

  test('a measured zero summary remains zero with noDataAsFull enabled', async () => {
    vi.mocked(api.get).mockResolvedValue({
      data: {
        ...summary,
        data: { models: [{ ...summary.data.models[0], success_rate: 0 }] },
      },
    })
    render(
      <QueryClientProvider client={client}>
        <ModelDetailsContent {...props} />
      </QueryClientProvider>
    )
    expect(await screen.findByText('0.00%')).toBeInTheDocument()
    expect(screen.queryByText('100.00%')).not.toBeInTheDocument()
  })
})
