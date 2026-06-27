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
import {
  ArrowDown,
  ArrowUp,
  ArrowUpDown,
  Banknote,
  Globe,
  Search,
} from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useDebounce } from '@/hooks/use-debounce'
import { formatQuota, formatTimestampToDate } from '@/lib/format'
import { ROLE } from '@/lib/roles'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { useReferralAdminLeaderboard } from '../hooks/use-referral-admin-leaderboard'
import type {
  AffAdminLeaderboardItem,
  AffAdminSortColumn,
  AffAdminSortOrder,
} from '../types'
import { ReferralCashSettlementDialog } from './referral-cash-settlement-dialog'

const PAGE_SIZE = 10

const RANK_BADGE_CLASS: Record<number, string> = {
  1: 'bg-amber-500/15 text-amber-600 dark:text-amber-400',
  2: 'bg-zinc-400/15 text-zinc-600 dark:text-zinc-300',
  3: 'bg-orange-500/15 text-orange-600 dark:text-orange-400',
}

// Stable keys for the loading skeleton (avoids array-index keys). One inviter row =
// rank + name + 7 right-aligned numeric cells, plus an actions cell for root users.
const SKELETON_ROWS = ['r1', 'r2', 'r3', 'r4', 'r5']
const SKELETON_CELLS = ['c1', 'c2', 'c3', 'c4', 'c5', 'c6', 'c7', 'c8']

interface SortHeaderProps {
  label: string
  column: AffAdminSortColumn
  activeColumn: AffAdminSortColumn
  order: AffAdminSortOrder
  onSort: (column: AffAdminSortColumn) => void
  align?: 'left' | 'right'
}

/** A clickable column header that toggles server-side sort for a whitelisted column. */
function SortHeader(props: SortHeaderProps) {
  const isActive = props.activeColumn === props.column
  let Icon = ArrowUpDown
  if (isActive) {
    Icon = props.order === 'asc' ? ArrowUp : ArrowDown
  }
  return (
    <button
      type='button'
      onClick={() => props.onSort(props.column)}
      className={cn(
        'text-muted-foreground hover:text-foreground inline-flex items-center gap-1 transition-colors',
        props.align === 'right' && 'flex-row-reverse',
        isActive && 'text-foreground'
      )}
    >
      <span>{props.label}</span>
      <Icon className='size-3.5 shrink-0 opacity-70' aria-hidden='true' />
    </button>
  )
}

