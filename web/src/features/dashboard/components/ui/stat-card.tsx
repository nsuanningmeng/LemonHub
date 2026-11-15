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
import { HStack } from '@astryxdesign/core/HStack'
import { VStack } from '@astryxdesign/core/VStack'
import type { LucideIcon } from 'lucide-react'
import { useId, type ReactNode } from 'react'

import type { IconBadgeTone } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

type StatCardTone = 'accent-1' | 'accent-2' | 'accent-3'
type StatCardSparklineVariant = 'bars' | 'line'
type StatCardDetailTone =
  | 'default'
  | 'muted'
  | 'success'
  | 'warning'
  | 'destructive'

export interface StatCardDetail {
  label: string
  value: string
  tone?: StatCardDetailTone
}

interface StatCardProps {
  title: string
  value: string | number
  description: string
  icon: LucideIcon
  sparkline?: number[]
  sparklineVariant?: StatCardSparklineVariant
  details?: StatCardDetail[]
  tone?: StatCardTone
  loading?: boolean
  error?: boolean
  action?: ReactNode
  iconTone?: IconBadgeTone
  compactMobile?: boolean
}

const TONE_CLASSES: Record<StatCardTone, string> = {
  'accent-1':
    'from-overview-accent-1/80 via-overview-accent-1/45 to-overview-accent-1/5 dark:from-overview-accent-1/70 dark:via-overview-accent-1/30',
  'accent-2':
    'from-overview-accent-2/80 via-overview-accent-2/45 to-overview-accent-2/5 dark:from-overview-accent-2/70 dark:via-overview-accent-2/30',
  'accent-3':
    'from-overview-accent-3/80 via-overview-accent-3/45 to-overview-accent-3/5 dark:from-overview-accent-3/70 dark:via-overview-accent-3/30',
}

const LINE_TONE_CLASSES: Record<StatCardTone, string> = {
  'accent-1': 'text-tone-teal-strong',
  'accent-2': 'text-tone-violet-strong',
  'accent-3': 'text-tone-blue-strong',
}

const DETAIL_TONE_CLASSES: Record<StatCardDetailTone, string> = {
  default: 'text-foreground',
  muted: 'text-muted-foreground',
  success: 'text-success',
  warning: 'text-warning',
  destructive: 'text-destructive',
}

interface SparklineBucket {
  position: number
  height: number
}

function normalizeSparkline(values?: number[]): SparklineBucket[] {
  if (!values?.length) return []

  const sanitized = values.map((value) => Math.max(0, Number(value) || 0))
  const max = Math.max(...sanitized)
  if (max <= 0) {
    return sanitized.map((_, position) => ({ position, height: 0 }))
  }

  return sanitized.map((value, position) => ({
    position,
    height: Math.max(8, (value / max) * 100),
  }))
}

function buildLineSparkline(values?: number[]) {
  if (!values?.length) return null

  const sanitized = values.map((value) => Math.max(0, Number(value) || 0))
  const width = 160
  const height = 36
  const padding = 3
  const max = Math.max(...sanitized)
  const min = Math.min(...sanitized)
  const range = max - min

  const points = sanitized.map((value, index) => {
    const x =
      sanitized.length === 1
        ? width / 2
        : (index / (sanitized.length - 1)) * width
    let normalized = 0
    if (range > 0) {
      normalized = (value - min) / range
    } else if (max > 0) {
      normalized = 0.5
    }
    const y = height - padding - normalized * (height - padding * 2)

    return { x, y }
  })

  const linePath = points
    .map((point, index) => `${index === 0 ? 'M' : 'L'} ${point.x} ${point.y}`)
    .join(' ')
  const firstPoint = points.at(0)
  const lastPoint = points.at(-1)
  if (!firstPoint || !lastPoint) return null
  const areaPath = `${linePath} L ${lastPoint.x} ${height} L ${firstPoint.x} ${height} Z`

  return {
    areaPath,
    linePath,
  }
}

function LineSparkline(props: { values?: number[]; tone: StatCardTone }) {
  const rawGradientId = useId()
  const gradientId = `stat-card-line-${rawGradientId.replaceAll(':', '')}`
  const paths = buildLineSparkline(props.values)

  if (!paths) return <div className='h-8' aria-hidden='true' />

  return (
    <div
      className={cn(
        'relative h-8 overflow-hidden rounded-lg',
        LINE_TONE_CLASSES[props.tone]
      )}
      aria-hidden='true'
    >
      <svg
        viewBox='0 0 160 36'
        preserveAspectRatio='none'
        className='size-full'
      >
        <defs>
          <linearGradient id={gradientId} x1='0' x2='0' y1='0' y2='1'>
            <stop offset='0%' stopColor='currentColor' stopOpacity='0.24' />
            <stop offset='100%' stopColor='currentColor' stopOpacity='0' />
          </linearGradient>
        </defs>
        <path d={paths.areaPath} fill={`url(#${gradientId})`} />
        <path
          d={paths.linePath}
          fill='none'
          stroke='currentColor'
          strokeLinecap='round'
          strokeLinejoin='round'
          strokeWidth='2.25'
          vectorEffect='non-scaling-stroke'
        />
      </svg>
    </div>
  )
}

