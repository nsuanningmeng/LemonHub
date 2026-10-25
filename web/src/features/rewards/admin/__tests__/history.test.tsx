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
import {
  focusManager,
  QueryClient,
  QueryClientProvider,
} from '@tanstack/react-query'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { formatTimestamp } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { CampaignHistory } from '../campaign-history'
import type { RewardCampaign } from '../types'

const originalConfig = useSystemConfigStore.getState().config
const originalAuth = useAuthStore.getState().auth
let client: QueryClient

const unclaimed: RewardCampaign & {
  claimed_count: number
  claimed_quota: number
  expires_in_days: number
  expires_at: number
} = {
  id: 1,
  title: 'Unclaimed fixed reward',
  description: '',
  type: 'fixed',
  min_quota: 500000,
  max_quota: 500000,
  audience: 'either',
  start_time: 1790812800,
  end_time: 1791417600,
  request_id: 'fixed-reward',
  created_at: 1791417600,
  created_by: 1,
  recipient_count: 3,
  claimed_count: 0,
  claimed_quota: 0,
  expires_in_days: 7,
  expires_at: 1792022400,
}
const partial: typeof unclaimed = {
  ...unclaimed,
  id: 2,
  title: 'Partly claimed lucky reward',
  type: 'random',
  max_quota: 2500000,
  recipient_count: 5,
  claimed_count: 2,
  claimed_quota: 1875000,
}

beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 1, username: 'admin', role: 100 })
  useSystemConfigStore.getState().setConfig({
    currency: { ...DEFAULT_CURRENCY_CONFIG, quotaPerUnit: 500000 },
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  useAuthStore.setState({ auth: originalAuth })
  useSystemConfigStore.getState().setConfig(originalConfig)
  focusManager.setFocused(undefined)
  localStorage.clear()
  vi.restoreAllMocks()
})

function renderHistory() {
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18next}>
        <CampaignHistory />
      </I18nextProvider>
    </QueryClientProvider>
  )
}

function campaignCells(title: string) {
  return within(
    screen.getByRole('row', { name: new RegExp(title) })
  ).getAllByRole('cell')
}

