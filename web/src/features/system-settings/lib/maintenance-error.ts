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
// Only exact errors emitted by our maintenance endpoints are translated.
// Unknown database, file-system and upstream diagnostics remain untouched.
const MAINTENANCE_ERROR_LABELS = new Map<string, string>([
  ['We could not load system tasks.', 'We could not load system tasks.'],
  ['target timestamp is required', 'Select a timestamp before clearing logs.'],
  [
    'no log rows were deleted',
    'No log entries were deleted. Please try again.',
  ],
  [
    'task lease expired',
    'The task stopped because its execution lease expired.',
  ],
  [
    'system task lock lost',
    'The task stopped because its execution lock was lost.',
  ],
  ['context canceled', 'The task was canceled.'],
  ['context deadline exceeded', 'The task timed out.'],
  ['task id is required', 'A task ID is required.'],
  ['task not found', 'Task not found.'],
  ['type is required', 'A task type is required.'],
  [
    'invalid mode, must be by_count or by_days',
    'Choose whether to keep a number of log files or remove files by age.',
  ],
  [
    'invalid value, must be a positive integer',
    'Enter a positive whole number.',
  ],
  [
    'log directory not configured',
    'The server log directory is not configured.',
  ],
])

export function getMaintenanceErrorMessage(
  message: string,
  t: (key: string) => string
): string {
  const label = MAINTENANCE_ERROR_LABELS.get(message)
  return label ? t(label) : message
}
