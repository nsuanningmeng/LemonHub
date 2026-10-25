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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatQuotaWithCurrency } from '@/lib/currency'
import dayjs from '@/lib/dayjs'

import { useRewards } from '../hooks/use-rewards'
import { RewardPagination } from './reward-pagination'

export function RewardHistory() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [status, setStatus] = useState<'claimed' | 'expired'>('claimed')
  const history = useRewards({ status, p: page, page_size: 10 })
  const expiredHistory = status === 'expired'
  let emptyMessage = expiredHistory
    ? t('No expired rewards')
    : t('No rewards claimed yet')
  if (history.isPending) emptyMessage = t('Loading...')
  return (
    <section
      aria-labelledby='reward-history-heading'
      className='bg-card rounded-3xl border p-5 sm:p-7'
    >
      <h2 id='reward-history-heading' className='mb-5 text-lg font-semibold'>
        {t('Reward history')}
      </h2>
      <Tabs
        value={status}
        onValueChange={(value) => {
          if (value === 'claimed' || value === 'expired') {
            setStatus(value)
            setPage(1)
          }
        }}
        className='mb-5'
      >
        <TabsList aria-label={t('Reward history status')}>
          <TabsTrigger value='claimed' className='px-4'>
            {t('Claimed')}
          </TabsTrigger>
          <TabsTrigger value='expired' className='px-4'>
            {t('Expired')}
          </TabsTrigger>
        </TabsList>
      </Tabs>
      {history.isError ? (
        <div role='alert' className='space-y-3 text-sm'>
          <p>{t('Unable to load rewards')}</p>
          <Button
            type='button'
            variant='outline'
            onClick={() => void history.refetch()}
          >
            {t('Retry')}
          </Button>
        </div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow className='bg-muted/60'>
              <TableHead>{t('Campaign')}</TableHead>
              <TableHead>{t('Type')}</TableHead>
              <TableHead>{t('Amount')}</TableHead>
              <TableHead>{t('Status')}</TableHead>
              <TableHead>
                {expiredHistory ? t('Expires at') : t('Claimed at')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {history.data?.items.map((reward) => (
              <TableRow key={reward.id}>
                <TableCell className='max-w-72 wrap-anywhere whitespace-normal'>
                  {reward.title}
                </TableCell>
                <TableCell>
                  {reward.type === 'random'
                    ? t('Lucky red packet')
                    : t('Reward credit')}
                </TableCell>
                <TableCell className='font-medium'>
                  {expiredHistory
                    ? '—'
                    : `+${formatQuotaWithCurrency(reward.quota_awarded)}`}
                </TableCell>
                <TableCell>
                  {expiredHistory ? (
                    <span className='text-muted-foreground bg-muted rounded-md px-2 py-1 text-xs'>
                      {t('Expired')}
                    </span>
                  ) : (
                    <span className='rounded-md bg-emerald-500/10 px-2 py-1 text-xs text-emerald-700 dark:text-emerald-300'>
                      {t('Claimed')}
                    </span>
                  )}
                </TableCell>
                <TableCell className='text-muted-foreground'>
                  {dayjs
                    .unix(
                      expiredHistory ? reward.expires_at : reward.claimed_at
                    )
                    .format('YYYY-MM-DD HH:mm')}
                </TableCell>
              </TableRow>
            ))}
            {!history.data?.items.length && (
              <TableRow>
                <TableCell
                  colSpan={5}
                  className='text-muted-foreground py-12 text-center'
                >
                  {emptyMessage}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      )}
      <RewardPagination
        page={page}
        pageSize={10}
        total={history.data?.total ?? 0}
        loading={history.isFetching}
        onChange={setPage}
      />
    </section>
  )
}
