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
import { act, cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { Turnstile } from '../turnstile'

function installTurnstileSdk() {
  window.turnstile = {
    render: (element, options) => {
      const button = document.createElement('button')
      button.type = 'button'
      button.textContent = `Verify ${options.sitekey}`
      button.addEventListener('click', () => {
        const callback = options.callback as (token: string) => void
        callback(`token-${options.sitekey}`)
      })
      element.append(button)
    },
  }
}

beforeEach(() => {
  delete window.turnstile
  document.querySelector('#cf-turnstile')?.remove()
})

afterEach(() => {
  cleanup()
  delete window.turnstile
  document.querySelector('#cf-turnstile')?.remove()
})

describe('Turnstile SDK loading lifecycle', () => {
  test('renders verification after closing and reopening before the SDK loads', async () => {
    const user = userEvent.setup()
    const onOldVerify = vi.fn()
    const onVerify = vi.fn()
    const first = render(<Turnstile siteKey='email' onVerify={onOldVerify} />)
    const script = document.querySelector('#cf-turnstile')
    expect(script).toBeInTheDocument()
    first.unmount()
    render(<Turnstile siteKey='email' onVerify={onVerify} />)

    installTurnstileSdk()
    act(() => script?.dispatchEvent(new Event('load')))

    await user.click(screen.getByRole('button', { name: 'Verify email' }))
    expect(onVerify).toHaveBeenCalledWith('token-email')
    expect(onOldVerify).not.toHaveBeenCalled()
  })

  test('initializes all mounted widgets sharing a pending SDK script', async () => {
    const user = userEvent.setup()
    const onEmailVerify = vi.fn()
    const onCheckinVerify = vi.fn()
    render(
      <>
        <Turnstile siteKey='email' onVerify={onEmailVerify} />
        <Turnstile siteKey='checkin' onVerify={onCheckinVerify} />
      </>
    )

    installTurnstileSdk()
    act(() =>
      document.querySelector('#cf-turnstile')?.dispatchEvent(new Event('load'))
    )

    await user.click(screen.getByRole('button', { name: 'Verify email' }))
    await user.click(screen.getByRole('button', { name: 'Verify checkin' }))
    expect(onEmailVerify).toHaveBeenCalledWith('token-email')
    expect(onCheckinVerify).toHaveBeenCalledWith('token-checkin')
  })

  test('uses the latest site key if it changes before the SDK loads', async () => {
    const user = userEvent.setup()
    const onVerify = vi.fn()
    const view = render(<Turnstile siteKey='old' onVerify={onVerify} />)
    view.rerender(<Turnstile siteKey='current' onVerify={onVerify} />)

    installTurnstileSdk()
    act(() =>
      document.querySelector('#cf-turnstile')?.dispatchEvent(new Event('load'))
    )

    expect(
      screen.queryByRole('button', { name: 'Verify old' })
    ).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Verify current' }))
    expect(onVerify).toHaveBeenCalledWith('token-current')
  })

  test('renders immediately when the SDK is already available', async () => {
    const user = userEvent.setup()
    const onVerify = vi.fn()
    installTurnstileSdk()

    render(<Turnstile siteKey='email' onVerify={onVerify} />)

    await user.click(screen.getByRole('button', { name: 'Verify email' }))
    expect(onVerify).toHaveBeenCalledWith('token-email')
  })
})
