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
import { ChevronRight, Copy } from 'lucide-react'
import { memo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { getLobeIcon } from '@/lib/lobe-icon'

import { DEFAULT_TOKEN_UNIT } from '../constants'
import {
  getDynamicDisplayGroupRatio,
  getDynamicPricingSummary,
  getExpressionBillingLabel,
} from '../lib/dynamic-price'
import { parseTags } from '../lib/filters'
import { isTokenBasedModel } from '../lib/model-helpers'
import { formatPrice, formatRequestPrice } from '../lib/price'
import type { PricingModel, TokenUnit } from '../types'
import { ModelBillingModeBadge } from './model-billing-mode-badge'
import {
  ModelPerfBadge,
  type ModelPerfBadgeData,
  type ModelPerfBadgeProps,
} from './model-perf-badge'

export interface ModelCardProps {
  model: PricingModel
  onClick: () => void
  priceRate?: number
  usdExchangeRate?: number
  tokenUnit?: TokenUnit
  showRechargePrice?: boolean
  selectedGroup?: string
  perf?: ModelPerfBadgeData
  perfStatus?: ModelPerfBadgeProps['status']
}

export const ModelCard = memo(function ModelCard(props: ModelCardProps) {
  const { t } = useTranslation()
  const { copyToClipboard } = useCopyToClipboard()
  const tokenUnit = props.tokenUnit ?? DEFAULT_TOKEN_UNIT
  const priceRate = props.priceRate ?? 1
  const usdExchangeRate = props.usdExchangeRate ?? 1
  const showRechargePrice = props.showRechargePrice ?? false
  const isTokenBased = isTokenBasedModel(props.model)
  const tokenUnitLabel = tokenUnit === 'K' ? '1K' : '1M'
  const tags = parseTags(props.model.tags)
  const groups = props.model.enable_groups || []
  const endpoints = props.model.supported_endpoint_types || []
  const modelIconKey = props.model.icon || props.model.vendor_icon
  const modelIcon = modelIconKey ? getLobeIcon(modelIconKey, 28) : null
  const initial = props.model.model_name?.charAt(0).toUpperCase() || '?'
  const isDynamicPricing =
    props.model.billing_mode === 'tiered_expr' &&
    Boolean(props.model.billing_expr)
  const hasCachedPrice = isTokenBased && props.model.cache_ratio != null
  const dynamicSummary = isDynamicPricing
    ? getDynamicPricingSummary(props.model, {
        tokenUnit,
        showRechargePrice,
        priceRate,
        usdExchangeRate,
        groupRatioMultiplier: getDynamicDisplayGroupRatio(
          props.model,
          props.selectedGroup
        ),
      })
    : null

  const primaryGroup = groups[0]
  const bottomTags = [...endpoints.slice(0, 2), ...tags.slice(0, 2)]
  const hiddenCount =
    Math.max(groups.length - 1, 0) +
    Math.max(endpoints.length - 2, 0) +
    Math.max(tags.length - 2, 0)

  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation()
    copyToClipboard(props.model.model_name || '')
  }

  let priceSummary: ReactNode
  if (dynamicSummary) {
    const visibleEntries = [
      ...dynamicSummary.primaryEntries,
      ...dynamicSummary.secondaryEntries.filter(
        (entry) => entry.field === 'cacheReadPrice'
      ),
    ]
    if (dynamicSummary.isSpecialExpression) {
      priceSummary = (
        <span className='text-amber-700 dark:text-amber-300'>
          {t('Special billing expression')}
        </span>
      )
    } else if (visibleEntries.length > 0) {
      priceSummary = (
        <>
          {visibleEntries.map((entry) => (
            <span
              key={entry.key}
              className='text-muted-foreground whitespace-nowrap'
            >
              {t(entry.shortLabel)}{' '}
              <span className='text-foreground font-mono font-semibold'>
                {entry.formatted}
              </span>
            </span>
          ))}
        </>
      )
    } else {
      priceSummary = (
        <span className='text-muted-foreground text-sm'>
          {t(getExpressionBillingLabel(props.model.billing_expr || ''))}
        </span>
      )
    }
  } else if (isTokenBased) {
    priceSummary = (
      <>
        <span className='text-muted-foreground whitespace-nowrap'>
          {t('Input')}{' '}
          <span className='text-foreground font-mono font-semibold'>
            {formatPrice(
              props.model,
              'input',
              tokenUnit,
              showRechargePrice,
              priceRate,
              usdExchangeRate,
              props.selectedGroup
            )}
          </span>
        </span>
        <span className='text-muted-foreground whitespace-nowrap'>
          {t('Output')}{' '}
          <span className='text-foreground font-mono font-semibold'>
            {formatPrice(
              props.model,
              'output',
              tokenUnit,
              showRechargePrice,
              priceRate,
              usdExchangeRate,
              props.selectedGroup
            )}
          </span>
        </span>
        {hasCachedPrice && (
          <span className='text-muted-foreground whitespace-nowrap'>
            {t('Cached')}{' '}
            <span className='text-foreground font-mono font-semibold'>
              {formatPrice(
                props.model,
                'cache',
                tokenUnit,
                showRechargePrice,
                priceRate,
                usdExchangeRate,
                props.selectedGroup
              )}
            </span>
          </span>
        )}
      </>
    )
  } else {
    priceSummary = (
      <span className='text-muted-foreground whitespace-nowrap'>
        <span className='text-foreground font-mono font-semibold'>
          {formatRequestPrice(
            props.model,
            showRechargePrice,
            priceRate,
            usdExchangeRate,
            props.selectedGroup
          )}
        </span>{' '}
        / {t('request')}
      </span>
    )
  }

  return (
    <VStack
      as='article'
      gap={4}
      aria-label={props.model.model_name}
      className='surface-violet group border-tone-violet-border hover:border-primary/50 min-w-0 overflow-hidden rounded-2xl border p-5 transition-colors sm:p-6'
    >
      <HStack gap={3} hAlign='between' vAlign='start'>
        <HStack gap={3} vAlign='start' className='min-w-0'>
          <HStack
            hAlign='center'
            vAlign='center'
            className='surface-blue border-tone-blue-border size-11 shrink-0 rounded-2xl border'
          >
            {modelIcon || (
              <b className='text-tone-blue-strong text-sm'>{initial}</b>
            )}
          </HStack>
          <VStack gap={1} className='min-w-0'>
            {props.model.vendor_name && (
              <p className='text-tone-violet-strong text-xs'>
                {props.model.vendor_name}
              </p>
            )}
            <h3 className='text-tone-violet-strong text-base leading-snug font-semibold [overflow-wrap:anywhere]'>
              {props.model.model_name}
            </h3>
          </VStack>
        </HStack>
        <button
          type='button'
          onClick={handleCopy}
          className='text-tone-violet-strong hover:bg-tone-violet focus-visible:ring-ring shrink-0 rounded-md p-2 transition-colors focus-visible:ring-2 focus-visible:outline-none'
          aria-label={t('Copy model name')}
          title={t('Copy')}
        >
          <Copy className='size-4' aria-hidden />
        </button>
      </HStack>

      <p className='text-tone-violet-strong line-clamp-2 flex-1 text-sm leading-relaxed'>
        {props.model.description || t('No description available.')}
      </p>

      <HStack gap={2} wrap='wrap' className='min-w-0'>
        {primaryGroup && (
          <p className='text-muted-foreground text-xs font-medium'>
            {primaryGroup}
          </p>
        )}
        <ModelBillingModeBadge model={props.model} />
        {bottomTags.map((item) => (
          <p key={item} className='surface-teal rounded-md px-2 py-1 text-xs'>
            {item}
          </p>
        ))}
        {hiddenCount > 0 && (
          <p className='text-muted-foreground text-xs'>+{hiddenCount}</p>
        )}
      </HStack>

      <VStack
        gap={3}
        className='border-tone-violet-border bg-card rounded-xl border p-4'
      >
        <HStack gap={3} wrap='wrap' vAlign='center' className='text-xs'>
          {priceSummary}
          {isTokenBased && (
            <small className='text-muted-foreground'>/ {tokenUnitLabel}</small>
          )}
        </HStack>
        <HStack gap={3} hAlign='between' vAlign='center' wrap='wrap'>
          <ModelPerfBadge perf={props.perf} status={props.perfStatus} />
          <button
            type='button'
            onClick={props.onClick}
            className='surface-blue text-tone-blue-strong focus-visible:ring-ring inline-flex items-center gap-1 rounded-lg px-3 py-2 text-xs font-semibold transition-opacity hover:opacity-80 focus-visible:ring-2 focus-visible:outline-none'
          >
            {t('Details')}
            <ChevronRight className='size-3.5' aria-hidden />
          </button>
        </HStack>
      </VStack>
    </VStack>
  )
})
