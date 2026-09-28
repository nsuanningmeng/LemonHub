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
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { CheckinCalendarCard } from '../checkin-calendar-card'

const validation = {
  lot_number: 'checkin-lot',
  captcha_output: 'checkin-output',
  pass_token: 'checkin-pass-token',
  gen_time: '1790294400',
}
const originalSystemConfig = useSystemConfigStore.getState()
let queryClient: QueryClient
let checkedIn: boolean

beforeEach(() => {
  localStorage.clear()
  checkedIn = false
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  queryClient.setQueryData(['status'], {
    turnstile_check: true,
    captcha_provider: 'geetest',
    geetest_captcha_id: 'checkin-captcha-id',
  })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (!url.startsWith('/api/user/checkin?month=')) {
      throw new Error(`Unexpected GET ${url}`)
    }
    return {
      data: {
        success: true,
        data: {
          enabled: true,
          stats: {
            checked_in_today: checkedIn,
            total_checkins: checkedIn ? 1 : 0,
            total_quota: checkedIn ? 500 : 0,
            checkin_count: checkedIn ? 1 : 0,
            records: [],
          },
        },
      },
    }
  })
  vi.spyOn(api, 'post').mockImplementation(async (url) => {
    const token = new URL(url, 'https://gateway.example.com').searchParams.get(
      'turnstile'
    )
    if (!token) {
      return { data: { success: false, message: '需要人机验证' } }
    }
    checkedIn = true
    return { data: { success: true, data: { quota_awarded: 500 } } }
  })

  // Replace only the remote SDK: the application widget, dialog, and check-in
  // request handling remain real. The challenge floats outside normal flow.
  window.initGeetest4 = (_config, handler) => {
    let onSuccess = () => {}
    const widget = document.createElement('div')
    const trigger = document.createElement('button')
    trigger.type = 'button'
    trigger.textContent = 'Open GeeTest challenge'
    const challenge = document.createElement('section')
    challenge.setAttribute('aria-label', 'GeeTest challenge')
    challenge.style.position = 'absolute'
    challenge.hidden = true
    const complete = document.createElement('button')
    complete.type = 'button'
    complete.textContent = 'Complete GeeTest verification'
    complete.addEventListener('click', () => onSuccess())
    challenge.append(complete)
    trigger.addEventListener('click', () => {
      challenge.hidden = false
    })
    widget.append(trigger, challenge)
    handler({
      appendTo: (selector) => document.querySelector(selector)?.append(widget),
      onSuccess: (callback) => {
        onSuccess = callback
      },
      onError: () => {},
      getValidate: () => validation,
      destroy: () => widget.remove(),
    })
  }
})

afterEach(() => {
  cleanup()
  queryClient.clear()
  delete window.initGeetest4
  useSystemConfigStore.setState(originalSystemConfig, true)
  localStorage.clear()
})

async function openChallenge() {
  const user = userEvent.setup()
  render(
    <QueryClientProvider client={queryClient}>
      <CheckinCalendarCard checkinEnabled captchaEnabled />
    </QueryClientProvider>
  )
  await user.click(await screen.findByRole('button', { name: 'Check in now' }))
  const dialog = await screen.findByRole('dialog', { name: 'Security Check' })
  await user.click(
    within(dialog).getByRole('button', { name: 'Open GeeTest challenge' })
  )
  return { user, dialog }
}

describe('check-in captcha dialog', () => {
  test('does not clip the floating challenge at any dialog ancestor', async () => {
    const { dialog } = await openChallenge()
    const challenge = within(dialog).getByRole('region', {
      name: 'GeeTest challenge',
    })

    for (
      let ancestor = challenge.parentElement;
      ancestor && dialog.contains(ancestor);
      ancestor = ancestor.parentElement
    ) {
      const clippingClasses = [...ancestor.classList].filter((name) =>
        /^overflow(?:-[xy])?-(?:hidden|clip|auto|scroll)$/.test(name)
      )
      expect(clippingClasses).toEqual([])
      if (ancestor !== dialog) {
        expect(
          [...ancestor.classList].filter((name) => name.startsWith('max-h-'))
        ).toEqual([])
      }
    }
  })

  test('submits successful verification and closes the dialog after check-in succeeds', async () => {
    const { user, dialog } = await openChallenge()
    await user.click(
      within(dialog).getByRole('button', {
        name: 'Complete GeeTest verification',
      })
    )

    expect(api.post).toHaveBeenLastCalledWith(
      `/api/user/checkin?turnstile=${encodeURIComponent(JSON.stringify(validation))}`
    )
    await waitFor(() =>
      expect(
        screen.queryByRole('dialog', { name: 'Security Check' })
      ).not.toBeInTheDocument()
    )
    expect(
      await screen.findByRole('button', { name: 'Checked in' })
    ).toBeDisabled()
  })

  test('closing with Escape lets a later check-in open a fresh challenge', async () => {
    const { user, dialog } = await openChallenge()
    const oldChallenge = within(dialog).getByRole('region', {
      name: 'GeeTest challenge',
    })
    await user.keyboard('{Escape}')
    await waitFor(() => expect(oldChallenge).not.toBeInTheDocument())
    expect(api.post).toHaveBeenCalledTimes(1)

    await user.click(screen.getByRole('button', { name: 'Check in now' }))
    const reopenedDialog = await screen.findByRole('dialog', {
      name: 'Security Check',
    })
    expect(
      within(reopenedDialog).queryByRole('region', {
        name: 'GeeTest challenge',
      })
    ).not.toBeInTheDocument()
    await user.click(
      within(reopenedDialog).getByRole('button', {
        name: 'Open GeeTest challenge',
      })
    )
    expect(
      within(reopenedDialog).getByRole('button', {
        name: 'Complete GeeTest verification',
      })
    ).toBeEnabled()
  })
})
