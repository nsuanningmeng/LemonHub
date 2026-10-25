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
import { getCurrencyDisplay } from '@/lib/currency'

import type { RewardCampaign } from '../types'

function decimalRatio(
  value: number
): { numerator: bigint; denominator: bigint } | null {
  if (!Number.isFinite(value) || value <= 0) return null
  const [mantissa, exponent = '0'] = value.toString().split('e')
  const [whole, fraction = ''] = mantissa.split('.')
  const numerator = BigInt(whole + fraction)
  const shift = Number(exponent) - fraction.length
  if (shift >= 0) {
    return { numerator: numerator * 10n ** BigInt(shift), denominator: 1n }
  }
  return { numerator, denominator: 10n ** BigInt(-shift) }
}

/** Format aggregate claim totals without narrowing their integer quota to Number. */
export function formatRewardClaimedAmount(
  campaign: Pick<RewardCampaign, 'claimed_quota' | 'claimed_quota_exact'>,
  locale?: Intl.LocalesArgument
): string {
  let quota: bigint
  if (campaign.claimed_quota_exact !== undefined) {
    if (!/^\d+$/.test(campaign.claimed_quota_exact)) return '-'
    quota = BigInt(campaign.claimed_quota_exact)
  } else {
    const legacyQuota = campaign.claimed_quota
    if (
      typeof legacyQuota !== 'number' ||
      !Number.isSafeInteger(legacyQuota) ||
      legacyQuota < 0
    ) {
      return '-'
    }
    quota = BigInt(legacyQuota)
  }

  const { config, meta } = getCurrencyDisplay()
  if (meta.kind === 'tokens') return quota.toString()

  const unit = decimalRatio(config.quotaPerUnit)
  const rate = decimalRatio(meta.exchangeRate)
  if (!unit || !rate) return '-'
  const numerator = quota * rate.numerator * unit.denominator
  const denominator = rate.denominator * unit.numerator
  const digits = numerator >= denominator ? 2 : 4
  const scale = 10n ** BigInt(digits)
  let rounded = (numerator * scale * 2n + denominator) / (denominator * 2n)
  // Match the standard currency display's smallest nonzero amount.
  if (numerator > 0n && rounded === 0n) rounded = 1n
  const integer = rounded / scale
  const fraction = (rounded % scale)
    .toString()
    .padStart(digits, '0')
    .replace(/0+$/, '')
  const parts = new Intl.NumberFormat(locale, {
    ...(meta.kind === 'currency'
      ? ({
          style: 'currency',
          currency: meta.currencyCode,
          currencyDisplay: 'narrowSymbol',
        } as const)
      : {}),
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).formatToParts(integer)
  let result = parts.map((part) => part.value).join('')
  if (fraction) {
    const decimal =
      new Intl.NumberFormat(locale)
        .formatToParts(1.1)
        .find((part) => part.type === 'decimal')?.value ?? '.'
    const localizedFraction = new Intl.NumberFormat(locale, {
      useGrouping: false,
      minimumIntegerDigits: fraction.length,
      maximumFractionDigits: 0,
    }).format(BigInt(fraction))
    const lastInteger = parts.map((part) => part.type).lastIndexOf('integer')
    result = parts
      .map(
        (part, index) =>
          part.value +
          (index === lastInteger ? decimal + localizedFraction : '')
      )
      .join('')
  }
  return meta.kind === 'custom' ? `${meta.symbol} ${result}` : result
}
