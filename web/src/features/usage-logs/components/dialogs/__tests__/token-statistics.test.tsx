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
import { describe, expect, test } from 'vitest'

import { formatLogQuota } from '@/lib/format'

import { usageLogSchema } from '../../../data/schema'
import { DetailsDialog } from '../details-dialog'

function renderTokenDetails(fields: Record<string, unknown>): void {
  const log = usageLogSchema.parse({
    id: 1,
    user_id: 1,
    created_at: 1728000000,
    type: 2,
    content: '',
    prompt_tokens: 2,
    completion_tokens: 1117,
    quota: 5000,
    other: JSON.stringify({
      cache_tokens: 589733,
      cache_creation_tokens: 1088,
    }),
    ...fields,
  })
  render(
    <DetailsDialog
      log={log}
      isAdmin={false}
      open
      onOpenChange={() => undefined}
    />
  )
}

describe('inclusive input token statistics', () => {
  test('shows inclusive statistics beside raw input and cache counts without changing the cost', () => {
    renderTokenDetails({ input_tokens_total: 590823 })
    expect(
      screen.getByText('Total Input Tokens (including cache)').parentElement
    ).toHaveTextContent('590,823')
    expect(screen.getByText('Input Tokens').parentElement).toHaveTextContent(
      /^Input Tokens\s*2$/
    )
    expect(screen.getByText('Cache Read').parentElement).toHaveTextContent(
      '589,733'
    )
    expect(screen.getByText('Total Cost').parentElement).toHaveTextContent(
      formatLogQuota(5000)
    )
  })

  test.each([null, undefined])(
    'does not invent an inclusive total for a legacy log (%s)',
    (inputTokensTotal) => {
      renderTokenDetails({ input_tokens_total: inputTokensTotal })
      expect(
        screen.queryByText('Total Input Tokens (including cache)')
      ).not.toBeInTheDocument()
      expect(screen.getByText('Input Tokens')).toBeVisible()
    }
  )

  test('preserves an explicit zero total on a free refusal with no output', () => {
    renderTokenDetails({
      input_tokens_total: 0,
      prompt_tokens: 0,
      completion_tokens: 0,
      quota: 0,
      other: '{}',
    })
    expect(
      screen.getByText('Total Input Tokens (including cache)').parentElement
    ).toHaveTextContent('0')
    expect(screen.getByText('Total Cost').parentElement).toHaveTextContent(
      formatLogQuota(0)
    )
  })

  test('keeps cache-only legacy usage visible without guessing its input total', () => {
    renderTokenDetails({ prompt_tokens: 0, completion_tokens: 0 })
    expect(screen.getByText('Cache Read').parentElement).toHaveTextContent(
      '589,733'
    )
    expect(
      screen.queryByText('Total Input Tokens (including cache)')
    ).not.toBeInTheDocument()
  })
})
