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
import { useQuery } from '@tanstack/react-query'
import { Activity, RotateCw } from 'lucide-react'
import { memo } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { IconBadge } from '@/components/ui/icon-badge'
import { getUptimeStatus } from '@/features/dashboard/api'
import type { UptimeMonitor } from '@/features/dashboard/types'
import { cn } from '@/lib/utils'

import { PanelWrapper } from '../ui/panel-wrapper'

const STATUS_COLOR_MAP: Record<number, string> = {
  1: 'bg-emerald-500',
  0: 'bg-red-500',
  2: 'bg-amber-500',
  3: 'bg-blue-500',
}
const DEFAULT_STATUS_COLOR = 'bg-muted-foreground/40'

const StatusDot = memo(function StatusDot(props: { status: number }) {
  const color = STATUS_COLOR_MAP[props.status] ?? DEFAULT_STATUS_COLOR
  return <span className={cn('inline-block size-2 rounded-full', color)} />
})

export function UptimePanel() {
  const { t } = useTranslation()
  const uptimeQuery = useQuery({
    queryKey: ['uptime-status'],
    queryFn: async () => {
      const result = await getUptimeStatus().catch((cause: unknown) => {
        // Monitoring failures belong to this optional panel, not the global error page.
        throw new Error('Failed to load uptime status', { cause })
      })
      if (!result.success) throw new Error('Failed to load')
      return result.data ?? []
    },
    staleTime: 60 * 1000,
    retry: false,
  })
  const groups = uptimeQuery.data ?? []

  return (
    <PanelWrapper
      title={
        <span className='flex items-center gap-2'>
          <IconBadge tone='success' size='sm'>
            <Activity />
          </IconBadge>
          {t('Uptime')}
        </span>
      }
      description={t('Grouped monitor status from Uptime Kuma')}
      loading={uptimeQuery.isPending}
      empty={uptimeQuery.isError || !groups.length}
      emptyMessage={
        uptimeQuery.isError
          ? t('Failed to load')
          : t('No uptime monitoring configured')
      }
      headerActions={
        <Button
          variant='ghost'
          size='sm'
          aria-label={t('Refresh')}
          onClick={() => void uptimeQuery.refetch()}
          disabled={uptimeQuery.isFetching}
          className='size-7 p-0'
        >
          <RotateCw
            className={cn('size-3.5', uptimeQuery.isFetching && 'animate-spin')}
            aria-hidden='true'
          />
        </Button>
      }
    >
      <div>
        {groups.map((group, groupIdx) => (
          <div key={group.categoryName}>
            <div className='bg-muted/30 border-border/60 border-b px-3 py-2 sm:px-5'>
              <div className='flex items-center gap-2'>
                <h4 className='text-muted-foreground text-xs font-semibold tracking-wider uppercase'>
                  {group.categoryName}
                </h4>
                <span className='text-muted-foreground font-mono text-xs tabular-nums'>
                  {group.monitors?.length || 0}
                </span>
              </div>
            </div>

            {group.monitors?.map(
              (monitor: UptimeMonitor, monitorIdx: number) => (
                <div
                  key={monitor.name}
                  className={cn(
                    'hover:bg-muted/40 flex items-center justify-between gap-2 px-3 py-2 transition-colors sm:px-5 sm:py-2.5',
                    monitorIdx < (group.monitors?.length || 0) - 1 &&
                      'border-border/40 border-b',
                    groupIdx < groups.length - 1 &&
                      monitorIdx === (group.monitors?.length || 0) - 1 &&
                      'border-border/60 border-b'
                  )}
                >
                  <div className='flex min-w-0 items-center gap-2.5'>
                    <StatusDot status={monitor.status} />
                    <span className='truncate text-sm'>{monitor.name}</span>
                    {monitor.group && (
                      <span className='text-muted-foreground shrink-0 text-xs'>
                        ({monitor.group})
                      </span>
                    )}
                  </div>
                  <span className='text-foreground shrink-0 font-mono text-sm font-semibold tabular-nums'>
                    {((monitor.uptime ?? 0) * 100).toFixed(2)}%
                  </span>
                </div>
              )
            )}
          </div>
        ))}
      </div>
    </PanelWrapper>
  )
}
