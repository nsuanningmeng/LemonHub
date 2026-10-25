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
import { useEffect, useState } from 'react'

import { useAuthStore } from '@/stores/auth-store'

import { useRewardNotifications } from '../hooks/use-rewards'
import { useRewardDialogStore } from '../lib/reward-dialog-store'
import { isRewardExpired } from '../lib/reward-expiry'
import type { UserReward } from '../types'
import { RewardClaimDialog } from './reward-claim-dialog'

function RewardInbox(props: { userId: number }) {
  const notifications = useRewardNotifications()
  const requestedReward = useRewardDialogStore((state) => state.reward)
  const clearReward = useRewardDialogStore((state) => state.clearReward)
  const [activeReward, setActiveReward] = useState<UserReward | null>(null)
  const storageKey = `rewards:dismissed:${props.userId}`
  const [dismissedIds, setDismissedIds] = useState<Set<number>>(() => {
    try {
      const saved: unknown = JSON.parse(
        sessionStorage.getItem(storageKey) || '[]'
      )
      return new Set(
        Array.isArray(saved)
          ? saved.filter((id): id is number => Number.isSafeInteger(id))
          : []
      )
    } catch {
      return new Set()
    }
  })
  const rewards = notifications.data?.items ?? []
  const nextReward = rewards.find(
    (reward) =>
      reward.user_id === props.userId &&
      !dismissedIds.has(reward.id) &&
      !isRewardExpired(reward)
  )

  useEffect(() => {
    if (requestedReward?.user_id === props.userId) {
      setActiveReward(requestedReward)
      clearReward()
    } else if (!activeReward && nextReward) {
      setActiveReward(nextReward)
    }
  }, [activeReward, clearReward, nextReward, props.userId, requestedReward])

  const closeReward = (deferRemaining: boolean) => {
    if (!activeReward) return
    const nextIds = new Set(dismissedIds)
    nextIds.add(activeReward.id)
    if (deferRemaining) rewards.forEach((reward) => nextIds.add(reward.id))
    setDismissedIds(nextIds)
    try {
      sessionStorage.setItem(storageKey, JSON.stringify([...nextIds]))
    } catch {
      // Private browsing can disable storage; the mounted session still remembers.
    }
    setActiveReward(null)
  }

  if (!activeReward || activeReward.user_id !== props.userId) return null
  return (
    <RewardClaimDialog
      key={activeReward.id}
      reward={activeReward}
      onDismiss={() => closeReward(true)}
      onComplete={() => closeReward(false)}
    />
  )
}

export function RewardsNotification() {
  const userId = useAuthStore((state) => state.auth.user?.id)
  return userId ? <RewardInbox key={userId} userId={userId} /> : null
}
