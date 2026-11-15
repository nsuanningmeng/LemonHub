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
import type { Table } from '@tanstack/react-table'
import { ChevronDown, Loader2, Search, X as Cross2Icon } from 'lucide-react'
import * as React from 'react'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useDebounce } from '@/hooks'
import { cn } from '@/lib/utils'

import { DataTableFacetedFilter } from './faceted-filter'
import { DataTableViewOptions } from './view-options'

type FilterDef = {
  columnId: string
  title: string
  options: {
    label: string
    value: string
    icon?: React.ComponentType<{ className?: string }>
    iconNode?: React.ReactNode
    count?: number
  }[]
  singleSelect?: boolean
}

type SearchDraft = {
  baseValue: string
  value: string
}

export type DataTableToolbarProps<TData> = {
  table: Table<TData>
  /**
   * Placeholder for the default search input. Defaults to `t('Filter...')`.
   */
  searchPlaceholder?: string
  /**
   * Delay committing the default search input. Defaults to immediate updates.
   */
  searchDebounceMs?: number
  /**
   * Column id to filter on. When provided, the search input filters
   * a specific column. When omitted, the search input updates the
   * table's `globalFilter`.
   */
  searchKey?: string
  /**
   * Column-level filter chips (faceted multi-select / single-select).
   */
  filters?: FilterDef[]
  /**
   * Replaces the default search input entirely. Use when the primary
   * "search" is something custom — e.g. a date-time range picker.
   */
  customSearch?: ReactNode
  /**
   * Extra inputs/selects displayed in the primary row alongside the
   * search input and filter chips.
   */
  additionalSearch?: ReactNode
  /**
   * Whether non-table filters (e.g. `additionalSearch` or `expandable`
   * inputs) are currently active. Controls Reset button visibility
   * when no column filters are set.
   */
  hasAdditionalFilters?: boolean
  /**
   * Callback invoked when the user clicks Reset.
   */
  onReset?: () => void
  /**
   * Additional filter inputs hidden behind an Expand/Collapse toggle.
   * Inputs flow inline with the primary row when expanded.
   */
  expandable?: ReactNode
  /**
   * When `expandable` is collapsed, highlights the toggle if any of
   * the expandable inputs currently hold a value.
   */
  hasExpandedActiveFilters?: boolean
  /**
   * Custom action buttons rendered BEFORE the built-in
   * Reset / Search / View buttons.
   */
  preActions?: ReactNode
  /**
   * Explicit "Search" / "Apply" callback. When provided the toolbar
   * shows a primary Search button. Filters are committed only on click
   * (form-mode workflow).
   */
  onSearch?: () => void
  /**
   * Loading state for the explicit Search button.
   */
  searchLoading?: boolean
  /**
   * Hide the View Options (column visibility) dropdown.
   */
  hideViewOptions?: boolean
  /**
   * Optional view-mode toggle (e.g. table vs. card) rendered in the right
   * action cluster, before the View Options dropdown. Typically a
   * {@link DataTableViewModeToggle}. Omitted by default.
   */
  viewToggle?: ReactNode
  /**
   * Secondary actions rendered below the search rail and advanced filters.
   * Display controls and search actions remain beside the primary filters.
   */
  leftActions?: ReactNode
  /**
   * Outer wrapper className override.
   */
  className?: string
}

