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
import { useRouterState } from '@tanstack/react-router'
import { useEffect, useRef } from 'react'

export function useSiteVisitTracking(): void {
  const pathname = useRouterState({
    select: (state) => (state.isLoading ? undefined : state.location.pathname),
  })
  const lastPathname = useRef<string | undefined>(undefined)
  const delivery = useRef(Promise.resolve())

  useEffect(() => {
    if (!pathname || lastPathname.current === pathname) return
    lastPathname.current = pathname

    if (
      !navigator.cookieEnabled ||
      navigator.doNotTrack === '1' ||
      ('globalPrivacyControl' in navigator &&
        navigator.globalPrivacyControl === true)
    ) {
      return
    }

    // Let the server set its visitor cookie before sending the next page view.
    delivery.current = delivery.current
      .then(async () => {
        await fetch('/api/site-analytics/visit', {
          method: 'POST',
          headers: { 'X-Site-Visit': '1' },
          mode: 'same-origin',
          credentials: 'same-origin',
          referrerPolicy: 'no-referrer',
          keepalive: true,
        })
      })
      .catch(() => {
        // Navigation and later page views can proceed if this request fails.
      })
  }, [pathname])
}
