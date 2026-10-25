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
export type RewardType = 'fixed' | 'random'
export type RewardStatus = 'pending' | 'claimed' | 'expired'

export interface UserReward {
  id: number
  campaign_id: number
  user_id: number
  title: string
  description: string
  type: RewardType
  min_quota: number
  max_quota: number
  quota_awarded: number
  created_at: number
  claimed_at: number
  expires_at: number
  status: RewardStatus
}

export interface RewardSummary {
  pending_count: number
  fixed_count: number
  random_count: number
  fixed_quota: number
}

export interface RewardList {
  items: UserReward[]
  total: number
}

export interface ClaimRewardResult {
  reward: UserReward
  quota: number
}

export interface RewardListParams {
  status: RewardStatus
  type?: RewardType
  p: number
  page_size: number
}
