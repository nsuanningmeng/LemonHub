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
import { useQuery } from '@tanstack/react-query'
import i18next from 'i18next'
import { useEffect } from 'react'
import { toast } from 'sonner'

import { isHttpUrl } from '@/lib/content-format'

import { getHomePageContent } from '../api'
import type { HomePageContentResult } from '../types'

const STORAGE_KEY = 'home_page_content'

function getCachedHomeContent(): string | undefined {
  try {
    return localStorage.getItem(STORAGE_KEY) ?? undefined
  } catch {
    return undefined
  }
}

/** Load admin-configured Markdown, HTML, or an embedded URL. */
export function useHomePageContent(): HomePageContentResult {
  const { data, isPending, error } = useQuery({
    queryKey: ['home-page-content'],
    queryFn: async () => {
      const response = await getHomePageContent().catch((cause: unknown) => {
        // This optional content has a local fallback, so a failed request must
        // not trigger the application's global Axios error-page redirect.
        throw new Error('Failed to load home page content', { cause })
      })
      if (!response.success) {
        throw new Error(response.message || 'Failed to load home page content')
      }
      const content = response.data ?? ''
      try {
        if (content) localStorage.setItem(STORAGE_KEY, content)
        else localStorage.removeItem(STORAGE_KEY)
      } catch {
        // Storage is an optional cache; live content remains usable without it.
      }
      return content
    },
    placeholderData: getCachedHomeContent,
    staleTime: 0,
    refetchOnMount: 'always',
    retry: false,
  })

  useEffect(() => {
    if (error) toast.error(i18next.t('Failed to load home page content'))
  }, [error])

  // Query errors discard placeholderData, but cached content is still useful.
  const content = data ?? getCachedHomeContent() ?? ''
  return { content, isLoaded: !isPending, isUrl: isHttpUrl(content) }
}
