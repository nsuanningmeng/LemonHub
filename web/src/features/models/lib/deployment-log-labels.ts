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
// Container status identifiers are kept by the API. Only known display labels
// are localized; provider-specific statuses and raw log lines pass through.
const CONTAINER_STATUS_LABELS = new Map<string, string>([
  ['created', 'Created'],
  ['running', 'Running'],
  ['paused', 'Paused'],
  ['restarting', 'Restarting'],
  ['removing', 'Removing'],
  ['exited', 'Exited'],
  ['dead', 'Dead'],
  ['pending', 'Pending'],
  ['stopped', 'Stopped'],
  ['failed', 'Failed'],
])

export function getDeploymentContainerStatusLabel(
  status: string,
  t: (key: string) => string
): string {
  const label = CONTAINER_STATUS_LABELS.get(status.toLowerCase())
  return label ? t(label) : status
}
