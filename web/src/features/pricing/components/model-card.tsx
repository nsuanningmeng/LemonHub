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

import { StatusBadgeTypeContext } from '@/components/status-badge'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { getLobeIcon } from '@/lib/lobe-icon'

import { DEFAULT_TOKEN_UNIT, FILTER_ALL, QUOTA_TYPES } from '../constants'
import {
  getDynamicDisplayGroupRatio,
  getDynamicPricingSummary,
  getExpressionBillingLabel,
} from '../lib/dynamic-price'
import { getModelQuotaType, parseTags } from '../lib/filters'
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
  const showsTokenUnit = getModelQuotaType(props.model) === QUOTA_TYPES.TOKEN
  const tokenUnitLabel = tokenUnit === 'K' ? '1K' : '1M'
  const tags = parseTags(props.model.tags)
  const groups = props.model.enable_groups || []
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

  const primaryGroup =
    props.selectedGroup &&
    props.selectedGroup !== FILTER_ALL &&
    groups.includes(props.selectedGroup)
      ? props.selectedGroup
      : undefined
  const bottomTags = tags.slice(0, 2)
  const hiddenCount = Math.max(tags.length - 2, 0)

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
        <p className='text-muted-foreground text-sm'>
          {t('Special billing expression')}
        </p>
      )
    } else if (visibleEntries.length > 0) {
      priceSummary = visibleEntries.map((entry) => (
        <PriceValue
          key={entry.key}
          label={t(entry.shortLabel)}
          value={entry.formatted}
        />
      ))
    } else {
      priceSummary = (
        <p className='text-muted-foreground text-sm'>
          {t(getExpressionBillingLabel(props.model.billing_expr || ''))}
        </p>
      )
    }
  } else if (isTokenBased) {
    priceSummary = (
      <>
        <PriceValue
          label={t('Input')}
          value={formatPrice(
            props.model,
            'input',
            tokenUnit,
            showRechargePrice,
            priceRate,
            usdExchangeRate,
            props.selectedGroup
          )}
        />
        <PriceValue
          label={t('Output')}
          value={formatPrice(
            props.model,
            'output',
            tokenUnit,
            showRechargePrice,
            priceRate,
            usdExchangeRate,
            props.selectedGroup
          )}
        />
        {hasCachedPrice && (
          <PriceValue
            label={t('Cached')}
            value={formatPrice(
              props.model,
              'cache',
              tokenUnit,
              showRechargePrice,
              priceRate,
              usdExchangeRate,
              props.selectedGroup
            )}
          />
        )}
      </>
    )
  } else {
    priceSummary = (
      <p className='text-muted-foreground whitespace-nowrap'>
        <strong className='text-foreground font-mono text-base font-medium'>
          {formatRequestPrice(
            props.model,
            showRechargePrice,
            priceRate,
            usdExchangeRate,
            props.selectedGroup
          )}
        </strong>{' '}
        / {t('request')}
      </p>
    )
  }

  return (
    <VStack
      as='article'
      gap={3}
      aria-label={props.model.model_name}
      className='group border-border/70 hover:bg-muted/20 min-w-0 border-b py-5 transition-colors'
    >
      <HStack gap={3} hAlign='between' vAlign='start'>
        <HStack gap={3} vAlign='start' className='min-w-0'>
          <HStack hAlign='center' vAlign='center' className='size-8 shrink-0'>
            {modelIcon || (
              <b className='text-muted-foreground text-base'>{initial}</b>
            )}
          </HStack>
          <VStack gap={1} className='min-w-0'>
            <h3 className='text-foreground text-base leading-snug font-semibold [overflow-wrap:anywhere]'>
              {props.model.model_name}
            </h3>
            {props.model.vendor_name && (
              <p className='text-muted-foreground text-xs'>
                {props.model.vendor_name}
              </p>
            )}
          </VStack>
        </HStack>
        <button
          type='button'
          onClick={handleCopy}
          className='text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:ring-ring shrink-0 rounded-md p-2 transition-colors focus-visible:ring-2 focus-visible:outline-none'
          aria-label={t('Copy model name')}
          title={t('Copy')}
        >
          <Copy className='size-4' aria-hidden />
        </button>
      </HStack>

      {props.model.description && (
        <p className='text-muted-foreground line-clamp-2 text-sm leading-relaxed'>
          {props.model.description}
        </p>
      )}

      <HStack gap={4} wrap='wrap' vAlign='end' className='text-xs'>
        {priceSummary}
        {showsTokenUnit && (
          <small className='text-muted-foreground'>/ {tokenUnitLabel}</small>
        )}
      </HStack>

      <HStack
        gap={3}
        wrap='wrap'
        className='text-muted-foreground min-w-0 text-xs'
      >
        <StatusBadgeTypeContext value='text'>
          <ModelBillingModeBadge
            model={props.model}
            className='text-muted-foreground text-xs'
          />
        </StatusBadgeTypeContext>
        {primaryGroup && (
          <p className='max-w-full truncate' title={primaryGroup}>
            {primaryGroup}
          </p>
        )}
        {bottomTags.map((item) => (
          <p key={item} className='max-w-full truncate' title={item}>
            {item}
          </p>
        ))}
        {hiddenCount > 0 && <p>+{hiddenCount}</p>}
      </HStack>

      <HStack
        gap={3}
        hAlign='between'
        vAlign='center'
        wrap='wrap'
        className='mt-auto'
      >
        <ModelPerfBadge perf={props.perf} status={props.perfStatus} />
        <button
          type='button'
          onClick={props.onClick}
          className='text-foreground hover:text-primary focus-visible:ring-ring ml-auto inline-flex items-center gap-1 rounded-md py-1 text-xs font-medium transition-colors focus-visible:ring-2 focus-visible:outline-none'
        >
          {t('Details')}
          <ChevronRight className='size-3.5' aria-hidden />
        </button>
      </HStack>
    </VStack>
  )
})

function PriceValue({ label, value }: { label: string; value: string }) {
  return (
    <p className='text-muted-foreground whitespace-nowrap'>
      {label}{' '}
      <strong className='text-foreground font-mono text-base font-medium'>
        {value}
      </strong>
    </p>
  )
}
