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
import { useEffect, useState } from 'react'

import { resolveThemeColor } from '@/lib/theme-color'

import { getDashboardChartColors } from '../lib/charts'

// The canvas renderer needs actual color values. Preserve enough distinct
// colors for accounts with more series than the five customizable theme slots.
export function useDashboardChartPalette(): string[] | undefined {
  const [palette, setPalette] = useState<string[]>()

  useEffect(() => {
    const update = () => {
      const styles = getComputedStyle(document.body)
      const colors: string[] = []
      for (let index = 1; index <= 5; index++) {
        const token = `--chart-${index}`
        const value = styles.getPropertyValue(token).trim()
        if (!value) return
        const color = value.startsWith('#') ? value : resolveThemeColor(token)
        if (!color) return
        colors.push(color)
      }
      setPalette([
        ...colors,
        ...getDashboardChartColors(20).slice(colors.length),
      ])
    }
    update()
    const observer = new MutationObserver(update)
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class'],
    })
    observer.observe(document.body, {
      attributes: true,
      attributeFilter: ['data-theme-preset'],
    })
    return () => observer.disconnect()
  }, [])
  return palette
}
