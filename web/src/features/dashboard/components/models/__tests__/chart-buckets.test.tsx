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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import type { QuotaDataItem } from '../../../types'
import { ConsumptionDistributionChart } from '../consumption-distribution-chart'
import { ModelCharts } from '../model-charts'

interface RenderedChartSpec {
  type: string
  color?: { range?: string[] }
  xField: string
  data: {
    values: {
      Time?: string
      Timestamp?: number
      rawQuota?: number
      Count?: number
      value?: number
    }[]
  }[]
  axes?: {
    orient: string
    label?: { formatMethod: (value: number) => string }
  }[]
}

// Canvas is the external rendering boundary; the real consumers, chart builder,
// UI controls, theme context defaults and generated chart specs remain in use.
const rendered = vi.hoisted(() => ({ specs: [] as RenderedChartSpec[] }))
vi.mock('@visactor/react-vchart', () => ({
  VChart: (props: { spec: RenderedChartSpec }) => {
    rendered.specs.push(props.spec)
    return <output aria-label='Chart rendered'>{props.spec.type}</output>
  },
}))
vi.mock('@visactor/vchart', () => ({
  ThemeManager: { setCurrentTheme: () => undefined },
}))

beforeEach(() => {
  rendered.specs = []
  vi.stubEnv('TZ', 'UTC')
})
afterEach(() => {
  vi.unstubAllEnvs()
})

const rows: QuotaDataItem[] = [
  {
    created_at: 1767225600,
    model_name: 'local-model',
    count: 5,
    quota: 1500000,
  },
  {
    created_at: 1735689600,
    model_name: 'local-model',
    count: 2,
    quota: 500000,
  },
]

test('consumption chart switching preserves two equal labels as distinct ordered buckets', async () => {
  const user = userEvent.setup()
  render(<ConsumptionDistributionChart data={rows} timeGranularity='day' />)
  expect(await screen.findByLabelText('Chart rendered')).toHaveTextContent(
    'bar'
  )
  const bar = rendered.specs.at(-1)
  expect(bar?.xField).toBe('Timestamp')
  expect(bar?.data[0].values.map((row) => row.Timestamp)).toEqual([
    1735689600, 1767225600,
  ])
  expect(bar?.data[0].values.map((row) => row.rawQuota)).toEqual([
    500000, 1500000,
  ])
  const axis = bar?.axes?.find((axis) => axis.orient === 'bottom')
  expect(axis?.label?.formatMethod(1735689600)).toBe('01-01')
  expect(axis?.label?.formatMethod(1767225600)).toBe('01-01')
  const areaButton = screen.getByRole('button', {
    name: 'Area Chart',
    pressed: false,
  })
  areaButton.focus()
  await user.keyboard('{Enter}')
  expect(areaButton).toHaveAttribute('aria-pressed', 'true')
  await waitFor(() => {
    expect(screen.getByLabelText('Chart rendered')).toHaveTextContent('area')
  })
  const area = rendered.specs.at(-1)
  expect(area?.data[0].values.map((row) => row.Timestamp)).toEqual([
    1735689600, 1767225600,
  ])
  expect(area?.data[0].values.map((row) => row.rawQuota)).toEqual([
    500000, 1500000,
  ])
})

test('model chart switching keeps real trend buckets and matching distribution totals', async () => {
  const user = userEvent.setup()
  render(<ModelCharts data={rows} timeGranularity='day' />)
  expect(await screen.findByLabelText('Chart rendered')).toHaveTextContent(
    'area'
  )
  expect(rendered.specs.at(-1)?.data[0].values.map((row) => row.Count)).toEqual(
    [2, 5]
  )
  await user.click(
    screen.getByRole('button', { name: 'Call Count Distribution' })
  )
  await waitFor(() => {
    expect(screen.getByLabelText('Chart rendered')).toHaveTextContent('pie')
  })
  expect(rendered.specs.at(-1)?.data[0].values.map((row) => row.value)).toEqual(
    [7]
  )
  await user.click(screen.getByRole('button', { name: 'Call Trend' }))
  await waitFor(() => {
    expect(screen.getByLabelText('Chart rendered')).toHaveTextContent('area')
  })
  expect(
    rendered.specs.at(-1)?.data[0].values.map((row) => row.Timestamp)
  ).toEqual([1735689600, 1767225600])
})

test('switching the active preset updates chart series colors while keeping the selected chart', async () => {
  const colors = ['#346532', '#369098', '#b99446', '#7d589b', '#ba6356']
  const oldPreset = document.body.getAttribute('data-theme-preset')
  const previous = colors.map((_, index) =>
    document.body.style.getPropertyValue(`--chart-${index + 1}`)
  )
  try {
    colors.forEach((color, index) =>
      document.body.style.setProperty(`--chart-${index + 1}`, color)
    )
    render(<ConsumptionDistributionChart data={rows} />)
    await waitFor(() =>
      expect(rendered.specs.at(-1)?.color?.range?.slice(0, 5)).toEqual(colors)
    )
    document.body.style.setProperty('--chart-1', '#b3db9a')
    document.body.setAttribute('data-theme-preset', 'chart-test')
    await waitFor(() =>
      expect(rendered.specs.at(-1)?.color?.range?.[0]).toBe('#b3db9a')
    )
    expect(
      screen.getByRole('button', { name: 'Bar Chart', pressed: true })
    ).toBeVisible()
  } finally {
    previous.forEach((value, index) => {
      const token = `--chart-${index + 1}`
      if (value) document.body.style.setProperty(token, value)
      else document.body.style.removeProperty(token)
    })
    if (oldPreset === null) document.body.removeAttribute('data-theme-preset')
    else document.body.setAttribute('data-theme-preset', oldPreset)
  }
})