export function ReferralAdminLeaderboard() {
  const { t } = useTranslation()
  const [keywordInput, setKeywordInput] = useState('')
  const keyword = useDebounce(keywordInput.trim(), 400)
  const [sort, setSort] = useState<AffAdminSortColumn>('total_earned')
  const [order, setOrder] = useState<AffAdminSortOrder>('desc')
  const [page, setPage] = useState(1)
  const [settlementItem, setSettlementItem] =
    useState<AffAdminLeaderboardItem | null>(null)

  // Money-policy controls (recording cash settlements) are root-only; the backend rejects
  // non-root admins with 403, so hide the action + its column for them entirely.
  const isRoot = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const columnCount = isRoot ? 10 : 9
  const skeletonCells = isRoot ? SKELETON_CELLS : SKELETON_CELLS.slice(0, 7)

  const { data, isLoading, isFetching, isError, refetch } =
    useReferralAdminLeaderboard({
      page,
      pageSize: PAGE_SIZE,
      keyword: keyword || undefined,
      sort,
      order,
    })

  const items = data?.items ?? []
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  // Medals only mean something under the canonical ranking (earnings, descending).
  const showMedals = sort === 'total_earned' && order === 'desc'
  // Rank off the server-echoed page so numbering matches the rows actually shown: with
  // keepPreviousData the local `page` advances before the next page's rows arrive.
  const displayPage = data?.page ?? page

  const sortAria = (
    column: AffAdminSortColumn
  ): 'ascending' | 'descending' | 'none' => {
    if (sort !== column) return 'none'
    return order === 'asc' ? 'ascending' : 'descending'
  }

  const handleSort = (column: AffAdminSortColumn) => {
    if (column === sort) {
      setOrder((prev) => (prev === 'asc' ? 'desc' : 'asc'))
    } else {
      setSort(column)
      setOrder('desc')
    }
    setPage(1)
  }

  const handleKeyword = (value: string) => {
    setKeywordInput(value)
    setPage(1)
  }

  let body: ReactNode
  if (isLoading) {
    body = SKELETON_ROWS.map((rowKey) => (
      <TableRow key={rowKey}>
        <TableCell>
          <Skeleton className='h-5 w-6' />
        </TableCell>
        <TableCell>
          <Skeleton className='h-4 w-28' />
        </TableCell>
        {skeletonCells.map((cellKey) => (
          <TableCell key={cellKey} className='text-right'>
            <Skeleton className='ml-auto h-4 w-16' />
          </TableCell>
        ))}
      </TableRow>
    ))
  } else if (isError && items.length === 0) {
    body = (
      <TableRow>
        <TableCell colSpan={columnCount} className='h-24 text-center'>
          <div className='text-muted-foreground flex flex-col items-center gap-2'>
            <span>{t('Failed to load')}</span>
            <Button variant='outline' size='sm' onClick={() => refetch()}>
              {t('Retry')}
            </Button>
          </div>
        </TableCell>
      </TableRow>
    )
  } else if (items.length === 0) {
    body = (
      <TableRow>
        <TableCell
          colSpan={columnCount}
          className='text-muted-foreground h-24 text-center'
        >
          {t('No referral data yet')}
        </TableCell>
      </TableRow>
    )
  } else {
    body = items.map((item, index) => {
      const rank = (displayPage - 1) * PAGE_SIZE + index + 1
      return (
        <TableRow key={item.inviter_id}>
          <TableCell>
            <Badge
              variant='secondary'
              className={cn(
                'tabular-nums',
                showMedals ? RANK_BADGE_CLASS[rank] : undefined
              )}
            >
              {rank}
            </Badge>
          </TableCell>
          <TableCell className='font-medium'>
            <div className='flex flex-col'>
              <div className='flex items-center gap-1.5'>
                <span className='truncate'>{item.username}</span>
                {item.is_cash_settled && (
                  <Badge
                    variant='secondary'
                    className='shrink-0 bg-emerald-500/15 text-emerald-600 dark:text-emerald-400'
                  >
                    {t('Cash settled')}
                  </Badge>
                )}
              </div>
              {item.display_name && item.display_name !== item.username && (
                <span className='text-muted-foreground truncate text-xs'>
                  {item.display_name}
                </span>
              )}
            </div>
          </TableCell>
          <TableCell className='text-right font-semibold tabular-nums'>
            {formatQuota(item.total_earned_quota)}
          </TableCell>
          <TableCell className='text-right tabular-nums'>
            {formatQuota(item.pending_quota)}
          </TableCell>
          <TableCell className='text-right tabular-nums'>
            {item.activated_count.toLocaleString()}
          </TableCell>
          <TableCell className='text-right tabular-nums'>
            {item.total_invited.toLocaleString()}
          </TableCell>
          <TableCell className='text-right tabular-nums'>
            {formatQuota(item.cash_commission_owed ?? 0)}
          </TableCell>
          <TableCell className='text-right tabular-nums'>
            {formatQuota(item.month_commission_quota)}
          </TableCell>
          <TableCell className='text-muted-foreground text-right tabular-nums'>
            {item.last_at > 0 ? formatTimestampToDate(item.last_at) : '—'}
          </TableCell>
          {isRoot && (
            <TableCell className='text-right'>
              {(item.is_cash_settled ||
                (item.cash_commission_total ?? 0) > 0) && (
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => setSettlementItem(item)}
                >
                  <Banknote className='size-3.5' aria-hidden='true' />
                  {t('Record cash settlement')}
                </Button>
              )}
            </TableCell>
          )}
        </TableRow>
      )
    })
  }

  return (
    <>
      <Card data-card-hover='false'>
        <CardHeader>
          <div className='flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between'>
            <div className='space-y-1.5'>
              <CardTitle className='flex items-center gap-2'>
                <Globe
                  className='text-muted-foreground size-4'
                  aria-hidden='true'
                />
                {t('Global Referral Leaderboard')}
              </CardTitle>
              <CardDescription>
                {t('All inviters ranked by referral earnings')}
              </CardDescription>
            </div>
            <div className='relative w-full sm:w-64'>
              <Search
                className='text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2'
                aria-hidden='true'
              />
              <Input
                value={keywordInput}
                onChange={(e) => handleKeyword(e.target.value)}
                placeholder={t('Search inviter')}
                className='h-9 pl-8'
                aria-label={t('Search inviter')}
              />
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <div className='overflow-x-auto'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className='w-16'>{t('Rank')}</TableHead>
                  <TableHead aria-sort={sortAria('username')}>
                    <SortHeader
                      label={t('Inviter')}
                      column='username'
                      activeColumn={sort}
                      order={order}
                      onSort={handleSort}
                    />
                  </TableHead>
                  <TableHead
                    className='text-right'
                    aria-sort={sortAria('total_earned')}
                  >
                    <SortHeader
                      label={t('Total Earned')}
                      column='total_earned'
                      activeColumn={sort}
                      order={order}
                      onSort={handleSort}
                      align='right'
                    />
                  </TableHead>
                  <TableHead
                    className='text-right'
                    aria-sort={sortAria('pending')}
                  >
                    <SortHeader
                      label={t('Pending')}
                      column='pending'
                      activeColumn={sort}
                      order={order}
                      onSort={handleSort}
                      align='right'
                    />
                  </TableHead>
                  <TableHead
                    className='text-right'
                    aria-sort={sortAria('activated')}
                  >
                    <SortHeader
                      label={t('Activated')}
                      column='activated'
                      activeColumn={sort}
                      order={order}
                      onSort={handleSort}
                      align='right'
                    />
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('Total Invited')}
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('Cash commission owed')}
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('This Month')}
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('Last Activity')}
                  </TableHead>
                  {isRoot && (
                    <TableHead className='text-right'>{t('Actions')}</TableHead>
                  )}
                </TableRow>
              </TableHeader>
              <TableBody>{body}</TableBody>
            </Table>
          </div>

          <div className='mt-3 flex items-center justify-between gap-2'>
            <span className='text-muted-foreground text-xs tabular-nums'>
              {t('Total inviters')}: {total.toLocaleString()}
            </span>
            <div className='flex items-center gap-2'>
              <Button
                variant='outline'
                size='sm'
                disabled={page <= 1 || isFetching}
                onClick={() => setPage((p) => Math.max(1, p - 1))}
              >
                {t('Previous')}
              </Button>
              <span className='text-muted-foreground text-xs tabular-nums'>
                {page} / {totalPages}
              </span>
              <Button
                variant='outline'
                size='sm'
                disabled={page >= totalPages || isFetching}
                onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              >
                {t('Next')}
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      <ReferralCashSettlementDialog
        open={settlementItem !== null}
        onOpenChange={(v) => {
          if (!v) setSettlementItem(null)
        }}
        item={settlementItem}
      />
    </>
  )
}
