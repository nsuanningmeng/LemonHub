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
import { BookOpen, Network } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { PageTransition } from '@/components/page-transition'
import { Button } from '@/components/ui/button'

import {
  LoadingSkeleton,
  EmptyState,
  SearchBar,
  PricingTable,
  PricingSidebar,
  PricingToolbar,
  ModelCardGrid,
  ModelDetailsDrawer,
} from './components'
import { CatalogBillingFilter } from './components/catalog-billing-filter'
import { EXCLUDED_GROUPS, VIEW_MODES } from './constants'
import { useFilters } from './hooks/use-filters'
import { usePricingData } from './hooks/use-pricing-data'

export function Pricing() {
  const { t } = useTranslation()
  const [selectedModelName, setSelectedModelName] = useState<string | null>(
    null
  )

  const {
    models,
    vendors,
    groupRatio,
    usableGroup,
    endpointMap,
    autoGroups,
    isLoading,
    error,
    refetch,
    priceRate,
    usdExchangeRate,
  } = usePricingData()

  const {
    searchInput,
    sortBy,
    vendorFilter,
    groupFilter,
    quotaTypeFilter,
    endpointTypeFilter,
    tagFilter,
    tokenUnit,
    viewMode,
    showRechargePrice,
    setSearchInput,
    setSortBy,
    setVendorFilter,
    setGroupFilter,
    setQuotaTypeFilter,
    setEndpointTypeFilter,
    setTagFilter,
    setTokenUnit,
    setViewMode,
    setShowRechargePrice,
    filteredModels,
    hasActiveFilters,
    activeFilterCount,
    availableTags,
    clearFilters,
    clearSearch,
  } = useFilters(models || [])

  const handleModelClick = useCallback((modelName: string) => {
    setSelectedModelName(modelName)
  }, [])

  const selectedModel = useMemo(
    () =>
      selectedModelName
        ? (models || []).find(
            (model) => model.model_name === selectedModelName
          ) || null
        : null,
    [models, selectedModelName]
  )

  const availableGroups = useMemo(
    () =>
      Object.keys(usableGroup || {}).filter(
        (g) => !EXCLUDED_GROUPS.includes(g)
      ),
    [usableGroup]
  )

  const handleClearAll = useCallback(() => {
    clearFilters()
    clearSearch()
  }, [clearFilters, clearSearch])

  const renderPricingContent = () => {
    if (filteredModels.length === 0) {
      return (
        <EmptyState
          searchQuery={searchInput}
          hasActiveFilters={hasActiveFilters}
          onClearFilters={handleClearAll}
        />
      )
    }

    if (viewMode === VIEW_MODES.CARD) {
      return (
        <ModelCardGrid
          models={filteredModels}
          onModelClick={handleModelClick}
          priceRate={priceRate}
          usdExchangeRate={usdExchangeRate}
          tokenUnit={tokenUnit}
          showRechargePrice={showRechargePrice}
          selectedGroup={groupFilter}
        />
      )
    }

    return (
      <PricingTable
        models={filteredModels}
        priceRate={priceRate}
        usdExchangeRate={usdExchangeRate}
        tokenUnit={tokenUnit}
        showRechargePrice={showRechargePrice}
        selectedGroup={groupFilter}
        onModelClick={handleModelClick}
      />
    )
  }

  if (isLoading) {
    return (
      <PublicLayout showMainContainer={false}>
        <main
          id='main-content'
          className='mx-auto w-full max-w-screen-2xl px-5 pt-24 pb-12 sm:px-8 lg:px-12'
        >
          <LoadingSkeleton viewMode={viewMode} />
        </main>
      </PublicLayout>
    )
  }

  if (error && models.length === 0) {
    return (
      <PublicLayout>
        <VStack
          role='alert'
          gap={4}
          hAlign='center'
          className='surface-peach rounded-3xl p-8 text-center'
        >
          <h1 className='text-tone-peach-strong text-2xl font-semibold'>
            {t('Unable to load the model catalog.')}
          </h1>
          <p className='text-tone-peach-strong text-sm'>
            {t('Please try again later.')}
          </p>
          <Button
            onClick={() => {
              void refetch()
            }}
          >
            {t('Retry')}
          </Button>
        </VStack>
      </PublicLayout>
    )
  }

  return (
    <PublicLayout showMainContainer={false}>
      <main id='main-content' className='bg-background'>
        <PageTransition className='mx-auto w-full max-w-screen-2xl px-5 pt-24 pb-12 sm:px-8 lg:px-12'>
          <header className='surface-violet border-tone-violet-border mb-6 grid gap-8 rounded-3xl border p-6 sm:p-8 lg:grid-cols-[minmax(0,1fr)_minmax(16rem,24rem)] lg:items-end lg:p-10'>
            <VStack gap={4}>
              <p className='text-tone-violet-strong flex items-center gap-3 font-mono text-xs tracking-widest uppercase'>
                <BookOpen className='size-5' aria-hidden />
                {t('The model library')}
              </p>
              <h1 className='text-tone-violet-strong max-w-3xl text-4xl leading-tight font-medium tracking-tight sm:text-5xl lg:text-6xl'>
                {t('Different strengths.')}
                <br />
                <em className='text-tone-blue-strong not-italic'>
                  {t('One API.')}
                </em>
              </h1>
              <p className='text-tone-violet-strong max-w-xl text-sm leading-relaxed sm:text-base'>
                {t(
                  'Compare capabilities, understand the cost, and find the right model for your next idea.'
                )}
              </p>
            </VStack>
            <VStack gap={6}>
              <HStack as='dl' gap={3}>
                <VStack
                  gap={3}
                  className='surface-blue min-w-0 flex-1 rounded-2xl p-4'
                >
                  <dt className='text-tone-blue-strong flex items-center gap-2 text-xs'>
                    <BookOpen className='size-4' aria-hidden />
                    {t('Available models')}
                  </dt>
                  <dd className='text-tone-blue-strong text-3xl font-semibold tracking-tight tabular-nums'>
                    {(models?.length || 0).toLocaleString()}
                  </dd>
                </VStack>
                <VStack
                  gap={3}
                  className='surface-teal min-w-0 flex-1 rounded-2xl p-4'
                >
                  <dt className='text-tone-teal-strong flex items-center gap-2 text-xs'>
                    <Network className='size-4' aria-hidden />
                    {t('Providers')}
                  </dt>
                  <dd className='text-tone-teal-strong text-3xl font-semibold tracking-tight tabular-nums'>
                    {(vendors?.length || 0).toLocaleString()}
                  </dd>
                </VStack>
              </HStack>
              <SearchBar
                value={searchInput}
                onChange={setSearchInput}
                onClear={clearSearch}
                placeholder={t(
                  'Search model name, provider, endpoint, or tag...'
                )}
              />
            </VStack>
          </header>

          <section aria-label={t('Browse models')} className='mb-8'>
            <CatalogBillingFilter
              models={models || []}
              value={quotaTypeFilter}
              onChange={setQuotaTypeFilter}
            />
          </section>

          <div className='grid gap-6 xl:grid-cols-[15rem_minmax(0,1fr)] xl:gap-8'>
            <PricingSidebar
              quotaTypeFilter={quotaTypeFilter}
              endpointTypeFilter={endpointTypeFilter}
              vendorFilter={vendorFilter}
              groupFilter={groupFilter}
              tagFilter={tagFilter}
              onQuotaTypeChange={setQuotaTypeFilter}
              onEndpointTypeChange={setEndpointTypeFilter}
              onVendorChange={setVendorFilter}
              onGroupChange={setGroupFilter}
              onTagChange={setTagFilter}
              vendors={vendors || []}
              groups={availableGroups}
              groupRatios={groupRatio}
              tags={availableTags}
              models={models || []}
              hasActiveFilters={hasActiveFilters}
              onClearFilters={clearFilters}
              className='hover-scrollbar sticky top-24 hidden max-h-[calc(100dvh-7rem)] self-start overflow-y-auto xl:block'
            />

            <section
              aria-label={t('Model catalog')}
              className='min-w-0 space-y-6'
            >
              <PricingToolbar
                filteredCount={filteredModels.length}
                totalCount={models?.length}
                sortBy={sortBy}
                onSortChange={setSortBy}
                tokenUnit={tokenUnit}
                onTokenUnitChange={setTokenUnit}
                showRechargePrice={showRechargePrice}
                onRechargePriceChange={setShowRechargePrice}
                viewMode={viewMode}
                onViewModeChange={setViewMode}
                quotaTypeFilter={quotaTypeFilter}
                endpointTypeFilter={endpointTypeFilter}
                vendorFilter={vendorFilter}
                groupFilter={groupFilter}
                tagFilter={tagFilter}
                onQuotaTypeChange={setQuotaTypeFilter}
                onEndpointTypeChange={setEndpointTypeFilter}
                onVendorChange={setVendorFilter}
                onGroupChange={setGroupFilter}
                onTagChange={setTagFilter}
                vendors={vendors || []}
                groups={availableGroups}
                groupRatios={groupRatio}
                tags={availableTags}
                models={models || []}
                hasActiveFilters={hasActiveFilters}
                activeFilterCount={activeFilterCount}
                onClearFilters={clearFilters}
              />

              {renderPricingContent()}
            </section>
          </div>

          {selectedModel && (
            <ModelDetailsDrawer
              open={Boolean(selectedModel)}
              onOpenChange={(open) => {
                if (!open) setSelectedModelName(null)
              }}
              model={selectedModel}
              groupRatio={groupRatio || {}}
              usableGroup={usableGroup || {}}
              endpointMap={
                (endpointMap as Record<
                  string,
                  { path?: string; method?: string }
                >) || {}
              }
              autoGroups={autoGroups || []}
              priceRate={priceRate ?? 1}
              usdExchangeRate={usdExchangeRate ?? 1}
              tokenUnit={tokenUnit}
              showRechargePrice={showRechargePrice}
            />
          )}
        </PageTransition>
      </main>
    </PublicLayout>
  )
}
