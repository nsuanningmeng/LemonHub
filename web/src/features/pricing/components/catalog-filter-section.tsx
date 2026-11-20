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
import { ChevronDown } from 'lucide-react'
import type { ReactNode } from 'react'

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { cn } from '@/lib/utils'

import { FILTER_ALL } from '../constants'

export type FilterOption = {
  value: string
  label: string
  count?: number
  suffix?: string
  icon?: ReactNode
}

type FilterSectionProps = {
  defaultOpen?: boolean
  title: string
  value: string
  options: FilterOption[]
  onChange: (value: string) => void
}

function FilterChip(props: {
  option: FilterOption
  active: boolean
  onClick: () => void
}) {
  return (
    <button
      type='button'
      onClick={props.onClick}
      aria-pressed={props.active}
      className={cn(
        'group inline-flex min-h-7 max-w-full items-center gap-1.5 rounded-md border px-2 py-1 text-xs font-medium transition-colors',
        props.active
          ? 'border-primary bg-primary text-primary-foreground'
          : 'border-transparent text-muted-foreground hover:bg-muted hover:text-foreground'
      )}
      title={props.option.label}
    >
      {props.option.icon && (
        <span className='shrink-0'>{props.option.icon}</span>
      )}
      <span className='truncate'>{props.option.label}</span>
      {(props.option.suffix || props.option.count != null) && (
        <span
          className={cn(
            'rounded-sm px-1 font-mono tabular-nums',
            props.active
              ? 'bg-primary-foreground/15 text-primary-foreground'
              : 'bg-muted text-muted-foreground'
          )}
        >
          {props.option.suffix ?? props.option.count}
        </span>
      )}
    </button>
  )
}

export function FilterSection(props: FilterSectionProps) {
  const selectedOption =
    props.value !== FILTER_ALL &&
    props.options.find((option) => option.value === props.value)
  return (
    <Collapsible
      defaultOpen={props.defaultOpen ?? Boolean(selectedOption)}
      className='border-border/70 border-b pb-3 last:border-b-0'
    >
      <CollapsibleTrigger
        aria-label={props.title}
        className='group flex w-full items-center justify-between py-2.5 text-left'
      >
        <span className='text-foreground text-sm font-semibold'>
          {props.title}
          {selectedOption && (
            <small className='text-muted-foreground ms-2 font-normal'>
              {selectedOption.label}
            </small>
          )}
        </span>
        <ChevronDown className='text-muted-foreground size-4 transition-transform group-data-[panel-open]:rotate-180' />
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div
          role='group'
          aria-label={props.title}
          className='flex flex-wrap gap-1.5'
        >
          {props.options.map((option) => (
            <FilterChip
              key={option.value}
              option={option}
              active={props.value === option.value}
              onClick={() => props.onChange(option.value)}
            />
          ))}
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}
