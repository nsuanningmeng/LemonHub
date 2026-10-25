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
import i18next from 'i18next'

import { api } from '@/lib/api'

import type {
  ClaimRewardResult,
  RewardList,
  RewardListParams,
  RewardSummary,
} from './types'

interface RewardResponse<T> {
  success: boolean
  message?: string
  data?: T
}

export async function getRewardSummary(
  signal?: AbortSignal
): Promise<RewardSummary> {
  const response = await api.get<RewardResponse<RewardSummary>>(
    '/api/rewards/summary',
    { signal }
  )
  if (!response.data.success || !response.data.data) {
    throw new Error(
      response.data.message || i18next.t('Unable to load rewards')
    )
  }
  return response.data.data
}

export async function getRewards(
  params: RewardListParams,
  signal?: AbortSignal
): Promise<RewardList> {
  const response = await api.get<RewardResponse<RewardList>>('/api/rewards', {
    params,
    signal,
  })
  if (!response.data.success || !response.data.data) {
    throw new Error(
      response.data.message || i18next.t('Unable to load rewards')
    )
  }
  return response.data.data
}

export async function claimReward(id: number): Promise<ClaimRewardResult> {
  const response = await api.post<RewardResponse<ClaimRewardResult>>(
    `/api/rewards/${id}/claim`
  )
  if (!response.data.success || !response.data.data) {
    throw new Error(
      response.data.message ||
        i18next.t('Unable to claim reward. Please try again.')
    )
  }
  return response.data.data
}
