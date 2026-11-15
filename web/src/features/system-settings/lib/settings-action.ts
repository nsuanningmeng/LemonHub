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
import { isAxiosError } from 'axios'

export class OptionUpdateRejectedError extends Error {}

const reportedErrors = new WeakSet<object>()

export function markSettingsErrorReported(error: unknown) {
  if (error instanceof OptionUpdateRejectedError || isAxiosError(error)) {
    reportedErrors.add(error)
  }
}

export function isSettingsErrorReported(error: unknown) {
  return (
    typeof error === 'object' && error !== null && reportedErrors.has(error)
  )
}

// Consume expected request failures only at the UI event boundary. The save
// itself must still reject so callers stop subsequent writes and keep drafts.
export async function runSettingsAction(action: () => unknown) {
  try {
    await action()
  } catch (error) {
    if (!isSettingsErrorReported(error)) throw error
  }
}
