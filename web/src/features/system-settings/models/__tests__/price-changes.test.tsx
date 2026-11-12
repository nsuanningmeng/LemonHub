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
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useForm } from 'react-hook-form'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { ModelRatioForm } from '../model-ratio-form'
import { RatioSettingsCard } from '../ratio-settings-card'

const savedValues = {
  ModelPrice: '{"free-model":0,"removed-model":1}',
  ModelRatio: '{}',
  CacheRatio: '{}',
  CreateCacheRatio: '{}',
  CompletionRatio: '{}',
  ImageRatio: '{}',
  AudioRatio: '{}',
  AudioCompletionRatio: '{}',
  BillingMode: '{"dynamic-model":"tiered_expr"}',
  BillingExpr: '{"dynamic-model":"p * 2","inactive-model":"p * 4"}',
  ExposeRatioEnabled: false,
}
const draftValues = {
  ...savedValues,
  ModelPrice: '{"free-model":0,"added-model":2}',
  BillingExpr: '{"dynamic-model":"p * 3","inactive-model":"p * 5"}',
}
let client: QueryClient

afterEach(() => {
  client?.clear()
  localStorage.clear()
  vi.restoreAllMocks()
})

function Harness(props: {
  onSave: (values: typeof savedValues) => Promise<void>
  saved?: typeof savedValues
  draft?: typeof savedValues
}) {
  const form = useForm({ defaultValues: props.draft ?? draftValues })
  return (
    <ModelRatioForm
      form={form}
      savedValues={props.saved ?? savedValues}
      onSave={props.onSave}
      onReset={vi.fn()}
      isSaving={false}
      isResetting={false}
    />
  )
}

function renderForm(
  onSave = vi.fn().mockResolvedValue(undefined),
  saved = savedValues,
  draft = draftValues
) {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: [], vendors: [] },
  })
  render(
    <QueryClientProvider client={client}>
      <Harness onSave={onSave} saved={saved} draft={draft} />
    </QueryClientProvider>
  )
  return { user: userEvent.setup(), onSave }
}

test('reviews deleted, added and expression price changes before saving, preserving explicit zero prices', async () => {
  const { user, onSave } = renderForm()
  await user.click(screen.getByRole('button', { name: 'Switch to JSON' }))
  await user.click(screen.getByRole('button', { name: 'Save model prices' }))
  const dialog = await screen.findByRole('alertdialog', {
    name: 'Review price changes',
  })
  expect(onSave).not.toHaveBeenCalled()
  expect(within(dialog).getByText('removed-model')).toBeVisible()
  expect(within(dialog).getByText('added-model')).toBeVisible()
  expect(within(dialog).getByText('p * 2')).toBeVisible()
  expect(within(dialog).getByText('p * 3')).toBeVisible()
  expect(within(dialog).getByText('p * 4')).toBeVisible()
  expect(within(dialog).getByText('p * 5')).toBeVisible()
  expect(within(dialog).queryByText('free-model')).not.toBeInTheDocument()
  await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
  expect(onSave).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Save model prices' }))
  await user.click(
    within(await screen.findByRole('alertdialog')).getByRole('button', {
      name: 'Save model prices',
    })
  )
  await waitFor(() =>
    expect(onSave).toHaveBeenCalledExactlyOnceWith(draftValues)
  )
})

test('retains a failed save for review and blocks duplicate confirmation while pending', async () => {
  let reject!: (error: Error) => void
  const onSave = vi.fn(
    () =>
      new Promise<void>((_resolve, fail) => {
        reject = fail
      })
  )
  const { user } = renderForm(onSave)
  await user.click(screen.getByRole('button', { name: 'Switch to JSON' }))
  await user.click(screen.getByRole('button', { name: 'Save model prices' }))
  const dialog = await screen.findByRole('alertdialog', {
    name: 'Review price changes',
  })
  const confirm = within(dialog).getByRole('button', {
    name: 'Save model prices',
  })
  await user.click(confirm)
  expect(confirm).toBeDisabled()
  expect(within(dialog).getByRole('button', { name: 'Cancel' })).toBeDisabled()
  await user.click(confirm)
  expect(onSave).toHaveBeenCalledTimes(1)
  await act(() => reject(new Error('offline')))
  expect(dialog).toBeVisible()
  expect(confirm).toBeEnabled()
})

test('keeps deletion drafts visible and lets administrators undo without writing to the server', async () => {
  const { user, onSave } = renderForm()
  const row = screen.getByRole('row', { name: /removed-model/ })
  expect(within(row).getByText('Deleted')).toBeVisible()
  await user.click(within(row).getByRole('button', { name: 'Undo' }))
  expect(within(row).queryByText('Deleted')).not.toBeInTheDocument()
  expect(onSave).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Switch to JSON' }))
  await user.click(screen.getByRole('button', { name: 'Save model prices' }))
  const dialog = await screen.findByRole('alertdialog')
  expect(within(dialog).queryByText('removed-model')).not.toBeInTheDocument()
})

