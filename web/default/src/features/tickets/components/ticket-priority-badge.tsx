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
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'

import { TICKET_PRIORITY_META } from '../constants'
import type { TicketPriority } from '../types'

export function TicketPriorityBadge({
  priority,
}: {
  priority: TicketPriority
}) {
  const { t } = useTranslation()
  const meta = TICKET_PRIORITY_META[priority] ?? TICKET_PRIORITY_META.normal
  return (
    <StatusBadge
      label={t(meta.labelKey)}
      variant={meta.variant}
      copyable={false}
    />
  )
}
