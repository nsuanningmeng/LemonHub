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
export type RewardAudience = 'topup' | 'usage' | 'both' | 'either'
export type RewardType = 'fixed' | 'random'

export type RewardCampaignInput = {
  title: string
  description: string
  type: RewardType
  min_quota: number
  max_quota: number
  audience: RewardAudience
  start_time: number
  end_time: number
  expires_in_days: number
  continuous: boolean
  request_id: string
}

export type RewardDistributionStatus =
  | 'backfilling'
  | 'scheduled'
  | 'active'
  | 'completed'
  | 'legacy'

export type RewardCampaign = Omit<RewardCampaignInput, 'continuous'> & {
  continuous?: boolean
  distribution_status?: RewardDistributionStatus
  id: number
  created_at: number
  created_by: number
  recipient_count: number
  claimed_count: number
  claimed_quota: number | null
  claimed_quota_exact?: string
  expires_at: number
}

export type RewardCampaignPage = {
  items: RewardCampaign[]
  total: number
}

export type RewardRecipientPreview = { recipient_count: number }
