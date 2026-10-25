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
import { api } from '@/lib/api'

import type {
  RewardCampaign,
  RewardCampaignInput,
  RewardCampaignPage,
  RewardRecipientPreview,
} from './types'

type RewardResponse<T> = { success: boolean; data: T; message?: string }

export async function previewRewardRecipients(
  input: Pick<RewardCampaignInput, 'audience' | 'start_time' | 'end_time'>
): Promise<RewardRecipientPreview> {
  const response = await api.post<RewardResponse<RewardRecipientPreview>>(
    '/api/reward-campaigns/preview',
    input
  )
  if (!response.data.success) {
    throw new Error(response.data.message || 'Failed to preview recipients')
  }
  return response.data.data
}

export async function createRewardCampaign(
  input: RewardCampaignInput
): Promise<RewardCampaign> {
  const response = await api.post<RewardResponse<RewardCampaign>>(
    '/api/reward-campaigns',
    input
  )
  if (!response.data.success) {
    throw new Error(response.data.message || 'Failed to issue rewards')
  }
  return response.data.data
}

export async function listRewardCampaigns(
  page: number
): Promise<RewardCampaignPage> {
  const response = await api.get<RewardResponse<RewardCampaignPage>>(
    '/api/reward-campaigns',
    {
      params: { p: page, page_size: 10 },
    }
  )
  if (!response.data.success) {
    throw new Error(response.data.message || 'Failed to load reward campaigns')
  }
  return response.data.data
}
