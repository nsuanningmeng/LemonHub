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
import i18next from 'i18next'
import { toast } from 'sonner'
import { deleteUser, manageUser } from '../api'
import { type ManageUserAction } from '../types'

// ============================================================================
// User Action Messages
// ============================================================================

const ACTION_MESSAGES: Record<ManageUserAction, string> = {
  enable: 'User enabled successfully',
  disable: 'User disabled successfully',
  promote: 'User promoted to admin successfully',
  demote: 'User demoted to regular user successfully',
  delete: 'User deleted successfully',
  add_quota: 'Quota adjusted successfully',
}

/**
 * Get success message for user management action
 */
export function getUserActionMessage(action: ManageUserAction): string {
  return ACTION_MESSAGES[action]
}

// ============================================================================
// Batch Actions
// ============================================================================

// Suppress the global per-request error toast for batch sub-requests so only the
// aggregated success/failure summary is shown (avoids K individual toasts plus
// one aggregate when several users are rejected by the backend role guards).
const BATCH_REQUEST_CONFIG = {
  skipBusinessError: true,
  skipErrorHandler: true,
} as const

/**
 * Run a per-user request over a set of ids and report aggregated success/fail
 * toasts. The backend `/api/user/manage` (and delete) endpoint validates each
 * user individually (role guards, root protection), so partial failures are
 * expected and surfaced rather than aborting the whole batch.
 */
async function runUserBatch(
  ids: number[],
  request: (id: number) => Promise<{ success: boolean }>,
  messages: { successKey: string; failKey: string },
  onSuccess?: () => void
): Promise<void> {
  if (ids.length === 0) {
    toast.error(i18next.t('No users selected'))
    return
  }

  const results = await Promise.allSettled(ids.map(request))
  const successCount = results.filter(
    (r) => r.status === 'fulfilled' && r.value.success
  ).length
  const failCount = results.length - successCount

  if (successCount > 0) {
    toast.success(i18next.t(messages.successKey, { count: successCount }))
    onSuccess?.()
  }
  if (failCount > 0) {
    toast.error(i18next.t(messages.failKey, { count: failCount }))
  }
}

/**
 * Batch enable users
 */
export function handleBatchEnableUsers(
  ids: number[],
  onSuccess?: () => void
): Promise<void> {
  return runUserBatch(
    ids,
    (id) => manageUser(id, 'enable', BATCH_REQUEST_CONFIG),
    {
      successKey: '{{count}} user(s) enabled',
      failKey: '{{count}} user(s) failed to enable',
    },
    onSuccess
  )
}

/**
 * Batch disable users
 */
export function handleBatchDisableUsers(
  ids: number[],
  onSuccess?: () => void
): Promise<void> {
  return runUserBatch(
    ids,
    (id) => manageUser(id, 'disable', BATCH_REQUEST_CONFIG),
    {
      successKey: '{{count}} user(s) disabled',
      failKey: '{{count}} user(s) failed to disable',
    },
    onSuccess
  )
}

/**
 * Batch delete users (hard delete, mirrors the single-user delete dialog)
 */
export function handleBatchDeleteUsers(
  ids: number[],
  onSuccess?: () => void
): Promise<void> {
  return runUserBatch(
    ids,
    (id) => deleteUser(id, BATCH_REQUEST_CONFIG),
    {
      successKey: '{{count}} user(s) deleted',
      failKey: '{{count}} user(s) failed to delete',
    },
    onSuccess
  )
}
