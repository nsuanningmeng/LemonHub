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
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { EmailBindDialog } from '../email-bind-dialog'

const validation = {
  lot_number: 'email-lot-1',
  captcha_output: 'output+with/slash=',
  pass_token: 'email-pass-token',
  gen_time: '1790294400',
}
let queryClient: QueryClient
let challengeCount: number
const onSuccess = vi.fn()

function EmailBindFixture() {
  const [open, setOpen] = useState(true)
  return (
    <QueryClientProvider client={queryClient}>
      <button type='button' onClick={() => setOpen(true)}>
        Change email
      </button>
      <EmailBindDialog
        open={open}
        onOpenChange={setOpen}
        currentEmail='old@example.com'
        onSuccess={onSuccess}
      />
    </QueryClientProvider>
  )
}

async function openEmailDialog() {
  const user = userEvent.setup()
  render(<EmailBindFixture />)
  await user.type(screen.getByLabelText('Email Address'), 'new+tag@example.com')
  return user
}

beforeEach(() => {
  localStorage.clear()
  challengeCount = 0
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  queryClient.setQueryData(['status'], {
    turnstile_check: true,
    captcha_provider: 'geetest',
    geetest_captcha_id: 'email-captcha-id',
  })
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true } })
  vi.spyOn(api, 'post').mockResolvedValue({ data: { success: true } })

  // Only replace the remote SDK; exercise the real dialog, widget, hook and API.
  window.initGeetest4 = (_config, handler) => {
    const token = { ...validation, lot_number: `email-lot-${++challengeCount}` }
    let onVerify = () => {}
    const challenge = document.createElement('section')
    challenge.setAttribute('aria-label', 'GeeTest challenge')
    challenge.style.position = 'absolute'
    const complete = document.createElement('button')
    complete.type = 'button'
    complete.textContent = 'Complete GeeTest verification'
    complete.addEventListener('click', () => onVerify())
    challenge.append(complete)
    handler({
      appendTo: (selector) =>
        document.querySelector(selector)?.append(challenge),
      onSuccess: (callback) => {
        onVerify = callback
      },
      onError: () => {},
      getValidate: () => token,
      destroy: () => challenge.remove(),
    })
  }
})

afterEach(() => {
  cleanup()
  queryClient.clear()
  delete window.initGeetest4
  delete window.turnstile
  localStorage.clear()
})

