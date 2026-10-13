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
import { cleanup, render, screen, within } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'

import { EmailPromotionSection } from '../email-promotion-section'

type CampaignFixture = {
  status: 'pending' | 'sending' | 'completed' | 'failed'
  total_count: number
  sent_count: number
  fail_count: number
  skip_count?: number | null
}

const clients: QueryClient[] = []

afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
})

async function renderCampaign(campaign: CampaignFixture, language = 'en') {
  const i18n = createInstance()
  await i18n.init({
    lng: language,
    fallbackLng: 'en',
    nsSeparator: false,
    resources: { en, zh },
  })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        page: 1,
        page_size: 10,
        total: 1,
        items: [
          {
            id: 1,
            subject: 'Model availability notice',
            content: 'Models are changing.',
            target_group: '',
            target_status: 1,
            source: 'manual',
            created_at: 1791878400,
            finished_at: 0,
            ...campaign,
          },
        ],
      },
    },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>
        <EmailPromotionSection
          defaultValues={{ announcementEmailEnabled: false, ratePerMinute: 60 }}
        />
      </I18nextProvider>
    </QueryClientProvider>
  )
  return within(
    await screen.findByRole('row', { name: /Model availability notice/ })
  )
}

test('completed campaign counts skipped recipients as processed and shows each outcome', async () => {
  const row = await renderCampaign({
    status: 'completed',
    total_count: 5817,
    sent_count: 3551,
    fail_count: 0,
    skip_count: 2266,
  })

  expect(row.getByText('Processed: 5817/5817')).toBeInTheDocument()
  expect(row.getByText('Sent: 3551')).toBeInTheDocument()
  expect(row.getByText('Failed: 0')).toBeInTheDocument()
  expect(row.getByText('Skipped: 2266')).toBeInTheDocument()
})

test('completed campaign with delivery failures includes failures in progress and names them in its status', async () => {
  const row = await renderCampaign({
    status: 'completed',
    total_count: 5,
    sent_count: 3,
    fail_count: 1,
    skip_count: 1,
  })

  expect(row.getByText('Processed: 5/5')).toBeInTheDocument()
  expect(row.getByText('Failed: 1')).toBeInTheDocument()
  expect(row.getByText('Processing finished with failures')).toBeInTheDocument()
})

test.each(['sending', 'failed'] as const)(
  '%s campaign keeps unprocessed recipients out of its progress',
  async (status) => {
    const row = await renderCampaign({
      status,
      total_count: 10,
      sent_count: 2,
      fail_count: 1,
      skip_count: 2,
    })

    expect(row.getByText('Processed: 5/10')).toBeInTheDocument()
    expect(row.queryByText(/Processing finished/)).not.toBeInTheDocument()
  }
)

test('Chinese campaign progress translates processing and outcome labels with their counts', async () => {
  const row = await renderCampaign(
    {
      status: 'completed',
      total_count: 5,
      sent_count: 3,
      fail_count: 1,
      skip_count: 1,
    },
    'zh'
  )

  expect(row.getByText('已处理：5/5')).toBeInTheDocument()
  expect(row.getByText('成功：3')).toBeInTheDocument()
  expect(row.getByText('失败：1')).toBeInTheDocument()
  expect(row.getByText('跳过：1')).toBeInTheDocument()
  expect(row.getByText('处理已结束（有失败）')).toBeInTheDocument()
})

test('pending campaign with recorded zero skips shows zero processed', async () => {
  const row = await renderCampaign({
    status: 'pending',
    total_count: 10,
    sent_count: 0,
    fail_count: 0,
    skip_count: 0,
  })

  expect(row.getByText('Processed: 0/10')).toBeInTheDocument()
  expect(row.getByText('Skipped: 0')).toBeInTheDocument()
  expect(row.queryByText('Skipped: not recorded')).not.toBeInTheDocument()
})

test.each([null, undefined])(
  'completed legacy campaign with skip_count=%s reports missing statistics without inventing progress',
  async (skipCount) => {
    const row = await renderCampaign({
      status: 'completed',
      total_count: 5817,
      sent_count: 3551,
      fail_count: 0,
      skip_count: skipCount,
    })

    expect(row.getByText('Sent: 3551')).toBeInTheDocument()
    expect(row.getByText('Failed: 0')).toBeInTheDocument()
    expect(row.getByText('Skipped: not recorded')).toBeInTheDocument()
    expect(
      row.getByRole('cell', { name: 'Processing finished' })
    ).toBeInTheDocument()
    expect(row.queryByText(/3551\/5817/)).not.toBeInTheDocument()
    expect(row.queryByText(/2266/)).not.toBeInTheDocument()
  }
)

test.each(['sending', 'failed'] as const)(
  'legacy %s campaign labels known attempts separately from unknown skipped recipients',
  async (status) => {
    const row = await renderCampaign({
      status,
      total_count: 10,
      sent_count: 2,
      fail_count: 1,
      skip_count: null,
    })

    expect(row.getByText('Attempted: 3 · Candidates: 10')).toBeInTheDocument()
    expect(row.getByText('Skipped: not recorded')).toBeInTheDocument()
    expect(row.queryByText(/Processed:/)).not.toBeInTheDocument()
  }
)
