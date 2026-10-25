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
import type { TFunction } from 'i18next'

// Only translate the application's own enums. Unknown provider values remain
// verbatim so a new code never gets mistaken for an existing diagnostic.
export function getStreamStatusLabel(
  status: string | undefined,
  t: TFunction
): string {
  if (status === 'ok') return t('Completed')
  if (!status || status === 'error') return t('Error')
  return status
}

export function getStreamEndReasonLabel(
  reason: string | undefined,
  t: TFunction
): string {
  switch (reason) {
    case 'done':
      return t('Completion signal received')
    case 'timeout':
      return t('Stream timed out')
    case 'client_gone':
      return t('Client disconnected')
    case 'scanner_error':
      return t('Stream read failed')
    case 'handler_stop':
      return t('Stopped by response handler')
    case 'eof':
      return t('Upstream stream ended')
    case 'panic':
      return t('Stream handler panic')
    case 'ping_fail':
      return t('Keep-alive failed')
    default:
      return reason || t('Unknown')
  }
}
