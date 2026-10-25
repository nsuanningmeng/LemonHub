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
import userEvent from '@testing-library/user-event'
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

async function selectCampaignOption(label: string, option: string) {
  const user = userEvent.setup()
  await user.click(screen.getByRole('combobox', { name: label }))
  await user.click(await screen.findByRole('option', { name: option }))
}

async function previewRecipients() {
  fireEvent.click(screen.getByRole('button', { name: 'Preview recipients' }))
  await screen.findByText('Current eligible users')
}

async function confirmDistribution() {
  fireEvent.click(screen.getByRole('button', { name: 'Create campaign' }))
  await screen.findByRole('alertdialog', {
    name: 'Confirm campaign creation',
  })
  fireEvent.click(screen.getByRole('button', { name: 'Confirm and create' }))
}

describe('reward campaign form', () => {
  test('creates a continuous campaign without preview and never sends a recipient-count condition', async () => {
    const created = vi.fn()
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      data: { success: true, data: { id: 1, recipient_count: 0 } },
    })
    renderCampaignForm(created)
    enterCampaign()

    fireEvent.click(screen.getByRole('button', { name: 'Create campaign' }))
    const confirmation = await screen.findByRole('alertdialog', {
      name: 'Confirm campaign creation',
    })
    expect(within(confirmation).getAllByText('Not previewed')).toHaveLength(2)
    expect(within(confirmation).getByText('$2.5')).toBeVisible()
    expect(within(confirmation).getByText('Activity period')).toBeVisible()
    expect(post).not.toHaveBeenCalled()
    fireEvent.click(
      within(confirmation).getByRole('button', { name: 'Confirm and create' })
    )

    await waitFor(() => expect(created).toHaveBeenCalledOnce())
    expect(post).toHaveBeenCalledOnce()
    expect(post.mock.calls[0]?.[0]).toBe('/api/reward-campaigns')
    expect(post.mock.calls[0]?.[1]).toMatchObject({ continuous: true })
    expect(post.mock.calls[0]?.[1]).not.toHaveProperty(
      'expected_recipient_count'
    )
  })

  test('a failed optional preview still permits direct campaign creation', async () => {
    const created = vi.fn()
    vi.spyOn(api, 'post').mockImplementation(async (url) => {
      if (url === '/api/reward-campaigns/preview') {
        throw new Error('Preview unavailable')
      }
      return { data: { success: true, data: { id: 1, recipient_count: 0 } } }
    })
    renderCampaignForm(created)
    enterCampaign()
    fireEvent.click(screen.getByRole('button', { name: 'Preview recipients' }))
    await screen.findByText('Failed to preview recipients')

    fireEvent.click(screen.getByRole('button', { name: 'Create campaign' }))
    await screen.findByRole('alertdialog', {
      name: 'Confirm campaign creation',
    })
    fireEvent.click(screen.getByRole('button', { name: 'Confirm and create' }))

    await waitFor(() => expect(created).toHaveBeenCalledOnce())
  })

  test('the reward type menu supports keyboard selection and restores trigger focus', async () => {
    const user = userEvent.setup()
    renderCampaignForm()
    const trigger = screen.getByRole('combobox', { name: 'Reward type' })
    expect(trigger).toHaveAttribute('aria-expanded', 'false')

    act(() => trigger.focus())
    await user.keyboard('{ArrowDown}')
    expect(await screen.findByRole('listbox')).toBeVisible()
    expect(trigger).toHaveAttribute('aria-expanded', 'true')
    await user.keyboard('{ArrowDown}{Enter}')

    await waitFor(() =>
      expect(trigger).toHaveAttribute('aria-expanded', 'false')
    )
    expect(trigger).toHaveTextContent('Lucky red packet')
    expect(trigger).toHaveFocus()
    expect(screen.getByLabelText('Maximum amount (USD)')).toBeVisible()
  })

  test.each(['Reward type', 'Eligible users'])(
    'Escape dismisses the %s menu without changing its selection',
    async (name) => {
      const user = userEvent.setup()
      renderCampaignForm()
      const trigger = screen.getByRole('combobox', { name })
      const initialLabel = trigger.textContent
      await user.click(trigger)
      const list = await screen.findByRole('listbox')
      expect(within(list).getByRole('option', { selected: true })).toBeVisible()

      await user.keyboard('{Escape}')

      await waitFor(() =>
        expect(trigger).toHaveAttribute('aria-expanded', 'false')
      )
      expect(trigger).toHaveTextContent(initialLabel ?? '')
      expect(trigger).toHaveFocus()
      expect(screen.queryByRole('listbox')).not.toBeInTheDocument()
    }
  )

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
    expect(screen.getByText('14 days per reward')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Create campaign' }))
    const confirmation = await screen.findByRole('alertdialog', {
      name: 'Confirm campaign creation',
    })
    expect(within(confirmation).getByText('14 days per reward')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Confirm and create' }))

    await waitFor(() => expect(issued).toHaveBeenCalledOnce())
    expect(post.mock.calls[1]?.[1]).toMatchObject({ expires_in_days: 14 })
  })

  test('editing reward validity discards the old preview and keeps direct creation available', async () => {
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
      expect(
        screen.queryByText('Current eligible users')
      ).not.toBeInTheDocument()
    )
    expect(
      screen.getByRole('button', { name: 'Create campaign' })
    ).toBeEnabled()
  })

  test('shows the four activity audiences in the requested order', async () => {
    renderCampaignForm()

    await userEvent.click(
      screen.getByRole('combobox', { name: 'Eligible users' })
    )
    const options = within(await screen.findByRole('listbox')).getAllByRole(
      'option'
    )

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
    await selectCampaignOption(
      'Eligible users',
      'Users with successful top-ups and API calls'
    )

    await previewRecipients()
    expect(post.mock.calls[0]?.[1]).toMatchObject({ audience: 'both' })
    await confirmDistribution()

    await waitFor(() => expect(issued).toHaveBeenCalledOnce())
    expect(post.mock.calls[1]?.[1]).toMatchObject({
      audience: 'both',
      continuous: true,
    })
  })

  test('switching the activity audience invalidates the existing recipient preview', async () => {
    vi.spyOn(api, 'post').mockResolvedValue({
      data: { success: true, data: { recipient_count: 3 } },
    })
    renderCampaignForm()
    enterCampaign()
    await previewRecipients()

    await selectCampaignOption(
      'Eligible users',
      'Users with successful top-ups'
    )

    expect(
      screen.getByRole('combobox', { name: 'Eligible users' })
    ).toHaveTextContent('Users with successful top-ups')
    await waitFor(() =>
      expect(
        screen.queryByText('Current eligible users')
      ).not.toBeInTheDocument()
    )
    expect(
      screen.getByRole('button', { name: 'Create campaign' })
    ).toBeEnabled()
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
      continuous: true,
      expires_in_days: 7,
      request_id: expect.any(String),
    })
    expect(screen.queryByText('Current eligible users')).not.toBeInTheDocument()
  })

  test('invalid random amount bounds prevent recipient preview', async () => {
    const post = vi.spyOn(api, 'post')
    renderCampaignForm()
    enterCampaign()
    await selectCampaignOption('Reward type', 'Lucky red packet')
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

  test('editing an amount after preview discards the old estimate and permits direct creation', async () => {
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
      expect(
        screen.queryByText('Current eligible users')
      ).not.toBeInTheDocument()
    )
    expect(
      screen.getByRole('button', { name: 'Create campaign' })
    ).toBeEnabled()
  })

  test.each([
    { count: 0, amount: '2.5' },
    { count: 100001, amount: '2.5' },
    { count: 3, amount: '10000000000' },
  ])(
    'an optional preview of $count users at $amount each does not cap campaign creation',
    async ({ count, amount }) => {
      const created = vi.fn()
      const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
        if (url === '/api/reward-campaigns/preview') {
          return { data: { success: true, data: { recipient_count: count } } }
        }
        return { data: { success: true, data: { id: 1, recipient_count: 0 } } }
      })
      renderCampaignForm(created)
      enterCampaign()
      fireEvent.change(screen.getByLabelText('Amount per user (USD)'), {
        target: { value: amount },
      })
      await previewRecipients()

      expect(
        screen.getByRole('button', { name: 'Create campaign' })
      ).toBeEnabled()
      await confirmDistribution()

      await waitFor(() => expect(created).toHaveBeenCalledOnce())
      expect(post.mock.calls[1]?.[1]).not.toHaveProperty(
        'expected_recipient_count'
      )
    }
  )

  test('a preview response arriving after a reward type selection cannot restore a stale estimate', async () => {
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

    await selectCampaignOption('Reward type', 'Lucky red packet')
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
    expect(screen.queryByText('Current eligible users')).not.toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Create campaign' })
    ).toBeEnabled()
  })

  test('changing current recipient counts do not add an issuance precondition', async () => {
    const created = vi.fn()
    const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
      if (url === '/api/reward-campaigns/preview') {
        return { data: { success: true, data: { recipient_count: 3 } } }
      }
      return { data: { success: true, data: { id: 1, recipient_count: 5 } } }
    })
    renderCampaignForm(created)
    enterCampaign()
    await previewRecipients()
    await confirmDistribution()

    await waitFor(() => expect(created).toHaveBeenCalledOnce())
    expect(post.mock.calls[1]?.[1]).not.toHaveProperty(
      'expected_recipient_count'
    )
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  })

  test('creating while a preview is pending ignores its late estimate', async () => {
    let resolvePreview!: (value: {
      data: { success: boolean; data: { recipient_count: number } }
    }) => void
    const pendingPreview = new Promise<{
      data: { success: boolean; data: { recipient_count: number } }
    }>((resolve) => {
      resolvePreview = resolve
    })
    const created = vi.fn()
    vi.spyOn(api, 'post').mockImplementation(async (url) => {
      if (url === '/api/reward-campaigns/preview') return pendingPreview
      return { data: { success: true, data: { id: 1, recipient_count: 0 } } }
    })
    renderCampaignForm(created)
    enterCampaign()
    fireEvent.click(screen.getByRole('button', { name: 'Preview recipients' }))
    await screen.findByRole('button', { name: 'Loading...' })

    await confirmDistribution()
    await waitFor(() => expect(created).toHaveBeenCalledOnce())
    await act(async () =>
      resolvePreview({ data: { success: true, data: { recipient_count: 3 } } })
    )

    expect(screen.queryByText('Current eligible users')).not.toBeInTheDocument()
  })

  test('editing after a failed creation replaces the confirmation payload and request ID', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      data: { success: false, message: 'Temporary creation failure' },
    })
    renderCampaignForm()
    enterCampaign()
    await confirmDistribution()
    await screen.findByText(
      'Failed to create reward campaign. You can safely retry this request.'
    )
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    fireEvent.change(screen.getByLabelText('Amount per user (USD)'), {
      target: { value: '4' },
    })

    await confirmDistribution()
    await waitFor(() => expect(post).toHaveBeenCalledTimes(2))

    expect(post.mock.calls[1]?.[1]).toMatchObject({
      min_quota: 2000000,
      max_quota: 2000000,
    })
    const firstRequest = post.mock.calls[0]?.[1] as { request_id: string }
    const editedRequest = post.mock.calls[1]?.[1] as { request_id: string }
    expect(editedRequest.request_id).not.toBe(firstRequest.request_id)
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
      expect(screen.getByRole('button', { name: 'Creating...' })).toBeDisabled()

      await act(async () => rejectIssue(new Error('Connection interrupted')))
      await waitFor(() =>
        expect(
          screen.getByRole('button', { name: 'Confirm and create' })
        ).toBeEnabled()
      )
      fireEvent.click(
        screen.getByRole('button', { name: 'Confirm and create' })
      )

      await waitFor(() => expect(issued).toHaveBeenCalledOnce())
      expect(requests).toHaveLength(2)
      expect(requests[0]).toMatchObject({
        request_id: expect.stringMatching(/^[A-Za-z0-9_.-]{1,64}$/),
      })
      expect(requests[1]).toEqual(requests[0])
    }
  )
})
