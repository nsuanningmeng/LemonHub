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
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { CampaignForm } from '../campaign-form'

const originalConfig = useSystemConfigStore.getState().config
let client: QueryClient

beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  useSystemConfigStore.getState().setConfig({
    currency: { ...DEFAULT_CURRENCY_CONFIG, quotaPerUnit: 500000 },
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  useSystemConfigStore.getState().setConfig(originalConfig)
  localStorage.clear()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

function renderCampaignForm(onCreated?: () => void) {
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18next}>
        <CampaignForm onCreated={onCreated} />
      </I18nextProvider>
    </QueryClientProvider>
  )
}

function enterCampaign() {
  fireEvent.change(screen.getByLabelText('Campaign title'), {
    target: { value: 'October loyalty reward' },
  })
  fireEvent.change(screen.getByLabelText('Amount per user (USD)'), {
    target: { value: '2.5' },
  })
  fireEvent.change(screen.getByLabelText('Start time'), {
    target: { value: '2026-10-01T00:00' },
  })
  fireEvent.change(screen.getByLabelText('End time'), {
    target: { value: '2026-10-08T00:00' },
  })
}

async function previewRecipients() {
  fireEvent.click(screen.getByRole('button', { name: 'Preview recipients' }))
  await screen.findByText('Eligible recipients')
}

async function confirmDistribution() {
  fireEvent.click(screen.getByRole('button', { name: 'Issue rewards' }))
  await screen.findByRole('alertdialog', {
    name: 'Confirm reward distribution',
  })
  fireEvent.click(screen.getByRole('button', { name: 'Confirm and issue' }))
}

