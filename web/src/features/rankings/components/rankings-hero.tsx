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
import { useTranslation } from 'react-i18next'

import type { RankingPeriod } from '../types'

const PERIODS: { id: RankingPeriod; labelKey: string }[] = [
  { id: 'today', labelKey: 'Today' },
  { id: 'week', labelKey: 'Week' },
  { id: 'month', labelKey: 'Month' },
  { id: 'year', labelKey: 'Year' },
]

type RankingsHeroProps = {
  period: RankingPeriod
  onPeriodChange: (period: RankingPeriod) => void
}

export function RankingsHero(props: RankingsHeroProps) {
  const { t } = useTranslation()
  return (
    <header className='flex flex-col items-start justify-between gap-5 border-b pb-6 md:flex-row md:items-end'>
      <div>
        <h1 className='text-4xl leading-tight font-semibold tracking-tight sm:text-5xl'>
          {t('Rankings')}
        </h1>
        <p className='text-muted-foreground mt-3 max-w-xl text-base leading-relaxed'>
          {t(
            'Compare model traffic and provider share for the selected period.'
          )}
        </p>
      </div>
      <fieldset className='bg-muted flex max-w-full min-w-0 flex-wrap gap-1 rounded-lg p-1'>
        <legend className='sr-only'>{t('Period')}</legend>
        {PERIODS.map((period) => (
          <label key={period.id} className='relative'>
            <input
              className='peer sr-only'
              type='radio'
              name='rankings-period'
              value={period.id}
              checked={props.period === period.id}
              onChange={() => props.onPeriodChange(period.id)}
            />
            <span className='text-muted-foreground peer-checked:bg-brand-citrus peer-checked:text-brand-ink peer-focus-visible:outline-ring hover:text-foreground flex min-h-9 cursor-pointer items-center rounded-md px-3 text-sm font-medium peer-checked:shadow-xs peer-focus-visible:outline-2'>
              {t(period.labelKey)}
            </span>
          </label>
        ))}
      </fieldset>
    </header>
  )
}
