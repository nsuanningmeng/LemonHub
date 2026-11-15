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
import { resolveSystemName } from '@/lib/constants'

let defaultSeo: Record<string, string | null> | undefined

export function applySystemMetadataToDom(status: {
  system_name?: unknown
  seo_description?: unknown
  seo_keywords?: unknown
}) {
  if (typeof document === 'undefined') return
  defaultSeo ??= Object.fromEntries(
    ['description', 'keywords'].map((name) => [
      name,
      document.querySelector<HTMLMetaElement>(`meta[name="${name}"]`)
        ?.content ?? null,
    ])
  )
  const name = resolveSystemName(
    typeof status.system_name === 'string' ? status.system_name : undefined
  )
  document.title = name
  const metadata: Record<string, string | null> = { title: name }
  for (const [key, value] of Object.entries({
    description: status.seo_description,
    keywords: status.seo_keywords,
  })) {
    metadata[key] =
      typeof value === 'string' && value.trim() ? value : defaultSeo[key]
  }
  for (const [key, value] of Object.entries(metadata)) {
    let element = document.querySelector<HTMLMetaElement>(`meta[name="${key}"]`)
    if (value === null) {
      element?.remove()
      continue
    }
    if (!element) {
      element = document.createElement('meta')
      element.name = key
      document.head.appendChild(element)
    }
    element.content = value
  }
}

export function applyFaviconToDom(url: string) {
  if (typeof document === 'undefined' || !url) return
  try {
    const next = new URL(url, window.location.href).href
    const existing =
      document.querySelectorAll<HTMLLinkElement>('link[rel~="icon"]')
    if (existing.length === 1 && existing[0].href === next) return
    const link = document.createElement('link')
    link.rel = 'icon'
    link.href = url
    existing.forEach((l) => l.remove())
    document.head.appendChild(link)
  } catch {
    // Ignore malformed URLs
  }
}