function BarSparkline(props: { values?: number[]; tone: StatCardTone }) {
  const sparkline = normalizeSparkline(props.values)

  return (
    <div className='flex h-8 items-end gap-1' aria-hidden='true'>
      {sparkline.map((bucket) => (
        <span
          key={bucket.position}
          className={cn(
            'flex-1 rounded-t-sm bg-linear-to-t',
            bucket.height <= 0 && 'opacity-20',
            TONE_CLASSES[props.tone]
          )}
          style={{ height: `${bucket.height}%` }}
        />
      ))}
    </div>
  )
}

function StatCardDetails(props: { details: StatCardDetail[] }) {
  return (
    <div className='grid grid-cols-2 gap-2'>
      {props.details.map((detail) => (
        <div
          key={detail.label}
          className='bg-muted/40 rounded-lg border border-transparent px-2.5 py-2'
        >
          <div className='text-muted-foreground truncate text-[11px] leading-none font-medium'>
            {detail.label}
          </div>
          <div
            className={cn(
              'mt-1.5 truncate text-xs font-semibold tabular-nums',
              DETAIL_TONE_CLASSES[detail.tone ?? 'default']
            )}
            title={detail.value}
          >
            {detail.value}
          </div>
        </div>
      ))}
    </div>
  )
}

export function StatCard(props: StatCardProps) {
  const Icon = props.icon
  const tone = props.tone ?? 'accent-3'
  const sparklineVariant = props.sparklineVariant ?? 'bars'
  let valueContent: ReactNode
  if (props.loading) {
    valueContent = (
      <div
        className={cn(
          'flex flex-col',
          props.compactMobile ? 'gap-1' : 'gap-1.5'
        )}
      >
        <Skeleton className='h-5 w-16 sm:h-7 sm:w-24' />
        <Skeleton
          className={cn(
            'h-3 w-24 sm:h-3.5 sm:w-32',
            props.compactMobile && 'hidden sm:block'
          )}
        />
      </div>
    )
  } else if (props.error) {
    valueContent = (
      <div className='flex min-w-0 flex-col gap-1'>
        <div className='text-muted-foreground mt-0.5 font-mono text-base font-bold tracking-tight break-all tabular-nums sm:text-2xl'>
          --
        </div>
        <p className='text-muted-foreground text-xs leading-relaxed'>
          {props.description}
        </p>
      </div>
    )
  } else {
    valueContent = (
      <div className='flex min-w-0 flex-col gap-1'>
        <div
          className='text-foreground overflow-x-auto font-mono text-2xl leading-tight font-medium tracking-tight whitespace-nowrap tabular-nums xl:text-3xl'
          title={String(props.value)}
        >
          {props.value}
        </div>
        <p className={cn('text-muted-foreground text-xs leading-relaxed')}>
          {props.description}
        </p>
      </div>
    )
  }

  let visualization: ReactNode
  if (props.details?.length) {
    visualization = <StatCardDetails details={props.details} />
  } else if (sparklineVariant === 'line') {
    visualization = <LineSparkline values={props.sparkline} tone={tone} />
  } else {
    visualization = <BarSparkline values={props.sparkline} tone={tone} />
  }

  return (
    <VStack
      as='article'
      gap={props.compactMobile ? 4 : 6}
      justify='between'
      className='group h-full min-w-0 text-inherit'
      aria-label={props.title}
      aria-busy={Boolean(props.loading)}
    >
      <HStack gap={2} align='start' justify='between' className='text-inherit'>
        <VStack gap={4} className='text-inherit'>
          <Icon
            className={cn('size-6', LINE_TONE_CLASSES[tone])}
            aria-hidden='true'
          />
          <h4 className='text-xs leading-snug font-medium'>{props.title}</h4>
        </VStack>
        {props.action}
      </HStack>
      {valueContent}
      {!props.error && !props.loading && (
        <section className='hidden sm:block' aria-hidden='true'>
          {visualization}
        </section>
      )}
    </VStack>
  )
}
