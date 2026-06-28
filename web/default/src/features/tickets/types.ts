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

/** Generic business response wrapper used across the API. */
export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

/** Lifecycle status of a support ticket. */
export type TicketStatus = 'open' | 'awaiting_user' | 'closed'

/** Triage priority of a support ticket. Defaults to `normal`. */
export type TicketPriority = 'low' | 'normal' | 'high' | 'urgent'

/** Who authored the most recent reply on a ticket. */
export type TicketReplyAuthor = 'user' | 'admin'

/** A configured ticket category (from the backend ticket config). */
export interface TicketTypeConfig {
  key: string
  name: string
  prompt_template: string
}

/** Global ticket feature configuration. */
export interface TicketConfig {
  enabled: boolean
  types: TicketTypeConfig[]
}

/** A support ticket summary row. Timestamps are unix SECONDS. */
export interface Ticket {
  id: number
  type: string
  title: string
  status: TicketStatus
  priority: TicketPriority
  last_reply_at: number
  last_reply_by: TicketReplyAuthor
  created_at: number
  updated_at: number
  closed_at: number
  // Present on admin list responses.
  username?: string
  user_email?: string
  message_num?: number
}

/** An image attachment carried by a message. */
export interface TicketAttachment {
  id: number
  file_name: string
  mime_type: string
  file_size: number
}

/** A single message within a ticket conversation. */
export interface TicketMessage {
  id: number
  is_admin: boolean
  content: string
  created_at: number
  username: string
  attachments?: TicketAttachment[]
}

/** Ticket plus its full conversation thread. */
export interface TicketDetail {
  ticket: Ticket
  messages: TicketMessage[]
}

/** Paginated ticket list payload. */
export interface TicketListResult {
  page: number
  page_size: number
  total: number
  items: Ticket[]
}

/** Result of a successful attachment upload. */
export interface UploadedAttachment {
  id: number
  url: string
  file_name: string
  mime_type: string
  file_size: number
}

/** Request body for creating a new ticket. */
export interface CreateTicketPayload {
  type: string
  title: string
  content: string
  attachment_ids: number[]
  priority?: TicketPriority
}

/** Request body for replying to a ticket. */
export interface ReplyTicketPayload {
  content: string
  attachment_ids: number[]
}

/** Query parameters for the user ticket list. */
export interface TicketListParams {
  p?: number
  page_size?: number
  status?: string
}

/** Query parameters for the admin ticket list. */
export interface AdminTicketListParams {
  p?: number
  page_size?: number
  status?: string
  type?: string
  priority?: string
  user_id?: string
  keyword?: string
}

/** Request body for the admin status/priority update endpoint. At least one
 *  of `status`/`priority` must be present. */
export interface AdminTicketUpdatePayload {
  status?: string
  priority?: string
}

/** Request body for the attachment cleanup job. */
export interface CleanupPayload {
  orphan_hours?: number
  closed_days?: number
  purge_closed_tickets?: boolean
}

/** Result of the attachment cleanup job. */
export interface CleanupResult {
  deleted_rows: number
  deleted_files: number
  failed_files: number
  deleted_tickets?: number
  deleted_messages?: number
  errors?: string[]
}