describe('reward campaign form', () => {
  test('defaults to seven valid days and includes an edited validity in the preview, confirmation and issuance', async () => {
    const issued = vi.fn()
    const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
      if (url === '/api/reward-campaigns/preview') {
        return { data: { success: true, data: { recipient_count: 3 } } }
      }
      return {
        data: { success: true, data: { id: 1, recipient_count: 3 } },
      }
    })
    renderCampaignForm(issued)
    enterCampaign()
    const validity = screen.getByLabelText('Reward validity (days)')
    expect(validity).toHaveValue(7)
    fireEvent.change(validity, { target: { value: '14' } })

    await previewRecipients()
    expect(screen.getByText('Valid for 14 days')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Issue rewards' }))
    const confirmation = await screen.findByRole('alertdialog', {
      name: 'Confirm reward distribution',
    })
    expect(within(confirmation).getByText('Valid for 14 days')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Confirm and issue' }))

    await waitFor(() => expect(issued).toHaveBeenCalledOnce())
    expect(post.mock.calls[1]?.[1]).toMatchObject({ expires_in_days: 14 })
  })

  test('editing reward validity requires a new recipient preview before issuance', async () => {
    vi.spyOn(api, 'post').mockResolvedValue({
      data: { success: true, data: { recipient_count: 3 } },
    })
    renderCampaignForm()
    enterCampaign()
    await previewRecipients()

    fireEvent.change(screen.getByLabelText('Reward validity (days)'), {
      target: { value: '14' },
    })

    await waitFor(() =>
      expect(screen.queryByText('Eligible recipients')).not.toBeInTheDocument()
    )
    expect(
      screen.queryByRole('button', { name: 'Issue rewards' })
    ).not.toBeInTheDocument()
  })

  test('shows the four activity audiences in the requested order', () => {
    renderCampaignForm()

    const options = within(
      screen.getByLabelText('Eligible users')
    ).getAllByRole('option')

    expect(options.map((option) => option.textContent)).toEqual([
      'Users with successful top-ups',
      'Users with successful API calls',
      'Users with successful top-ups and API calls',
      'Users with successful top-ups or API calls',
    ])
    expect(
      screen.getByText(
        'Recipients must be enabled. Within the selected period, “and” requires both a successful top-up and a successful API call; “or” requires either activity.'
      )
    ).toBeVisible()
  })

  test('selecting both activities previews and issues rewards to the intersection audience', async () => {
    const issued = vi.fn()
    const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
      if (url === '/api/reward-campaigns/preview') {
        return { data: { success: true, data: { recipient_count: 2 } } }
      }
      return {
        data: { success: true, data: { id: 1, recipient_count: 2 } },
      }
    })
    renderCampaignForm(issued)
    enterCampaign()
    fireEvent.change(screen.getByLabelText('Eligible users'), {
      target: { value: 'both' },
    })

    await previewRecipients()
    expect(post.mock.calls[0]?.[1]).toMatchObject({ audience: 'both' })
    await confirmDistribution()

    await waitFor(() => expect(issued).toHaveBeenCalledOnce())
    expect(post.mock.calls[1]?.[1]).toMatchObject({
      audience: 'both',
      expected_recipient_count: 2,
    })
  })

  test('switching the activity audience invalidates the existing recipient preview', async () => {
    vi.spyOn(api, 'post').mockResolvedValue({
      data: { success: true, data: { recipient_count: 3 } },
    })
    renderCampaignForm()
    enterCampaign()
    await previewRecipients()

    fireEvent.change(screen.getByLabelText('Eligible users'), {
      target: { value: 'topup' },
    })

    expect(screen.getByLabelText('Eligible users')).toHaveValue('topup')
    await waitFor(() =>
      expect(screen.queryByText('Eligible recipients')).not.toBeInTheDocument()
    )
    expect(
      screen.queryByRole('button', { name: 'Issue rewards' })
    ).not.toBeInTheDocument()
  })

  test('previews eligible users and issues a fixed reward in wallet quota units', async () => {
    const issued = vi.fn()
    const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
      if (url === '/api/reward-campaigns/preview') {
        return { data: { success: true, data: { recipient_count: 3 } } }
      }
      return {
        data: { success: true, data: { id: 1, recipient_count: 3 } },
      }
    })
    renderCampaignForm(issued)
    enterCampaign()

    await previewRecipients()
    expect(post.mock.calls[0]).toEqual([
      '/api/reward-campaigns/preview',
      {
        audience: 'either',
        start_time: new Date('2026-10-01T00:00').getTime() / 1000,
        end_time: new Date('2026-10-08T00:00').getTime() / 1000,
      },
    ])
    await confirmDistribution()

    await waitFor(() => expect(issued).toHaveBeenCalledOnce())
    expect(post.mock.calls[1]?.[0]).toBe('/api/reward-campaigns')
    expect(post.mock.calls[1]?.[1]).toMatchObject({
      title: 'October loyalty reward',
      type: 'fixed',
      min_quota: 1250000,
      max_quota: 1250000,
      audience: 'either',
      expected_recipient_count: 3,
      expires_in_days: 7,
      request_id: expect.any(String),
    })
    expect(screen.queryByText('Eligible recipients')).not.toBeInTheDocument()
  })

  test('invalid random amount bounds prevent recipient preview', async () => {
    const post = vi.spyOn(api, 'post')
    renderCampaignForm()
    enterCampaign()
    fireEvent.change(screen.getByLabelText('Reward type'), {
      target: { value: 'random' },
    })
    fireEvent.change(screen.getByLabelText('Minimum amount (USD)'), {
      target: { value: '5' },
    })
    fireEvent.change(screen.getByLabelText('Maximum amount (USD)'), {
      target: { value: '2' },
    })

    fireEvent.click(screen.getByRole('button', { name: 'Preview recipients' }))

    await waitFor(() =>
      expect(screen.getByLabelText('Maximum amount (USD)')).toHaveAttribute(
        'aria-invalid',
        'true'
      )
    )
    expect(post).not.toHaveBeenCalled()
  })

  test('editing an amount after preview requires a new preview', async () => {
    vi.spyOn(api, 'post').mockResolvedValue({
      data: { success: true, data: { recipient_count: 3 } },
    })
    renderCampaignForm()
    enterCampaign()
    await previewRecipients()

    fireEvent.change(screen.getByLabelText('Amount per user (USD)'), {
      target: { value: '4' },
    })

    await waitFor(() =>
      expect(screen.queryByText('Eligible recipients')).not.toBeInTheDocument()
    )
    const issueButton = screen.queryByRole('button', { name: 'Issue rewards' })
    if (issueButton) expect(issueButton).toBeDisabled()
  })

  test('a preview with no eligible users cannot issue rewards', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      data: { success: true, data: { recipient_count: 0 } },
    })
    renderCampaignForm()
    enterCampaign()

    await previewRecipients()

    expect(screen.getByRole('button', { name: 'Issue rewards' })).toBeDisabled()
    expect(post).toHaveBeenCalledOnce()
  })

  test('a preview response arriving after the form changes cannot enable issuance', async () => {
    let resolvePreview!: (value: {
      data: { success: boolean; data: { recipient_count: number } }
    }) => void
    const previewResponse = new Promise<{
      data: { success: boolean; data: { recipient_count: number } }
    }>((resolve) => {
      resolvePreview = resolve
    })
    const post = vi.spyOn(api, 'post').mockReturnValue(previewResponse)
    renderCampaignForm()
    enterCampaign()
    fireEvent.click(screen.getByRole('button', { name: 'Preview recipients' }))
    await waitFor(() => expect(post).toHaveBeenCalledOnce())

    fireEvent.change(screen.getByLabelText('Amount per user (USD)'), {
      target: { value: '4' },
    })
    await act(async () =>
      resolvePreview({
        data: { success: true, data: { recipient_count: 3 } },
      })
    )

    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Preview recipients' })
      ).toBeEnabled()
    )
    expect(screen.queryByText('Eligible recipients')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Issue rewards' })
    ).not.toBeInTheDocument()
  })

  test('a changed recipient count requires a fresh preview before retrying issuance', async () => {
    vi.spyOn(api, 'post').mockImplementation(async (url) => {
      if (url === '/api/reward-campaigns/preview') {
        return { data: { success: true, data: { recipient_count: 3 } } }
      }
      return {
        data: {
          success: false,
          message: 'reward audience changed since preview',
        },
      }
    })
    renderCampaignForm()
    enterCampaign()
    await previewRecipients()

    await confirmDistribution()

    await waitFor(() =>
      expect(screen.queryByText('Eligible recipients')).not.toBeInTheDocument()
    )
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Preview recipients' })
    ).toBeEnabled()
  })

  test.each(['available', 'unavailable'] as const)(
    'pending issuance blocks repeat clicks and a failed retry preserves the request ID when crypto.randomUUID is %s',
    async (randomUUIDAvailability) => {
      if (randomUUIDAvailability === 'unavailable') {
        vi.stubGlobal('crypto', {
          getRandomValues: globalThis.crypto.getRandomValues.bind(
            globalThis.crypto
          ),
        })
      }
      let rejectIssue!: (reason: Error) => void
      const pendingIssue = new Promise<never>((_resolve, reject) => {
        rejectIssue = reject
      })
      const issued = vi.fn()
      const requests: unknown[] = []
      vi.spyOn(api, 'post').mockImplementation(async (url, body) => {
        if (url === '/api/reward-campaigns/preview') {
          return { data: { success: true, data: { recipient_count: 2 } } }
        }
        requests.push(body)
        if (requests.length === 1) return pendingIssue
        return {
          data: { success: true, data: { id: 2, recipient_count: 2 } },
        }
      })
      renderCampaignForm(issued)
      enterCampaign()
      await previewRecipients()
      await confirmDistribution()
      await waitFor(() => expect(requests).toHaveLength(1))
      expect(screen.getByRole('button', { name: 'Sending...' })).toBeDisabled()

      await act(async () => rejectIssue(new Error('Connection interrupted')))
      await waitFor(() =>
        expect(
          screen.getByRole('button', { name: 'Confirm and issue' })
        ).toBeEnabled()
      )
      fireEvent.click(screen.getByRole('button', { name: 'Confirm and issue' }))

      await waitFor(() => expect(issued).toHaveBeenCalledOnce())
      expect(requests).toHaveLength(2)
      expect(requests[0]).toMatchObject({
        request_id: expect.stringMatching(/^[A-Za-z0-9_.-]{1,64}$/),
      })
      expect(requests[1]).toEqual(requests[0])
    }
  )
})
