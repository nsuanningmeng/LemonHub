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
// Only loader-specific failures qualify. HTTP/API errors must retain their
// normal error page and must never consume the resource reload allowance.
export function isResourceLoadError(error: unknown): boolean {
  if (typeof error !== 'object' || error === null) return false
  const value = error as Record<string, unknown>
  const response = value.response
  if (
    typeof value.status === 'number' ||
    (typeof response === 'object' &&
      response !== null &&
      typeof (response as Record<string, unknown>).status === 'number')
  ) {
    return false
  }
  if (typeof value.message !== 'string') return false
  if (
    value.name === 'ChunkLoadError' &&
    /^Loading (?:CSS )?chunk \S+ failed\./i.test(value.message)
  ) {
    return true
  }
  if (
    value.code === 'CSS_CHUNK_LOAD_FAILED' &&
    /^Loading CSS chunk \S+ failed\./i.test(value.message)
  ) {
    return true
  }
  return (
    value.name === 'TypeError' &&
    /^(?:Failed to fetch dynamically imported module|Importing a module script failed|error loading dynamically imported module)(?::|\.|$)/i.test(
      value.message
    )
  )
}

// Production Rsbuild entry filenames contain the content hash. Using the entry
// (not the failed chunk or route) gives the whole build one allowance per tab.
// Unknown/non-hashed entry pages fall back to manual recovery.
export function getResourceBuildId(document: Document): string | undefined {
  for (const script of document.querySelectorAll<HTMLScriptElement>(
    'script[src]'
  )) {
    if (!script.defer && script.type !== 'module') continue
    try {
      const url = new URL(script.src, document.baseURI)
      if (/\/index\.[a-f0-9]{6,}\.js$/i.test(url.pathname)) {
        return url.href.split(/[?#]/)[0]
      }
    } catch {
      // An unidentifiable build cannot safely claim automatic recovery.
    }
  }
  return undefined
}

export function claimResourceReload(
  storage: Pick<Storage, 'getItem' | 'setItem'>,
  buildId: string
): boolean {
  if (!buildId) return false
  const key = `newapi:resource-reload:${buildId}`
  try {
    if (storage.getItem(key) !== null) return false
    storage.setItem(key, '1')
    // Storage implementations may silently refuse a write. Verify it before
    // navigating; an in-memory fallback would allow a reload loop.
    return storage.getItem(key) === '1'
  } catch {
    return false
  }
}
