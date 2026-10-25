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

import { isRewardExpired } from '../lib/reward-expiry'
import type { UserReward } from '../types'

// Refresh only at the deadline (or when the tab returns), without a countdown.
export function useRewardExpired(
  reward: Pick<UserReward, 'expires_at' | 'status'>
): boolean {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!reward.expires_at || reward.status !== 'pending') return
    const deadline = reward.expires_at * 1000
    const remaining = deadline - Date.now()
    if (remaining <= 0) {
      if (now < deadline) setNow(Date.now())
      return
    }
    const updateNow = () => setNow(Date.now())
    // Browser timers overflow above 2^31-1 ms; long deadlines reschedule safely.
    const timer = window.setTimeout(
      updateNow,
      Math.min(remaining, 2_147_483_647)
    )
    document.addEventListener('visibilitychange', updateNow)
    return () => {
      window.clearTimeout(timer)
      document.removeEventListener('visibilitychange', updateNow)
    }
  }, [now, reward.expires_at, reward.status])
  return isRewardExpired(reward, now)
}