describe('reward distribution history', () => {
  test.each([
    ['backfilling', 'Processing eligible users'],
    ['scheduled', 'Waiting for activity period'],
    ['active', 'Automatically distributing'],
    ['completed', 'Activity period ended'],
  ])(
    'a continuous %s campaign shows per-user validity and its distribution status',
    async (status, label) => {
      const campaign = {
        ...unclaimed,
        title: 'Future activity reward',
        continuous: true,
        distribution_status: status,
        expires_at: 0,
        recipient_count: 0,
      }
      vi.spyOn(api, 'get').mockResolvedValue({
        data: { success: true, data: { items: [campaign], total: 1 } },
      })
      renderHistory()

      const row = await screen.findByRole('row', {
        name: /Future activity reward/,
      })
      expect(within(row).getByText('7 days per reward')).toBeVisible()
      expect(within(row).getByText(label)).toBeVisible()
      if (status === 'completed') {
        expect(
          within(row).getByText(
            'Late records from the activity period can still qualify for rewards.'
          )
        ).toBeVisible()
      }
      expect(within(row).queryByText('No expiration')).not.toBeInTheDocument()
    }
  )

  test('shows the issued expiry timestamp and explicitly identifies a legacy campaign without expiry', async () => {
    const legacy = {
      ...partial,
      title: 'Legacy reward without expiry',
      continuous: false,
      distribution_status: 'legacy',
      expires_in_days: 0,
      expires_at: 0,
    }
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: { items: [unclaimed, legacy], total: 2 } },
    })
    renderHistory()

    await screen.findByText(unclaimed.title)

    expect(
      screen.getByRole('columnheader', { name: 'Reward validity' })
    ).toBeVisible()
    expect(campaignCells(unclaimed.title)[8]).toHaveTextContent(
      formatTimestamp(unclaimed.expires_at)
    )
    expect(campaignCells(legacy.title)[8]).toHaveTextContent('No expiration')
    expect(campaignCells(legacy.title)[9]).toHaveTextContent(
      'Legacy one-time distribution'
    )
  })

  test('large cumulative claims display the exact quota total instead of a missing or rounded number', async () => {
    useSystemConfigStore.getState().setConfig({
      currency: { ...DEFAULT_CURRENCY_CONFIG, quotaPerUnit: 100 },
    })
    const campaign = {
      ...unclaimed,
      title: 'Large claimed total',
      claimed_quota: null,
      claimed_quota_exact: '9007199254740993',
    }
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: { items: [campaign], total: 1 } },
    })
    renderHistory()

    await screen.findByText(campaign.title)

    expect(campaignCells(campaign.title)[5]).toHaveTextContent(
      '$90,071,992,547,409.93'
    )
  })

  test('an unclaimed campaign displays zero recipients claimed and zero actual amount', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: { items: [unclaimed], total: 1 } },
    })
    renderHistory()

    await screen.findByText(unclaimed.title)

    expect(
      screen.getByRole('columnheader', { name: 'Claimed recipients' })
    ).toBeVisible()
    expect(
      screen.getByRole('columnheader', { name: 'Total claimed amount' })
    ).toBeVisible()
    const cells = campaignCells(unclaimed.title)
    expect(cells[3]).toHaveTextContent(/^3$/)
    expect(cells[4]).toHaveTextContent(/^0$/)
    expect(cells[5]).toHaveTextContent(/^\$0$/)
  })

  test('a partly claimed random campaign displays the actual total instead of the configured reward range', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: { items: [partial], total: 1 } },
    })
    renderHistory()

    await screen.findByText(partial.title)

    const cells = campaignCells(partial.title)
    expect(cells[2]).toHaveTextContent('$1 – $5')
    expect(cells[3]).toHaveTextContent(/^5$/)
    expect(cells[4]).toHaveTextContent(/^2$/)
    expect(cells[5]).toHaveTextContent(/^\$3.75$/)
  })

  test('moving between history pages keeps each campaign claim totals on its own page', async () => {
    const get = vi
      .spyOn(api, 'get')
      .mockImplementation(async (_url, config) => {
        const params = config?.params as { p: number } | undefined
        return {
          data: {
            success: true,
            data: {
              items: [params?.p === 2 ? partial : unclaimed],
              total: 11,
            },
          },
        }
      })
    renderHistory()
    await screen.findByText(unclaimed.title)
    expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled()

    fireEvent.click(screen.getByRole('button', { name: 'Next' }))
    await screen.findByText(partial.title)

    expect(get).toHaveBeenLastCalledWith('/api/reward-campaigns', {
      params: { p: 2, page_size: 10 },
    })
    expect(screen.queryByText(unclaimed.title)).not.toBeInTheDocument()
    expect(campaignCells(partial.title)[5]).toHaveTextContent(/^\$3.75$/)
    expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: 'Previous' }))
    await screen.findByText(unclaimed.title)
    expect(campaignCells(unclaimed.title)[5]).toHaveTextContent(/^\$0$/)
  })

  test('returning to the page refreshes claim totals after a user claims a reward', async () => {
    const get = vi
      .spyOn(api, 'get')
      .mockResolvedValueOnce({
        data: { success: true, data: { items: [unclaimed], total: 1 } },
      })
      .mockResolvedValue({
        data: {
          success: true,
          data: {
            items: [{ ...unclaimed, claimed_count: 1, claimed_quota: 500000 }],
            total: 1,
          },
        },
      })
    renderHistory()
    await screen.findByText(unclaimed.title)

    await act(async () => {
      focusManager.setFocused(false)
      focusManager.setFocused(true)
    })

    await waitFor(() => expect(get).toHaveBeenCalledTimes(2))
    await waitFor(() =>
      expect(campaignCells(unclaimed.title)[4]).toHaveTextContent(/^1$/)
    )
    expect(campaignCells(unclaimed.title)[5]).toHaveTextContent(/^\$1$/)
  })
})
