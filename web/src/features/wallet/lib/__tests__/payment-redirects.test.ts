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
import { expect, test, vi } from 'vitest'

import { getSafePaymentRedirectUrl, submitPaymentForm } from '../payment'

test.each([
  'javascript:alert(1)',
  'data:text/html,payment',
  '//pay.example.com/checkout',
  '/checkout',
  'https://',
  'https:pay.example.com',
  'https://trusted.example@untrusted.example/checkout',
  '',
  null,
])('rejects an unsafe hosted checkout URL %s', (url) => {
  expect(getSafePaymentRedirectUrl(url)).toBeNull()
})

test.each([
  'https://pay.example.com/checkout?token=signed%2Bvalue',
  'http://pay.example.com/checkout?token=signed%2Bvalue',
])('preserves the supported HTTP(S) checkout URL %s exactly', (url) => {
  expect(getSafePaymentRedirectUrl(url)).toBe(url)
})

test.each([
  'javascript:alert(1)',
  'data:text/html,payment',
  'http://pay.example.com/submit.php',
  '//pay.example.com/submit.php',
  '/submit.php',
])('refuses an unsafe Epay form URL %s before submitting', (url) => {
  const submit = vi
    .spyOn(HTMLFormElement.prototype, 'submit')
    .mockImplementation(() => undefined)

  expect(() => submitPaymentForm(url, { sign: 'test-signature' })).toThrow(
    'Payment request failed'
  )
  expect(submit).not.toHaveBeenCalled()
  expect(document.querySelector('form')).toBeNull()
})

test('submits the HTTPS gateway form with its signed parameters unchanged', () => {
  const params = {
    sign: 'signed+value/with=padding',
    money: '30000.00',
    name: '<img src=x onerror=alert(1)> & "quoted"',
  }
  const submit = vi
    .spyOn(HTMLFormElement.prototype, 'submit')
    .mockImplementation(function (this: HTMLFormElement) {
      expect(this.action).toBe('https://pay.example.com/submit.php')
      expect(this.method).toBe('post')
      expect(this.getAttribute('rel')).toBe('noopener noreferrer')
      expect(Object.fromEntries(new FormData(this))).toEqual(params)
      expect(this.querySelector('img')).toBeNull()
    })

  submitPaymentForm('https://pay.example.com/submit.php', params)

  expect(submit).toHaveBeenCalledOnce()
  expect(document.querySelector('form')).toBeNull()
})
