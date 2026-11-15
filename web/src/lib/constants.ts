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
/**
 * Application-wide constants
 */

// System Configuration Defaults
export const DEFAULT_SYSTEM_NAME = 'LemonHub'
export const DEFAULT_LOGO = '/lemonhub-mark.svg'

/** Stock upstream branding is the unset state for a LemonHub installation. */
export function resolveSystemName(value?: string | null): string {
  const name = value?.trim()
  return !name || name === 'New API' ? DEFAULT_SYSTEM_NAME : name
}

export function resolveSystemLogo(value?: string | null): string {
  const logo = value?.trim()
  return !logo || logo === '/logo.png' ? DEFAULT_LOGO : logo
}

// LocalStorage Keys
export const STORAGE_KEYS = {
  SYSTEM_NAME: 'system_name',
  LOGO: 'logo',
  FOOTER_HTML: 'footer_html',
} as const
