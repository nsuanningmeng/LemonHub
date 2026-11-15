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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { toast } from 'sonner'
import { beforeAll, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import type { UpdateOptionRequest } from '../../types'
import { ToolPriceSettings } from '../tool-price-settings'

describe('tool price validation', () => {
  beforeAll(() => {
    i18next.addResourceBundle('en', 'translation', {
      'Price ($/1K calls)': 'Price ($/1K calls)',
      'Please enter a valid number': 'Please enter a valid number',
      'Tool identifier': 'Tool identifier',
    })
  })

  test('blocks an empty price without converting it to an explicit zero', () => {
    const queryClient = new QueryClient({
      defaultOptions: { mutations: { retry: false } },
    })

    render(
      <QueryClientProvider client={queryClient}>
        <ToolPriceSettings defaultValue='{"web_search":10}' />
      </QueryClientProvider>
    )

    const priceInput = screen.getByRole('spinbutton', {
      name: 'Price ($/1K calls): web_search',
    })
    const saveButton = screen.getByRole('button', { name: 'Save tool prices' })

    fireEvent.change(priceInput, { target: { value: '' } })

    expect(priceInput).toHaveAttribute('aria-invalid', 'true')
    expect(screen.getByText('Please enter a valid number')).toBeInTheDocument()
    expect(saveButton).toBeDisabled()

    fireEvent.change(priceInput, { target: { value: '0' } })

    expect(priceInput).toHaveAttribute('aria-invalid', 'false')
    expect(saveButton).toBeEnabled()

    queryClient.clear()
  })

  test.each([0.0001, 0.125, 1.2345])(
    'accepts and saves a tool price of %s without rounding',
    async (price) => {
      const queryClient = new QueryClient({
        defaultOptions: { mutations: { retry: false } },
      })
      const put = vi.spyOn(api, 'put').mockResolvedValue({
        data: { success: true },
      })
      const view = render(
        <QueryClientProvider client={queryClient}>
          <ToolPriceSettings defaultValue='{"web_search":10}' />
        </QueryClientProvider>
      )
      const priceInput = screen.getByRole('spinbutton', {
        name: 'Price ($/1K calls): web_search',
      })

      fireEvent.change(priceInput, { target: { value: String(price) } })

      expect(priceInput).toBeValid()
      fireEvent.click(screen.getByRole('button', { name: 'Save tool prices' }))
      await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
      const savedOption = put.mock.calls[0][1] as UpdateOptionRequest
      const savedPrices = JSON.parse(String(savedOption.value))
      expect(savedPrices.web_search).toBe(price)
      view.unmount()
      queryClient.clear()
    }
  )
  test('shows the DeepSeek Search and Fetch prices for a config saved before they existed', () => {
    const queryClient = new QueryClient()

    render(
      <QueryClientProvider client={queryClient}>
        <ToolPriceSettings defaultValue='{"web_search":10}' />
      </QueryClientProvider>
    )

    for (const key of ['deepseek_search', 'deepseek_fetch']) {
      expect(
        screen.getByRole('spinbutton', { name: `Price ($/1K calls): ${key}` })
      ).toHaveValue(0.75)
    }

    queryClient.clear()
  })

  test('a rejected tool price save retains the editable price, reports one error, and can be retried', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { mutations: { retry: false } },
    })
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValueOnce({
        data: { success: false, message: 'Tool price rejected' },
      })
      .mockResolvedValue({ data: { success: true } })
    const error = vi.spyOn(toast, 'error')
    const user = userEvent.setup()
    const view = render(
      <QueryClientProvider client={queryClient}>
        <ToolPriceSettings defaultValue='{"web_search":10}' />
      </QueryClientProvider>
    )
    const input = screen.getByRole('spinbutton', {
      name: 'Price ($/1K calls): web_search',
    })
    const save = screen.getByRole('button', { name: 'Save tool prices' })
    await user.clear(input)
    await user.type(input, '12.5')
    await user.click(save)

    await waitFor(() =>
      expect(error).toHaveBeenCalledExactlyOnceWith('Tool price rejected')
    )
    await waitFor(() => expect(save).toBeEnabled())
    expect(input).toHaveValue(12.5)
    expect(input).toBeEnabled()

    await user.click(save)
    await waitFor(() => expect(put).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(save).toBeEnabled())
    const request = put.mock.calls[1][1] as UpdateOptionRequest
    expect(request.key).toBe('tool_price_setting.prices')
    expect(JSON.parse(String(request.value)).web_search).toBe(12.5)
    expect(error).toHaveBeenCalledTimes(1)
    view.unmount()
    queryClient.clear()
  })
})
