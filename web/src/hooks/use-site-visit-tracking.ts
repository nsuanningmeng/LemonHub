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

async function sendSiteVisit(): Promise<void> {
  const controller = new AbortController()
  const timeout = window.setTimeout(() => controller.abort(), 5_000)
  try {
    await fetch('/api/site-analytics/visit', {
      method: 'POST',
      headers: { 'X-Site-Visit': '1' },
      mode: 'same-origin',
      credentials: 'same-origin',
      referrerPolicy: 'no-referrer',
      keepalive: true,
      signal: controller.signal,
    })
  } finally {
    window.clearTimeout(timeout)
  }
}

export function useSiteVisitTracking(): void {
  const pathname = useRouterState({
    select: (state) => (state.isLoading ? undefined : state.location.pathname),
  })
  const lastPathname = useRef<string | undefined>(undefined)
  const delivery = useRef(Promise.resolve())

  useEffect(() => {
    if (!pathname) return

    function recordVisit() {
      if (
        !navigator.cookieEnabled ||
        navigator.doNotTrack === '1' ||
        ('globalPrivacyControl' in navigator &&
          navigator.globalPrivacyControl === true)
      ) {
        return
      }

      // Let the first response establish the visitor cookie shared by all tabs.
      delivery.current = delivery.current
        .then(() =>
          navigator.locks
            ? navigator.locks.request('lemonhub-site-visit', sendSiteVisit)
            : sendSiteVisit()
        )
        .catch(() => {
          // A failed visit must not block subsequent navigation or collection.
        })
    }

    if (lastPathname.current !== pathname) {
      lastPathname.current = pathname
      recordVisit()
    }

    function onPageShow(event: PageTransitionEvent) {
      // A history restoration can display the same document without remounting React.
      if (event.persisted && window.location.pathname === pathname) {
        recordVisit()
      }
    }
    window.addEventListener('pageshow', onPageShow)
    return () => window.removeEventListener('pageshow', onPageShow)
  }, [pathname])
}
