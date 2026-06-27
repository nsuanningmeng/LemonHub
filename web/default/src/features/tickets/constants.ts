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
import type { StatusVariant } from '@/components/status-badge'

import type { TicketPriority, TicketStatus } from './types'

/** Client-side attachment size limit (backend also enforces this). */
export const MAX_ATTACHMENT_BYTES = 5 * 1024 * 1024

/** Visual + label metadata for each ticket status. */
export const TICKET_STATUS_META: Record<
  TicketStatus,
  { labelKey: string; variant: StatusVariant }
> = {
  open: { labelKey: 'Open', variant: 'warning' },
  awaiting_user: { labelKey: 'Awaiting Reply', variant: 'info' },
  closed: { labelKey: 'Closed', variant: 'neutral' },
}

/** Ordered status options for filters and the admin status selector. */
export const TICKET_STATUS_OPTIONS: {
  value: TicketStatus
  labelKey: string
}[] = [
  { value: 'open', labelKey: 'Open' },
  { value: 'awaiting_user', labelKey: 'Awaiting Reply' },
  { value: 'closed', labelKey: 'Closed' },
]

/** Default priority applied to new tickets when none is chosen. */
export const DEFAULT_TICKET_PRIORITY: TicketPriority = 'normal'

/** Visual + label metadata for each ticket priority. */
export const TICKET_PRIORITY_META: Record<
  TicketPriority,
  { labelKey: string; variant: StatusVariant }
> = {
  low: { labelKey: 'Low', variant: 'neutral' },
  normal: { labelKey: 'Normal', variant: 'info' },
  high: { labelKey: 'High', variant: 'amber' },
  urgent: { labelKey: 'Urgent', variant: 'red' },
}

/** Ordered priority options for filters and the create/admin selectors. */
export const TICKET_PRIORITY_OPTIONS: {
  value: TicketPriority
  labelKey: string
}[] = [
  { value: 'low', labelKey: 'Low' },
  { value: 'normal', labelKey: 'Normal' },
  { value: 'high', labelKey: 'High' },
  { value: 'urgent', labelKey: 'Urgent' },
]