describe('email binding human verification', () => {
  test('shows verification and blocks sending until the challenge succeeds', async () => {
    const user = await openEmailDialog()

    expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled()
    await user.click(
      await screen.findByRole('button', {
        name: 'Complete GeeTest verification',
      })
    )
    expect(screen.getByRole('button', { name: 'Send' })).toBeEnabled()
    expect(api.get).not.toHaveBeenCalled()
  })

  test('sends the complete token and binds the email without requiring another challenge', async () => {
    const user = await openEmailDialog()
    await user.click(
      await screen.findByRole('button', {
        name: 'Complete GeeTest verification',
      })
    )
    const oldChallenge = screen.getByRole('region', {
      name: 'GeeTest challenge',
    })
    await user.click(screen.getByRole('button', { name: 'Send' }))

    expect(api.get).toHaveBeenCalledTimes(1)
    const request = new URL(
      vi.mocked(api.get).mock.calls[0][0],
      'https://example.com'
    )
    expect(request.pathname).toBe('/api/verification')
    expect(request.searchParams.get('email')).toBe('new+tag@example.com')
    expect(request.searchParams.get('turnstile')).toBe(
      JSON.stringify(validation)
    )
    await waitFor(() => expect(oldChallenge).not.toBeInTheDocument())
    expect(screen.getByRole('button', { name: /^\d+s$/ })).toBeDisabled()

    await user.type(screen.getByLabelText('Verification Code'), '123456')
    await user.click(screen.getByRole('button', { name: 'Bind Email' }))
    expect(api.post).toHaveBeenCalledWith('/api/oauth/email/bind', {
      email: 'new+tag@example.com',
      code: '123456',
    })
    await waitFor(() => expect(onSuccess).toHaveBeenCalledTimes(1))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  test.each(['business rejection', 'network failure'])(
    'requires a fresh challenge before retrying after %s',
    async (failure) => {
      if (failure === 'business rejection') {
        vi.mocked(api.get).mockResolvedValueOnce({
          data: { success: false, message: 'Verification rejected' },
        })
      } else {
        vi.mocked(api.get).mockRejectedValueOnce(
          new Error('Network unavailable')
        )
      }
      const user = await openEmailDialog()
      await user.click(
        await screen.findByRole('button', {
          name: 'Complete GeeTest verification',
        })
      )
      const oldChallenge = screen.getByRole('region', {
        name: 'GeeTest challenge',
      })
      await user.click(screen.getByRole('button', { name: 'Send' }))

      await waitFor(() => expect(oldChallenge).not.toBeInTheDocument())
      expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled()
      expect(screen.getByLabelText('Email Address')).toHaveValue(
        'new+tag@example.com'
      )
      await user.click(screen.getByRole('button', { name: 'Send' }))
      expect(api.get).toHaveBeenCalledTimes(1)

      await user.click(
        await screen.findByRole('button', {
          name: 'Complete GeeTest verification',
        })
      )
      await user.click(screen.getByRole('button', { name: 'Send' }))
      expect(api.get).toHaveBeenCalledTimes(2)
      const retry = new URL(
        vi.mocked(api.get).mock.calls[1][0],
        'https://example.com'
      )
      expect(retry.searchParams.get('turnstile')).toBe(
        JSON.stringify({ ...validation, lot_number: 'email-lot-2' })
      )
    }
  )

  test('blocks sending when a completed Turnstile challenge expires', async () => {
    queryClient.setQueryData(['status'], {
      turnstile_check: true,
      captcha_provider: 'turnstile',
      turnstile_site_key: 'email-site-key',
    })
    window.turnstile = {
      render: (element, options) => {
        const complete = document.createElement('button')
        complete.type = 'button'
        complete.textContent = 'Complete Turnstile verification'
        complete.addEventListener('click', () => {
          const callback = options.callback as (token: string) => void
          callback('turnstile-email-token')
        })
        const expire = document.createElement('button')
        expire.type = 'button'
        expire.textContent = 'Expire Turnstile verification'
        expire.addEventListener('click', () => {
          const callback = options['expired-callback'] as () => void
          callback()
        })
        element.append(complete, expire)
      },
    }
    const user = await openEmailDialog()
    await user.click(
      await screen.findByRole('button', {
        name: 'Complete Turnstile verification',
      })
    )
    expect(screen.getByRole('button', { name: 'Send' })).toBeEnabled()
    await user.click(
      screen.getByRole('button', { name: 'Expire Turnstile verification' })
    )
    expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled()
    expect(api.get).not.toHaveBeenCalled()
  })

  test('closing and reopening discards the previous verification token', async () => {
    const user = await openEmailDialog()
    await user.click(
      await screen.findByRole('button', {
        name: 'Complete GeeTest verification',
      })
    )
    await user.keyboard('{Escape}')
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    await user.click(screen.getByRole('button', { name: 'Change email' }))
    await user.type(
      screen.getByLabelText('Email Address'),
      'new+tag@example.com'
    )

    expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled()
    await user.click(
      await screen.findByRole('button', {
        name: 'Complete GeeTest verification',
      })
    )
    expect(screen.getByRole('button', { name: 'Send' })).toBeEnabled()
    expect(api.get).not.toHaveBeenCalled()
  })

  test('prevents closing or binding while a verification email request is pending', async () => {
    let resolveRequest!: (result: { data: { success: boolean } }) => void
    const pendingRequest = new Promise<{ data: { success: boolean } }>(
      (resolve) => {
        resolveRequest = resolve
      }
    )
    vi.mocked(api.get).mockReturnValueOnce(pendingRequest)
    const user = await openEmailDialog()
    await user.type(screen.getByLabelText('Verification Code'), '123456')
    await user.click(
      await screen.findByRole('button', {
        name: 'Complete GeeTest verification',
      })
    )
    await user.click(screen.getByRole('button', { name: 'Send' }))

    expect(screen.getByRole('button', { name: 'Sending...' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Bind Email' })).toBeDisabled()
    await user.keyboard('{Escape}')
    expect(
      screen.getByRole('dialog', { name: 'Bind Email' })
    ).toBeInTheDocument()
    expect(api.post).not.toHaveBeenCalled()

    await act(async () => resolveRequest({ data: { success: true } }))
    expect(await screen.findByRole('button', { name: /^\d+s$/ })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Bind Email' })).toBeEnabled()
  })

  test('sends without a captcha token when human verification is disabled', async () => {
    queryClient.setQueryData(['status'], { turnstile_check: false })
    const user = await openEmailDialog()

    expect(
      screen.queryByRole('region', { name: 'GeeTest challenge' })
    ).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Send' }))
    expect(api.get).toHaveBeenCalledWith(
      '/api/verification?email=new%2Btag%40example.com'
    )
  })

  test('keeps the GeeTest floating challenge outside dialog clipping containers', async () => {
    await openEmailDialog()
    const dialog = screen.getByRole('dialog', { name: 'Bind Email' })
    const challenge = await within(dialog).findByRole('region', {
      name: 'GeeTest challenge',
    })

    for (
      let ancestor = challenge.parentElement;
      ancestor && dialog.contains(ancestor);
      ancestor = ancestor.parentElement
    ) {
      expect(
        [...ancestor.classList].filter((name) =>
          /^overflow(?:-[xy])?-(?:hidden|clip|auto|scroll)$/.test(name)
        )
      ).toEqual([])
    }
  })
})
