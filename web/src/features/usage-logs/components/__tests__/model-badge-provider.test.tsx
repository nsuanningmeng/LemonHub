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
import { expect, test } from 'vitest'

import { ModelBadge } from '../model-badge'

test.each(['codex-mini-latest', 'CODEX-mini-latest', 'gpt-4.1'])(
  '%s displays the OpenAI icon and preserves its model name',
  (modelName) => {
    render(<ModelBadge modelName={modelName} />)
    expect(screen.getByLabelText('OpenAI').querySelector('svg')).not.toBeNull()
    expect(screen.getByText(modelName)).toBeVisible()
  }
)
test('Claude and unknown models preserve their provider display', () => {
  render(
    <>
      <ModelBadge modelName='claude-sonnet-4' />
      <ModelBadge modelName='private-custom-model' />
    </>
  )
  expect(screen.getByLabelText('Claude')).toBeVisible()
  expect(screen.getByText('private-custom-model')).toBeVisible()
  expect(screen.queryByLabelText('OpenAI')).toBeNull()
})
