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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'

import { MaskedValueDisplay } from '@/components/masked-value-display'

it('lets a long masked value shrink while preserving independent copy and edit actions', async () => {
  const user = userEvent.setup()
  const edit = vi.fn()
  render(
    <section className='flex w-64'>
      <MaskedValueDisplay
        label='Full Code'
        fullValue='full-redemption-code'
        maskedValue='aaaaaaa******aaaaaaa'
        copyTooltip='Copy code'
        copyAriaLabel='Copy redemption code'
      />
      <button type='button' onClick={edit}>
        Edit
      </button>
    </section>
  )
  const value = screen.getByRole('button', { name: 'aaaaaaa******aaaaaaa' })
  expect(value).toHaveClass('shrink')
  expect(value).not.toHaveClass('shrink-0')
  expect(
    screen.getByRole('button', { name: 'Copy redemption code' })
  ).toHaveClass('shrink-0')
  await user.click(screen.getByRole('button', { name: 'Copy redemption code' }))
  expect(await navigator.clipboard.readText()).toBe('full-redemption-code')
  await user.click(screen.getByRole('button', { name: 'Edit' }))
  expect(edit).toHaveBeenCalledOnce()
})
