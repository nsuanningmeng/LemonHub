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
export const PRICE_REVIEW_FIELDS = [
  ['BillingMode', 'Billing mode'],
  ['ModelPrice', 'Model fixed pricing'],
  ['ModelRatio', 'Model ratio'],
  ['CompletionRatio', 'Completion ratio'],
  ['CacheRatio', 'Prompt cache ratio'],
  ['CreateCacheRatio', 'Create cache ratio'],
  ['ImageRatio', 'Image ratio'],
  ['AudioRatio', 'Audio ratio'],
  ['AudioCompletionRatio', 'Audio completion ratio'],
  ['BillingExpr', 'Billing expression'],
] as const

type PriceField = (typeof PRICE_REVIEW_FIELDS)[number][0]
type PriceValues = Partial<Record<PriceField, string>>
export type ModelPriceChange = {
  name: string
  saved: PriceValues
  draft: PriceValues
}
export type UnreadablePriceChange = {
  field: PriceField
  label: string
  saved: string
  draft: string
}
