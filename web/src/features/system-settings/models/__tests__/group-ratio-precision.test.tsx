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
import { fireEvent, render, screen } from '@testing-library/react'
import { expect, test, vi } from 'vitest'

import { GroupRatioVisualEditor } from '../group-ratio-visual-editor'

test.each([0.0001, 0.125, 1.2345])(
  'accepts and preserves group and top-up ratios of %s',
  (ratio) => {
    const onChange = vi.fn()
    render(
      <GroupRatioVisualEditor
        groupRatio='{"default":1}'
        topupGroupRatio='{"default":1}'
        userUsableGroups='{"default":"Default"}'
        groupGroupRatio='{}'
        autoGroups='[]'
        maxTokenAutoGroupsField={null}
        groupSpecialUsableGroup='{}'
        onChange={onChange}
      />
    )

    const [groupRatioInput, topupRatioInput] = screen.getAllByRole('spinbutton')
    fireEvent.change(groupRatioInput, { target: { value: String(ratio) } })
    expect(groupRatioInput).toBeValid()
    expect(onChange).toHaveBeenCalledWith(
      'GroupRatio',
      JSON.stringify({ default: ratio }, null, 2)
    )

    fireEvent.change(topupRatioInput, { target: { value: String(ratio) } })
    expect(topupRatioInput).toBeValid()
    expect(onChange).toHaveBeenCalledWith(
      'TopupGroupRatio',
      JSON.stringify({ default: ratio }, null, 2)
    )

    fireEvent.change(groupRatioInput, { target: { value: '-0.0001' } })
    expect(groupRatioInput).toBeInvalid()
    fireEvent.change(topupRatioInput, { target: { value: '-0.0001' } })
    expect(topupRatioInput).toBeInvalid()
  }
)
