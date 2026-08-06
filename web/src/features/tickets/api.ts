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
import { api } from '@/lib/api'

import type {
  AdminTicketListParams,
  AdminTicketUpdatePayload,
  ApiResponse,
  CleanupPayload,
  CleanupResult,
  CreateTicketPayload,
  ReplyTicketPayload,
  TicketConfig,
  TicketDetail,
  TicketListParams,
  TicketListResult,
  UploadedAttachment,
} from './types'

// ============================================================================
// User APIs
// ============================================================================

/** Get the global ticket feature configuration (enabled flag + types). */
export async function getTicketConfig(): Promise<ApiResponse<TicketConfig>> {
  const res = await api.get('/api/ticket/config')
  return res.data
}

/** Get a paginated list of the current user's tickets. */
export async function getTickets(
  params: TicketListParams = {}
): Promise<ApiResponse<TicketListResult>> {
  const { p = 1, page_size = 20, status = '' } = params
  const query = new URLSearchParams()
  query.set('p', String(p))
  query.set('page_size', String(page_size))
  if (status) query.set('status', status)
  const res = await api.get(`/api/ticket/?${query.toString()}`)
  return res.data
}

/** Create a new ticket. */
export async function createTicket(
  payload: CreateTicketPayload
): Promise<ApiResponse<{ id: number }>> {
  const res = await api.post('/api/ticket/', payload)
  return res.data
}

/** Get a single ticket with its full conversation thread. */
export async function getTicket(
  id: number
): Promise<ApiResponse<TicketDetail>> {
  const res = await api.get(`/api/ticket/${id}`)
  return res.data
}

/** Reply to a ticket as the owning user. */
export async function replyTicket(
  id: number,
  payload: ReplyTicketPayload
): Promise<ApiResponse<{ id: number }>> {
  const res = await api.post(`/api/ticket/${id}/reply`, payload)
  return res.data
}

/** Close a ticket. */
export async function closeTicket(id: number): Promise<ApiResponse> {
  const res = await api.post(`/api/ticket/${id}/close`)
  return res.data
}

/** Upload a single image attachment, returning its id for later submission. */
export async function uploadTicketAttachment(
  file: File
): Promise<ApiResponse<UploadedAttachment>> {
  const form = new FormData()
  form.append('file', file)
  const res = await api.post('/api/ticket/attachment', form)
  return res.data
}

// ============================================================================
// Admin APIs (role >= ADMIN)
// ============================================================================

/** Get a paginated list of all tickets with optional filters. */
export async function getAdminTickets(
  params: AdminTicketListParams = {}
): Promise<ApiResponse<TicketListResult>> {
  const {
    p = 1,
    page_size = 20,
    status,
    type,
    priority,
    user_id,
    keyword,
  } = params
  const query = new URLSearchParams()
  query.set('p', String(p))
  query.set('page_size', String(page_size))
  if (status) query.set('status', status)
  if (type) query.set('type', type)
  if (priority) query.set('priority', priority)
  if (user_id) query.set('user_id', user_id)
  if (keyword) query.set('keyword', keyword)
  const res = await api.get(`/api/ticket/admin/?${query.toString()}`)
  return res.data
}

/** Get a single ticket (admin view) with its conversation thread. */
export async function getAdminTicket(
  id: number
): Promise<ApiResponse<TicketDetail>> {
  const res = await api.get(`/api/ticket/admin/${id}`)
  return res.data
}

/** Reply to a ticket as an administrator. */
export async function replyAdminTicket(
  id: number,
  payload: ReplyTicketPayload
): Promise<ApiResponse<{ id: number }>> {
  const res = await api.post(`/api/ticket/admin/${id}/reply`, payload)
  return res.data
}

/** Update a ticket's status and/or priority as an administrator. At least one
 *  field must be supplied. */
export async function setAdminTicketStatus(
  id: number,
  payload: AdminTicketUpdatePayload
): Promise<ApiResponse> {
  const res = await api.post(`/api/ticket/admin/${id}/status`, payload)
  return res.data
}

/** Run the orphaned/closed attachment cleanup job. */
export async function cleanupAttachments(
  payload: CleanupPayload
): Promise<ApiResponse<CleanupResult>> {
  const res = await api.post('/api/ticket/admin/cleanup', payload)
  return res.data
}
