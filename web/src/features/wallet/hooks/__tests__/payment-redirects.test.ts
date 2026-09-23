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
import { expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { useCreemPayment } from '../use-creem-payment'
import { usePayment } from '../use-payment'
import { useWaffoPancakePayment } from '../use-waffo-pancake-payment'
import { useWaffoPayment } from '../use-waffo-payment'

// Exercise each provider's real response parsing and navigation boundary.
function usePaymentRedirects() {
  const stripe = usePayment()
  const creem = useCreemPayment()
  const waffo = useWaffoPayment()
  const pancake = useWaffoPancakePayment()
  return {
    stripe: () => stripe.processPayment(100, 'stripe'),
    creem: () => creem.processCreemPayment('product-test'),
    waffo: () => waffo.processWaffoPayment(100, 0),
    pancake: () => pancake.processWaffoPancakePayment(100),
  }
}

test.each(['stripe', 'creem', 'waffo', 'pancake'] as const)(
  '%s rejects a script URL returned by the payment provider without navigating',
  async (provider) => {
    const url = 'javascript:alert(document.domain)'
    vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        message: 'success',
        data: { pay_link: url, checkout_url: url, payment_url: url },
      },
    })
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    const notify = vi.spyOn(toast, 'error')
    const success = vi.spyOn(toast, 'success')
    const { result } = renderHook(usePaymentRedirects)

    await act(async () => {
      expect(await result.current[provider]()).toBe(false)
    })

    expect(open).not.toHaveBeenCalled()
    expect(success).not.toHaveBeenCalled()
    expect(notify).toHaveBeenCalledWith('Payment request failed')
  }
)

test.each(['stripe', 'creem', 'waffo'] as const)(
  '%s opens a valid checkout without an opener and accepts a null window handle',
  async (provider) => {
    const url = 'https://pay.example.com/checkout?session=payment-test'
    vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        message: 'success',
        data: { pay_link: url, checkout_url: url, payment_url: url },
      },
    })
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    const { result } = renderHook(usePaymentRedirects)

    await act(async () => {
      expect(await result.current[provider]()).toBe(true)
    })

    expect(open).toHaveBeenCalledWith(url, '_blank', 'noopener,noreferrer')
  }
)

test('Pancake keeps its same-tab checkout navigation after URL validation', async () => {
  const originalUrl = window.location.href
  const checkoutUrl = new URL('#wallet-checkout', originalUrl).href
  vi.spyOn(api, 'post').mockResolvedValue({
    data: { message: 'success', data: { checkout_url: checkoutUrl } },
  })
  const open = vi.spyOn(window, 'open').mockReturnValue(null)
  const { result } = renderHook(usePaymentRedirects)

  try {
    await act(async () => {
      expect(await result.current.pancake()).toBe(true)
    })

    expect(window.location.href).toBe(checkoutUrl)
    expect(open).not.toHaveBeenCalled()
  } finally {
    window.history.replaceState(null, '', originalUrl)
  }
})