test('reviews raw billing mode changes even when the effective price is unchanged', async () => {
  const saved = { ...savedValues, BillingMode: '{}' }
  const draft = { ...saved, BillingMode: '{"free-model":"per-token"}' }
  const { user, onSave } = renderForm(undefined, saved, draft)
  await user.click(screen.getByRole('button', { name: 'Switch to JSON' }))
  await user.click(screen.getByRole('button', { name: 'Save model prices' }))
  const dialog = await screen.findByRole('alertdialog', {
    name: 'Review price changes',
  })
  expect(within(dialog).getByText('free-model')).toBeVisible()
  expect(within(dialog).getByText('per-token')).toBeVisible()
  expect(onSave).not.toHaveBeenCalled()
  await user.click(
    within(dialog).getByRole('button', { name: 'Save model prices' })
  )
  await waitFor(() => expect(onSave).toHaveBeenCalledExactlyOnceWith(draft))
})

test('allows repairing corrupt stored expression JSON only after displaying the full replacement', async () => {
  const saved = { ...savedValues, BillingExpr: '{broken' }
  const { user, onSave } = renderForm(undefined, saved, draftValues)
  await user.click(screen.getByRole('button', { name: 'Switch to JSON' }))
  await user.click(screen.getByRole('button', { name: 'Save model prices' }))
  const dialog = await screen.findByRole('alertdialog', {
    name: 'Review price changes',
  })
  expect(within(dialog).getByText('{broken')).toBeVisible()
  expect(within(dialog).getByText(draftValues.BillingExpr)).toBeVisible()
  expect(
    within(dialog).getByText(
      'The saved configuration is invalid. Review the full replacement before saving.'
    )
  ).toBeVisible()
  expect(onSave).not.toHaveBeenCalled()
  await user.click(
    within(dialog).getByRole('button', { name: 'Save model prices' })
  )
  await waitFor(() =>
    expect(onSave).toHaveBeenCalledExactlyOnceWith(draftValues)
  )
})

test('deletes a newly added draft without deleting saved prices or contacting the server', async () => {
  const { user, onSave } = renderForm()
  const row = screen.getByRole('row', { name: /added-model/ })
  await user.click(within(row).getByRole('button', { name: 'Open menu' }))
  await user.click(screen.getByRole('menuitem', { name: 'Delete' }))
  expect(
    screen.queryByRole('row', { name: /added-model/ })
  ).not.toBeInTheDocument()
  expect(screen.getByRole('row', { name: /free-model/ })).toBeVisible()
  expect(onSave).not.toHaveBeenCalled()
})

test('a business save failure stops later option writes and keeps the reviewed prices open', async () => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: [], vendors: [] },
  })
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: false, message: 'not saved' } })
  render(
    <QueryClientProvider client={client}>
      <RatioSettingsCard
        modelDefaults={savedValues}
        groupDefaults={{
          GroupRatio: '{}',
          TopupGroupRatio: '{}',
          UserUsableGroups: '{}',
          GroupGroupRatio: '{}',
          AutoGroups: '[]',
          MaxTokenAutoGroups: 1,
          DefaultUseAutoGroup: false,
          GroupSpecialUsableGroup: '{}',
        }}
        toolPricesDefault='{}'
        visibleTabs={['models']}
      />
    </QueryClientProvider>
  )
  const user = userEvent.setup()
  await user.click(
    within(screen.getByRole('row', { name: /free-model/ })).getByRole(
      'button',
      { name: 'Edit' }
    )
  )
  const price = screen.getByPlaceholderText('0.01')
  await user.clear(price)
  await user.type(price, '2')
  await user.click(screen.getByRole('switch', { name: 'Expose ratio API' }))
  await user.click(screen.getByRole('button', { name: 'Save model prices' }))
  const dialog = await screen.findByRole('alertdialog', {
    name: 'Review price changes',
  })
  await user.click(
    within(dialog).getByRole('button', { name: 'Save model prices' })
  )
  await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
  const request = put.mock.calls[0]?.[1] as { key: string; value: string }
  expect(request.key).toBe('ModelPrice')
  expect(JSON.parse(request.value)).toEqual({
    'free-model': 2,
    'removed-model': 1,
  })
  await waitFor(() =>
    expect(
      within(dialog).getByRole('button', { name: 'Save model prices' })
    ).toBeEnabled()
  )
  expect(dialog).toBeVisible()
  expect(put).toHaveBeenCalledTimes(1)
})
