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
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { ModelRatioVisualEditor } from '../model-ratio-visual-editor'

let client: QueryClient | undefined

afterEach(() => {
  client?.clear()
  localStorage.clear()
  vi.restoreAllMocks()
})

async function renderEditor() {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: [], vendors: [] },
  })
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <ModelRatioVisualEditor
        savedModelPrice='{}'
        savedModelRatio='{}'
        savedCacheRatio='{}'
        savedCreateCacheRatio='{}'
        savedCompletionRatio='{}'
        savedImageRatio='{}'
        savedAudioRatio='{}'
        savedAudioCompletionRatio='{}'
        savedBillingMode='{}'
        savedBillingExpr='{}'
        modelPrice='{}'
        modelRatio='{}'
        cacheRatio='{}'
        createCacheRatio='{}'
        completionRatio='{}'
        imageRatio='{}'
        audioRatio='{}'
        audioCompletionRatio='{}'
        billingMode='{}'
        billingExpr='{}'
        onChange={vi.fn()}
        onSave={vi.fn()}
        isSaving={false}
      />
    </QueryClientProvider>
  )
  await waitFor(() => expect(client?.isFetching()).toBe(0))
}

it('keeps the pricing table full width until there is room for the desktop editor', async () => {
  await renderEditor()

  const grid = screen.getByText('Select a model to edit pricing').parentElement
    ?.parentElement?.parentElement
  expect(grid).toHaveClass(
    'min-w-0',
    'xl:grid-cols-[minmax(0,0.68fr)_minmax(0,1.32fr)]'
  )
  expect(grid).not.toHaveClass(
    'md:grid-cols-[minmax(0,0.72fr)_minmax(0,1.28fr)]'
  )
  expect(
    screen.getByText('Select a model to edit pricing').parentElement
      ?.parentElement
  ).toHaveClass('hidden', 'xl:block')
})

it('opens the pricing editor in a sheet on tablets so the single-column table remains usable', async () => {
  const matchMedia = window.matchMedia
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
    ...matchMedia(query),
    matches: query === '(max-width: 1279px)',
  }))
  const user = userEvent.setup()
  await renderEditor()

  await user.click(screen.getAllByRole('button', { name: 'Add model' })[0])
  expect(
    await screen.findByRole('dialog', { name: 'Add model pricing' })
  ).toBeVisible()
})
