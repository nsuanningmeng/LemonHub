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
import { act, renderHook } from '@testing-library/react'
import { toast } from 'sonner'
import { describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { requestPaymentAmount, usePayment } from '../use-payment'

describe('payment quotes', () => {
  test.each([
    ['alipay', '/api/user/amount'],
    ['stripe', '/api/user/stripe/amount'],
    ['waffo', '/api/user/waffo/amount'],
    ['waffo_pancake', '/api/user/waffo-pancake/amount'],
  ])('uses the backend quote for %s', async (method, endpoint) => {
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      data: { message: 'success', data: '7300.00' },
    })

    await expect(requestPaymentAmount(1000, method)).resolves.toBe(7300)
    expect(post).toHaveBeenCalledWith(
      endpoint,
      { amount: 1000 },
      expect.any(Object)
    )
  })

  test('rejects a failed quote with the backend reason instead of returning zero', async () => {
    vi.spyOn(api, 'post').mockResolvedValue({
      data: { message: 'error', data: 'top-up quota limit exceeded' },
    })

    await expect(requestPaymentAmount(1000, 'alipay')).rejects.toThrow(
      'This top-up exceeds the wallet balance limit. Reduce the amount or contact the administrator.'
    )
  })

  test.each(['0', '-1', 'NaN', 'Infinity', '7300invalid'])(
    'rejects an invalid successful quote of %s',
    async (data) => {
      vi.spyOn(api, 'post').mockResolvedValue({
        data: { message: 'success', data },
      })

      await expect(requestPaymentAmount(1000, 'alipay')).rejects.toThrow(
        'Payment request failed'
      )
    }
  )

  test('clears a failed quote when retrying and exposes the recovered amount', async () => {
    vi.spyOn(api, 'post')
      .mockResolvedValueOnce({
        data: { message: 'error', data: 'top-up quota limit exceeded' },
      })
      .mockResolvedValueOnce({ data: { message: 'success', data: '730.00' } })
    const { result } = renderHook(() => usePayment())

    await act(async () => {
      await result.current.calculatePaymentAmount(1000, 'alipay')
    })
    expect(result.current.error).toBe(
      'This top-up exceeds the wallet balance limit. Reduce the amount or contact the administrator.'
    )
    expect(result.current.calculating).toBe(false)

    await act(async () => {
      await result.current.calculatePaymentAmount(100, 'alipay')
    })
    expect(result.current.error).toBeNull()
    expect(result.current.amount).toBe(730)
  })

  test('an older failed request cannot overwrite the latest successful quote', async () => {
    type QuoteResponse = {
      data: { message: string; data: string }
    }
    let resolvePrevious!: (response: QuoteResponse) => void
    const previous = new Promise<QuoteResponse>((resolve) => {
      resolvePrevious = resolve
    })
    vi.spyOn(api, 'post')
      .mockReturnValueOnce(previous)
      .mockResolvedValueOnce({ data: { message: 'success', data: '730.00' } })
    const { result } = renderHook(() => usePayment())
    let previousCalculation: Promise<unknown>

    act(() => {
      previousCalculation = result.current.calculatePaymentAmount(
        1000,
        'alipay'
      )
    })
    await act(async () => {
      await result.current.calculatePaymentAmount(100, 'alipay')
    })
    await act(async () => {
      resolvePrevious({
        data: { message: 'error', data: 'top-up quota limit exceeded' },
      })
      await previousCalculation
    })

    expect(result.current.amount).toBe(730)
    expect(result.current.error).toBeNull()
    expect(result.current.calculating).toBe(false)
  })
})

describe('payment submission', () => {
  test('shows the actual backend error when order creation fails', async () => {
    vi.spyOn(api, 'post').mockResolvedValue({
      data: { message: 'error', data: '创建订单失败' },
    })
    const notify = vi.spyOn(toast, 'error')
    const { result } = renderHook(() => usePayment())

    await act(async () => {
      expect(await result.current.processPayment(1000, 'alipay')).toBe(false)
    })

    expect(notify).toHaveBeenCalledWith('创建订单失败')
    expect(result.current.processing).toBe(false)
  })

  test('submits the signed gateway form after a successful order', async () => {
    vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        message: 'success',
        data: { money: '7300.00', type: 'alipay', sign: 'test-signature' },
        url: 'https://pay.example.com/submit.php',
      },
    })
    const submit = vi
      .spyOn(HTMLFormElement.prototype, 'submit')
      .mockImplementation(function (this: HTMLFormElement) {
        expect(this.action).toBe('https://pay.example.com/submit.php')
        expect(this.method).toBe('post')
        expect(Object.fromEntries(new FormData(this))).toEqual({
          money: '7300.00',
          type: 'alipay',
          sign: 'test-signature',
        })
      })
    const { result } = renderHook(() => usePayment())

    await act(async () => {
      expect(await result.current.processPayment(1000, 'alipay')).toBe(true)
    })

    expect(submit).toHaveBeenCalledOnce()
  })
})
