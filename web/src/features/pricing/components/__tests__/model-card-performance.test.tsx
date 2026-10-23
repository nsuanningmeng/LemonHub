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
import { render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import type { PricingModel } from '../../types'
import { ModelCardGrid } from '../model-card-grid'

const model: PricingModel = {
  id: 1,
  model_name: 'test-model',
  quota_type: 0,
  model_ratio: 1,
  completion_ratio: 1,
  enable_groups: ['default'],
}

describe('model card performance status', () => {
  let queryClient: QueryClient

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    })
    queryClient.setQueryData(['status'], { perf_no_data_as_full: true })
  })

  afterEach(() => {
    queryClient.clear()
  })

  function renderCard() {
    return render(
      <QueryClientProvider client={queryClient}>
        <ModelCardGrid models={[model]} onModelClick={() => undefined} />
      </QueryClientProvider>
    )
  }

  test('a pending summary shows a loading state without a healthy success rate', () => {
    vi.spyOn(api, 'get').mockReturnValue(new Promise(() => undefined))

    renderCard()

    expect(screen.getByRole('status')).toHaveTextContent('Loading...')
    expect(screen.queryByTitle(/Success rate:/)).not.toBeInTheDocument()
  })

  test.each([true, false])(
    'a failed summary shows an error when noDataAsFull is %s',
    async (noDataAsFull) => {
      queryClient.setQueryData(['status'], {
        perf_no_data_as_full: noDataAsFull,
      })
      vi.spyOn(api, 'get').mockRejectedValue(new Error('HTTP 500'))

      renderCard()

      expect(await screen.findByRole('alert')).toHaveTextContent(
        'Failed to load performance data'
      )
      expect(screen.queryByTitle(/Success rate:/)).not.toBeInTheDocument()
      expect(
        screen.queryByText('No requests in the last 24 hours')
      ).not.toBeInTheDocument()
    }
  )

  test('a failed refresh replaces cached healthy metrics with an error state', async () => {
    queryClient.setQueryData(['perf-metrics-summary', 24], {
      success: true,
      data: {
        models: [
          {
            model_name: model.model_name,
            avg_latency_ms: 200,
            avg_tps: 20,
            success_rate: 100,
          },
        ],
      },
    })
    vi.spyOn(api, 'get').mockRejectedValue(new Error('HTTP 500'))

    renderCard()

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Failed to load performance data'
    )
    expect(screen.queryByTitle(/Success rate:/)).not.toBeInTheDocument()
  })

  test.each([true, false])(
    'a successful empty summary preserves the no-data setting %s',
    async (noDataAsFull) => {
      queryClient.setQueryData(['status'], {
        perf_no_data_as_full: noDataAsFull,
      })
      vi.spyOn(api, 'get').mockResolvedValue({
        data: { success: true, data: { models: [] } },
      })

      renderCard()

      await waitFor(() => {
        expect(screen.queryByRole('status')).not.toBeInTheDocument()
      })
      if (noDataAsFull) {
        expect(
          await screen.findByTitle('Success rate: 100.0%')
        ).toBeInTheDocument()
      } else {
        expect(screen.queryByTitle(/Success rate:/)).not.toBeInTheDocument()
      }
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    }
  )

  test('a measured zero success rate remains zero when noDataAsFull is enabled', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: {
          models: [
            {
              model_name: model.model_name,
              avg_latency_ms: 200,
              avg_tps: 20,
              success_rate: 0,
            },
          ],
        },
      },
    })

    renderCard()

    expect(await screen.findByTitle('Success rate: 0.0%')).toBeInTheDocument()
    expect(
      screen.queryByTitle('Success rate: 100.0%')
    ).not.toBeInTheDocument()
  })
})