/** Search, filters and display controls share a persistent record-workspace rail. */
export function DataTableToolbar<TData>(props: DataTableToolbarProps<TData>) {
  const { t } = useTranslation()
  const expandedFiltersId = React.useId()
  const [expanded, setExpanded] = useState(false)
  const [isSearchComposing, setIsSearchComposing] = useState(false)

  const filters = props.filters ?? []
  const hasExpandable = props.expandable != null
  const hasSearch = props.onSearch != null

  const isFiltered =
    props.table.getState().columnFilters.length > 0 ||
    !!props.table.getState().globalFilter ||
    !!props.hasAdditionalFilters

  const placeholder = props.searchPlaceholder ?? t('Filter...')
  const currentSearchValue = props.searchKey
    ? ((props.table.getColumn(props.searchKey)?.getFilterValue() as string) ??
      '')
    : ((props.table.getState().globalFilter as string | undefined) ?? '')

  const [searchDraft, setSearchDraft] = useState<SearchDraft | null>(null)
  const activeSearchDraft =
    searchDraft &&
    (isSearchComposing || searchDraft.baseValue === currentSearchValue)
      ? searchDraft
      : null
  const searchValue = activeSearchDraft?.value ?? currentSearchValue
  const searchDebounceMs = Math.max(0, props.searchDebounceMs ?? 0)
  const debouncedSearchValue = useDebounce(searchValue, searchDebounceMs)

  const commitSearchValue = React.useCallback(
    (value: string) => {
      if (value === currentSearchValue) {
        return
      }

      if (props.searchKey) {
        props.table.getColumn(props.searchKey)?.setFilterValue(value)
        return
      }

      props.table.setGlobalFilter(value)
    },
    [currentSearchValue, props.searchKey, props.table]
  )

  React.useEffect(() => {
    if (
      searchDebounceMs <= 0 ||
      isSearchComposing ||
      debouncedSearchValue !== searchValue
    ) {
      return
    }

    commitSearchValue(debouncedSearchValue)
  }, [
    commitSearchValue,
    debouncedSearchValue,
    isSearchComposing,
    searchDebounceMs,
    searchValue,
  ])

  const queueSearchValue = (value: string) => {
    if (searchDebounceMs <= 0) {
      commitSearchValue(value)
    }
  }

  const handleSearchChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const value = event.target.value
    setSearchDraft({ baseValue: currentSearchValue, value })

    if (!isSearchComposing) {
      queueSearchValue(value)
    }
  }

  const handleSearchCompositionStart = () => {
    setIsSearchComposing(true)
  }

  const handleSearchCompositionEnd = (
    event: React.CompositionEvent<HTMLInputElement>
  ) => {
    setIsSearchComposing(false)
    const value = event.currentTarget.value
    setSearchDraft({ baseValue: currentSearchValue, value })
    queueSearchValue(value)
  }

  const searchInput = (
    <label className='relative block min-w-0 flex-1 basis-full sm:max-w-72 sm:min-w-48 sm:basis-auto'>
      <Search
        aria-hidden='true'
        className='text-tone-blue-strong pointer-events-none absolute top-1/2 left-3 z-10 size-4 -translate-y-1/2'
      />
      <Input
        aria-label={placeholder}
        placeholder={placeholder}
        value={searchValue}
        onChange={handleSearchChange}
        onCompositionStart={handleSearchCompositionStart}
        onCompositionEnd={handleSearchCompositionEnd}
        className='bg-card border-tone-blue-border w-full ps-9 shadow-none'
      />
    </label>
  )

  const filterChips = React.useMemo(
    () =>
      filters.map((filter) => {
        const column = props.table.getColumn(filter.columnId)
        if (!column) return null
        return (
          <DataTableFacetedFilter
            key={filter.columnId}
            column={column}
            title={filter.title}
            options={filter.options}
            singleSelect={filter.singleSelect}
          />
        )
      }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [props.filters, props.table]
  )

  const handleReset = () => {
    setIsSearchComposing(false)
    setSearchDraft(null)
    props.table.resetColumnFilters()
    props.table.setGlobalFilter('')
    props.onReset?.()
  }

  // Reset: outline text-only for form mode (always visible, disabled when
  // nothing to reset); ghost text + X for filter-as-you-type mode (only
  // visible when active filters exist).
  let resetButton: ReactNode = null
  if (hasSearch) {
    resetButton = (
      <Button variant='outline' onClick={handleReset} disabled={!isFiltered}>
        {t('Reset')}
      </Button>
    )
  } else if (isFiltered) {
    resetButton = (
      <Button
        variant='ghost'
        onClick={handleReset}
        className='text-muted-foreground hover:text-foreground gap-1 px-2'
      >
        {t('Reset')}
        <Cross2Icon />
      </Button>
    )
  }

  const searchButton = hasSearch ? (
    <Button
      className='surface-brand bg-(--studio-brand) text-(--studio-on-brand) hover:bg-(--studio-brand) hover:brightness-110'
      onClick={props.onSearch}
      disabled={props.searchLoading}
    >
      {props.searchLoading && <Loader2 className='animate-spin' />}
      {t('Search')}
    </Button>
  ) : null

  const viewOptionsNode = !props.hideViewOptions ? (
    <DataTableViewOptions table={props.table} />
  ) : null

  const viewToggleNode = props.viewToggle ?? null

  const expandToggle = hasExpandable ? (
    <Button
      variant='ghost'
      onClick={() => setExpanded((p) => !p)}
      aria-expanded={expanded}
      aria-controls={expandedFiltersId}
      className={cn(
        'text-muted-foreground hover:text-foreground gap-1 px-2',
        props.hasExpandedActiveFilters &&
          !expanded &&
          'text-primary hover:text-primary'
      )}
    >
      {expanded ? t('Collapse') : t('Expand')}
      <ChevronDown
        className={cn(
          'size-3.5 transition-transform duration-200',
          expanded && 'rotate-180'
        )}
      />
    </Button>
  ) : null

  return (
    <VStack
      as='section'
      gap={3}
      role='search'
      aria-label={t('Filter records')}
      className={cn('min-w-0', props.className)}
    >
      <HStack gap={2} wrap='wrap' vAlign='center'>
        {props.customSearch !== undefined ? props.customSearch : searchInput}
        {props.additionalSearch}
        {filterChips}
        <HStack
          gap={1.5}
          wrap='wrap'
          vAlign='center'
          hAlign='end'
          className='ms-auto max-w-full'
        >
          {props.preActions}
          {resetButton}
          {searchButton}
          {expandToggle}
          {viewToggleNode}
          {viewOptionsNode}
        </HStack>
      </HStack>
      {hasExpandable && (
        <section
          id={expandedFiltersId}
          hidden={!expanded}
          className='border-tone-blue-border border-t pt-3'
        >
          <HStack gap={3} wrap='wrap' vAlign='center'>
            {props.expandable}
          </HStack>
        </section>
      )}
      {props.leftActions != null && (
        <HStack
          gap={2}
          wrap='wrap'
          vAlign='center'
          className='border-tone-blue-border border-t pt-3'
        >
          {props.leftActions}
        </HStack>
      )}
    </VStack>
  )
}
