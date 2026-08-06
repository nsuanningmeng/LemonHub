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
import { Trophy } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { formatQuota, formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { AffLeaderboardItem } from '../types'

interface ReferralLeaderboardTableProps {
  items?: AffLeaderboardItem[]
  loading: boolean
  /** Cash-settled promoter: note that the contribution amounts are settled off-platform in cash. */
  isCashSettled?: boolean
}

const RANK_BADGE_CLASS: Record<number, string> = {
  1: 'bg-amber-500/15 text-amber-600 dark:text-amber-400',
  2: 'bg-zinc-400/15 text-zinc-600 dark:text-zinc-300',
  3: 'bg-orange-500/15 text-orange-600 dark:text-orange-400',
}

export function ReferralLeaderboardTable(props: ReferralLeaderboardTableProps) {
  const { t } = useTranslation()
  const items = props.items ?? []

  return (
    <Card data-card-hover='false'>
      <CardHeader>
        <CardTitle className='flex items-center gap-2'>
          <Trophy className='text-muted-foreground size-4' aria-hidden='true' />
          {t('Top Contributors')}
        </CardTitle>
        <CardDescription>
          {props.isCashSettled
            ? t(
                'Who contributed the most commission. Amounts are settled off-platform in cash.'
              )
            : t('Who contributed the most commission')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className='w-16'>{t('Rank')}</TableHead>
              <TableHead>{t('Invitee')}</TableHead>
              <TableHead className='text-right'>{t('Contribution')}</TableHead>
              <TableHead className='text-right'>{t('Recharges')}</TableHead>
              <TableHead className='text-right'>{t('Last Activity')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {props.loading ? (
              Array.from({ length: 5 }).map((_, i) => (
                <TableRow key={i}>
                  <TableCell>
                    <Skeleton className='h-5 w-6' />
                  </TableCell>
                  <TableCell>
                    <Skeleton className='h-4 w-28' />
                  </TableCell>
                  <TableCell className='text-right'>
                    <Skeleton className='ml-auto h-4 w-16' />
                  </TableCell>
                  <TableCell className='text-right'>
                    <Skeleton className='ml-auto h-4 w-10' />
                  </TableCell>
                  <TableCell className='text-right'>
                    <Skeleton className='ml-auto h-4 w-24' />
                  </TableCell>
                </TableRow>
              ))
            ) : items.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={5}
                  className='text-muted-foreground h-24 text-center'
                >
                  {t('No referral data yet')}
                </TableCell>
              </TableRow>
            ) : (
              items.map((item, index) => {
                const rank = index + 1
                return (
                  <TableRow key={item.invitee_id}>
                    <TableCell>
                      <Badge
                        variant='secondary'
                        className={cn(
                          'tabular-nums',
                          RANK_BADGE_CLASS[rank]
                        )}
                      >
                        {rank}
                      </Badge>
                    </TableCell>
                    <TableCell className='font-medium'>
                      {item.username}
                    </TableCell>
                    <TableCell className='text-right font-semibold tabular-nums'>
                      {formatQuota(item.commission_quota)}
                    </TableCell>
                    <TableCell className='text-right tabular-nums'>
                      {item.recharge_count.toLocaleString()}
                    </TableCell>
                    <TableCell className='text-muted-foreground text-right tabular-nums'>
                      {formatTimestampToDate(item.last_at)}
                    </TableCell>
                  </TableRow>
                )
              })
            )}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}
