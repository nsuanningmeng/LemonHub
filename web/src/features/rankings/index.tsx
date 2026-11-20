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
import { useNavigate, useSearch } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { Skeleton } from '@/components/ui/skeleton'

import {
  MarketShareSection,
  ModelsSection,
  PulseSection,
  RankingsHero,
} from './components'
import { useRankings } from './hooks/use-rankings'
import type { RankingPeriod } from './types'

const VALID_PERIODS: RankingPeriod[] = ['today', 'week', 'month', 'year']

export function Rankings() {
  const { t } = useTranslation()
  const search = useSearch({ from: '/rankings/' })
  const navigate = useNavigate()

  const period: RankingPeriod = VALID_PERIODS.includes(
    search.period as RankingPeriod
  )
    ? (search.period as RankingPeriod)
    : 'week'

  const rankingsQuery = useRankings(period)
  const snapshot = rankingsQuery.data?.data

  const handlePeriodChange = (next: RankingPeriod) => {
    navigate({
      to: '/rankings',
      search: (prev) => ({ ...prev, period: next }),
    })
  }

  let content
  if (rankingsQuery.isLoading) {
    content = <RankingsLoading />
  } else if (
    snapshot &&
    [
      snapshot.models,
      snapshot.vendors,
      snapshot.top_movers,
      snapshot.top_droppers,
      snapshot.models_history?.points,
      snapshot.vendor_share_history?.points,
    ].every((rows) => !rows?.length)
  ) {
    content = (
      <section className='border-border border-y py-14' role='status'>
        <h2 className='text-2xl font-semibold tracking-tight'>
          {t('No usage in this period')}
        </h2>
        <p className='text-muted-foreground mt-2'>
          {t('Choose another period to see model activity.')}
        </p>
      </section>
    )
  } else if (snapshot) {
    content = (
      <div className='space-y-10'>
        <div className='grid items-start gap-8 xl:grid-cols-2'>
          <ModelsSection
            history={snapshot.models_history}
            rows={snapshot.models}
            period={period}
          />

          <MarketShareSection
            history={snapshot.vendor_share_history}
            rows={snapshot.vendors}
            period={period}
          />
        </div>
        <PulseSection
          movers={snapshot.top_movers}
          droppers={snapshot.top_droppers}
        />
      </div>
    )
  } else {
    content = (
      <RankingsError
        message={
          rankingsQuery.error instanceof Error
            ? rankingsQuery.error.message
            : t('Unable to load rankings data')
        }
      />
    )
  }

  return (
    <PublicLayout showMainContainer={false}>
      <main className='mx-auto w-full max-w-[76rem] space-y-6 px-5 pt-28 pb-12 sm:px-8 md:pt-32'>
        <RankingsHero period={period} onPeriodChange={handlePeriodChange} />

        {content}
      </main>
    </PublicLayout>
  )
}

function RankingsLoading() {
  return (
    <div aria-busy='true' className='space-y-6'>
      <Skeleton className='h-[420px] w-full rounded-xl' />
      <Skeleton className='h-[360px] w-full rounded-xl' />
      <Skeleton className='h-[180px] w-full rounded-xl' />
    </div>
  )
}

function RankingsError(props: { message: string }) {
  const { t } = useTranslation()
  return (
    <div role='alert' className='border-destructive border-s-2 py-1 ps-4'>
      <h2 className='text-foreground text-base font-semibold'>
        {t('Unable to load rankings')}
      </h2>
      <p className='text-muted-foreground mt-2 max-w-prose text-sm'>
        {props.message}
      </p>
    </div>
  )
}
