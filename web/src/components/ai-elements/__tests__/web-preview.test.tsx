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

import { WebPreview, WebPreviewBody } from '../web-preview'

test('inline previews get an opaque origin while hosted previews retain their origin', () => {
  const { rerender } = render(
    <WebPreview defaultUrl='https://example.org/'>
      <WebPreviewBody title='Document' srcDoc='<p>Preview</p>' />
    </WebPreview>
  )
  const frame = screen.getByTitle('Document')
  expect(frame.getAttribute('sandbox')?.split(' ')).not.toContain(
    'allow-same-origin'
  )
  expect(frame.getAttribute('sandbox')?.split(' ')).toContain('allow-scripts')

  rerender(
    <WebPreview defaultUrl='https://example.org/'>
      <WebPreviewBody title='Document' />
    </WebPreview>
  )
  expect(frame).toHaveAttribute('src', 'https://example.org/')
  expect(frame.getAttribute('sandbox')?.split(' ')).toContain(
    'allow-same-origin'
  )

  rerender(
    <WebPreview>
      <WebPreviewBody title='Document' srcDoc='<p>Preview</p>' sandbox='' />
    </WebPreview>
  )
  expect(frame).toHaveAttribute('sandbox', '')
})
