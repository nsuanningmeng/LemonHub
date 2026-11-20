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
import { ArrowUpRight, Copy } from 'lucide-react'
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
    <article className='model-directory-row'>
      <div className='model-directory-identity'>
        <span className='model-directory-icon' aria-hidden='true'>
          {modelIcon || initial}
        </span>
        <div className='min-w-0'>
          <h2>
            <button type='button' onClick={props.onClick} title={t('Details')}>
              {props.model.model_name}
              <ArrowUpRight
                aria-hidden='true'
                className='ms-1 inline size-3.5 align-baseline'
              />
            </button>
          </h2>
          <div className='model-directory-labels'>
            {props.model.vendor_name && <span>{props.model.vendor_name}</span>}
            <ModelBillingModeBadge model={props.model} />
            {primaryGroup && <span>{primaryGroup}</span>}
          </div>
          {props.model.description && (
            <p className='model-directory-description'>
              {props.model.description}
            </p>
          )}
          {(bottomTags.length > 0 || hiddenCount > 0) && (
            <div className='model-directory-tags'>
              {bottomTags.map((item) => (
                <span key={item}>{item}</span>
              ))}
              {hiddenCount > 0 && (
                <span title={t('Details')}>+{hiddenCount}</span>
              )}
            </div>
          )}
        </div>
      </div>
      <div className='model-directory-prices'>
        <div
          className='model-directory-amounts'
          data-token-based={isTokenBased || undefined}
        >
          {priceSummary}
        </div>
        {isTokenBased && (
          <small>
            {t('Prices shown per {{unit}} tokens', { unit: tokenUnitLabel })}
          </small>
        )}
      </div>
      <ModelPerfBadge
        perf={props.perf}
        status={props.perfStatus}
        className='model-directory-perf'
      />
      <button
        type='button'
        onClick={handleCopy}
        className='model-directory-copy'
        title={t('Copy model name')}
        aria-label={t('Copy model name')}
      >
        <Copy aria-hidden='true' className='size-4' />
      </button>
    </article>
  )
})
