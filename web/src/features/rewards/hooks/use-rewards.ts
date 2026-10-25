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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { handleServerError } from '@/lib/handle-server-error'
import { useAuthStore } from '@/stores/auth-store'

import { claimReward, getRewards, getRewardSummary } from '../api'
import type { RewardListParams, UserReward } from '../types'

export function useRewardSummary() {
  const userId = useAuthStore((state) => state.auth.user?.id)
  return useQuery({
    queryKey: ['rewards', userId, 'summary'],
    queryFn: ({ signal }) => getRewardSummary(signal),
    enabled: Boolean(userId),
    staleTime: 10_000,
    refetchInterval: 30_000,
  })
}

export function useRewards(params: RewardListParams) {
  const userId = useAuthStore((state) => state.auth.user?.id)
  return useQuery({
    queryKey: ['rewards', userId, 'list', params],
    queryFn: ({ signal }) => getRewards(params, signal),
    enabled: Boolean(userId),
    staleTime: 10_000,
    refetchInterval: params.status === 'claimed' ? false : 30_000,
  })
}

export function useRewardNotifications() {
  const userId = useAuthStore((state) => state.auth.user?.id)
  return useQuery({
    queryKey: ['rewards', userId, 'notifications'],
    queryFn: ({ signal }) =>
      getRewards({ status: 'pending', p: 1, page_size: 100 }, signal),
    enabled: Boolean(userId),
    staleTime: 10_000,
    refetchInterval: 30_000,
  })
}

export function useClaimReward() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (reward: Pick<UserReward, 'id' | 'user_id'>) =>
      claimReward(reward.id),
    onSuccess: async (result) => {
      const auth = useAuthStore.getState().auth
      // A response from the previous account must never overwrite the new wallet.
      if (auth.user && auth.user.id === result.reward.user_id) {
        auth.setUser({ ...auth.user, quota: result.quota })
      }
      await queryClient.invalidateQueries({
        queryKey: ['rewards', result.reward.user_id],
      })
    },
    onError: async (error, reward) => {
      if (error instanceof Error && error.message === 'reward expired') {
        await queryClient.invalidateQueries({
          queryKey: ['rewards', reward.user_id],
        })
        return
      }
      handleServerError(error)
    },
  })
}
