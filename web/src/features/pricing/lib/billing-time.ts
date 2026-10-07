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
export type BillingTimeZoneResolution =
  | { kind: 'supported'; timeZone: string }
  | { kind: 'invalid'; timeZone: 'UTC' }
  | {
      kind: 'unsupported'
      reason: 'server-local' | 'alias' | 'browser-unavailable'
    }

// These exact aliases have matching Go LoadLocation and Intl semantics.
// Other aliases may be valid in Go but need not match the browser's tzdata.
const VERIFIED_TIME_ZONE_ALIASES: Readonly<Record<string, string>> = {
  'Etc/UTC': 'UTC',
  'Etc/GMT': 'UTC',
  GMT: 'UTC',
  'US/Eastern': 'America/New_York',
}

export function resolveBillingTimeZone(
  timezone: string
): BillingTimeZoneResolution {
  const trimmed = timezone.trim()
  if (!trimmed) return { kind: 'supported', timeZone: 'UTC' }
  if (trimmed === 'Local') {
    return { kind: 'unsupported', reason: 'server-local' }
  }
  // Go's time.LoadLocation rejects numeric offsets accepted by some browsers.
  if (/^[+-]/.test(trimmed)) return { kind: 'invalid', timeZone: 'UTC' }

  let canonical: string
  try {
    canonical = new Intl.DateTimeFormat('en-US', {
      timeZone: trimmed,
    }).resolvedOptions().timeZone
  } catch {
    // A browser can lack a newer IANA region even when the server knows it.
    // Region-path typos are also uncertain here, so do not invent a UTC price.
    if (
      /^(Africa|America|Antarctica|Arctic|Asia|Atlantic|Australia|Europe|Indian|Pacific|Etc)\//.test(
        trimmed
      )
    ) {
      return { kind: 'unsupported', reason: 'browser-unavailable' }
    }
    return { kind: 'invalid', timeZone: 'UTC' }
  }
  if (canonical === trimmed) {
    return { kind: 'supported', timeZone: canonical }
  }
  // Intl accepts case-insensitive spellings; Go location names are case-sensitive.
  if (canonical.toLowerCase() === trimmed.toLowerCase()) {
    return { kind: 'invalid', timeZone: 'UTC' }
  }
  if (VERIFIED_TIME_ZONE_ALIASES[trimmed] === canonical) {
    return { kind: 'supported', timeZone: canonical }
  }
  return { kind: 'unsupported', reason: 'alias' }
}
