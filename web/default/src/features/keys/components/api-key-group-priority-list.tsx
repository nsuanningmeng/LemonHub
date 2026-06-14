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
import { useMemo, useState } from 'react'
import { ArrowDown, ArrowUp, ChevronsUpDown, Plus, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  GroupRatioBadge,
  type ApiKeyGroupOption,
} from './api-key-group-combobox'

type ApiKeyGroupPriorityListProps = {
  options: ApiKeyGroupOption[]
  value: string[]
  onChange: (value: string[]) => void
  placeholder?: string
  disabled?: boolean
}

/**
 * ApiKeyGroupPriorityList — 令牌「多分组 + 优先级」有序选择器。
 *
 * - 选中的分组以有序列表呈现，序号 1 为最高优先级（从上到下递减）。
 * - 通过 ↑/↓ 调整优先级，✕ 移除；底部下拉添加新分组（追加到末尾）。
 * - 请求报错时后端会按此顺序自动切换到下一优先级分组重试。
 */
export function ApiKeyGroupPriorityList({
  options,
  value,
  onChange,
  placeholder,
  disabled,
}: ApiKeyGroupPriorityListProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [searchValue, setSearchValue] = useState('')

  const optionMap = useMemo(() => {
    const map = new Map<string, ApiKeyGroupOption>()
    for (const option of options) {
      map.set(option.value, option)
    }
    return map
  }, [options])

  const availableOptions = useMemo(() => {
    const selected = new Set(value)
    const search = searchValue.trim().toLowerCase()
    return options
      .filter((option) => !selected.has(option.value))
      .filter((option) => {
        if (!search) return true
        const ratioText = String(option.ratio ?? '').toLowerCase()
        return (
          option.value.toLowerCase().includes(search) ||
          option.label.toLowerCase().includes(search) ||
          option.desc?.toLowerCase().includes(search) ||
          ratioText.includes(search)
        )
      })
  }, [options, value, searchValue])

  const addGroup = (groupValue: string) => {
    if (value.includes(groupValue)) return
    onChange([...value, groupValue])
    setSearchValue('')
    setOpen(false)
  }

  const removeGroup = (groupValue: string) => {
    onChange(value.filter((item) => item !== groupValue))
  }

  const moveGroup = (index: number, direction: -1 | 1) => {
    const target = index + direction
    if (target < 0 || target >= value.length) return
    const next = [...value]
    const tmp = next[index]
    next[index] = next[target]
    next[target] = tmp
    onChange(next)
  }

  return (
    <div className='space-y-2'>
      {value.length > 0 && (
        <ul className='space-y-2'>
          {value.map((groupValue, index) => {
            const option = optionMap.get(groupValue)
            return (
              <li
                key={groupValue}
                className='border-input bg-muted/30 flex items-center gap-2 rounded-lg border px-3 py-2'
              >
                <span
                  className='bg-primary/10 text-primary flex h-6 w-6 shrink-0 items-center justify-center rounded-md text-xs font-semibold'
                  title={t('Priority')}
                >
                  {index + 1}
                </span>
                <span className='min-w-0 flex-1'>
                  <span className='block truncate font-medium'>
                    {option?.label ?? groupValue}
                  </span>
                  {option?.desc && (
                    <span className='text-muted-foreground block truncate text-[11px] sm:text-xs'>
                      {option.desc}
                    </span>
                  )}
                </span>
                <span className='hidden sm:block'>
                  <GroupRatioBadge ratio={option?.ratio} />
                </span>
                <Button
                  type='button'
                  variant='ghost'
                  size='icon-sm'
                  disabled={disabled || index === 0}
                  onClick={() => moveGroup(index, -1)}
                  aria-label={t('Move up')}
                >
                  <ArrowUp className='h-4 w-4' />
                </Button>
                <Button
                  type='button'
                  variant='ghost'
                  size='icon-sm'
                  disabled={disabled || index === value.length - 1}
                  onClick={() => moveGroup(index, 1)}
                  aria-label={t('Move down')}
                >
                  <ArrowDown className='h-4 w-4' />
                </Button>
                <Button
                  type='button'
                  variant='ghost'
                  size='icon-sm'
                  className='text-muted-foreground hover:text-destructive'
                  disabled={disabled}
                  onClick={() => removeGroup(groupValue)}
                  aria-label={t('Remove')}
                >
                  <X className='h-4 w-4' />
                </Button>
              </li>
            )
          })}
        </ul>
      )}

      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger
          render={
            <Button
              type='button'
              variant='outline'
              role='combobox'
              aria-expanded={open}
              disabled={disabled || availableOptions.length === 0}
              className='border-input bg-muted/40 hover:bg-muted/55 w-full justify-between gap-2 rounded-lg px-3 py-2 text-start shadow-none'
            />
          }
        >
          <span className='text-muted-foreground flex min-w-0 items-center gap-1.5 truncate'>
            <Plus className='h-4 w-4 shrink-0' />
            {value.length === 0
              ? placeholder || t('Select groups (top = highest priority)')
              : t('Add another group')}
          </span>
          <ChevronsUpDown className='h-4 w-4 shrink-0 opacity-50' />
        </PopoverTrigger>
        <PopoverContent
          className='w-[var(--anchor-width)] overflow-hidden rounded-xl p-0 shadow-lg'
          onWheel={(event) => event.stopPropagation()}
          onTouchMove={(event) => event.stopPropagation()}
          onPointerDown={(event) => event.stopPropagation()}
        >
          <Command shouldFilter={false}>
            <CommandInput
              placeholder={t('Search...')}
              value={searchValue}
              onValueChange={setSearchValue}
            />
            <CommandList className='max-h-[320px]'>
              <CommandEmpty>{t('No group found.')}</CommandEmpty>
              <CommandGroup>
                {availableOptions.map((option) => (
                  <CommandItem
                    key={option.value}
                    value={option.value}
                    onSelect={() => addGroup(option.value)}
                    className='data-[selected=true]:bg-muted items-start gap-3 rounded-lg px-3 py-3 transition-colors'
                  >
                    <span className='min-w-0 flex-1'>
                      <span className='block truncate font-medium'>
                        {option.label}
                      </span>
                      {option.desc && (
                        <span className='text-muted-foreground block truncate text-xs'>
                          {option.desc}
                        </span>
                      )}
                    </span>
                    <GroupRatioBadge ratio={option.ratio} />
                  </CommandItem>
                ))}
              </CommandGroup>
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
    </div>
  )
}
