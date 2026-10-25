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
import type { StatusBadgeProps } from '@/components/status-badge'
import {
  BILLING_PRICING_VARS,
  normalizeTierLabel,
  parseTiersFromExpr,
  type ParsedTier,
} from '@/features/pricing/lib/billing-expr'

import type { UsageLog } from '../data/schema'
import type { LogOtherData } from '../types'

export { normalizeTierLabel }

const PARAM_OVERRIDE_ACTION_MAP: Record<string, string> = {
  set: 'Set',
  delete: 'Delete',
  copy: 'Copy',
  move: 'Move',
  append: 'Append',
  prepend: 'Prepend',
  trim_prefix: 'Trim Prefix',
  trim_suffix: 'Trim Suffix',
  ensure_prefix: 'Ensure Prefix',
  ensure_suffix: 'Ensure Suffix',
  trim_space: 'Trim Space',
  to_lower: 'To Lower',
  to_upper: 'To Upper',
  replace: 'Replace',
  regex_replace: 'Regex Replace',
  set_header: 'Set Header',
  delete_header: 'Delete Header',
  copy_header: 'Copy Header',
  move_header: 'Move Header',
  pass_headers: 'Pass Headers',
  sync_fields: 'Sync Fields',
  return_error: 'Return Error',
}

/**
 * Get localized label for a param override action
 */
export function getParamOverrideActionLabel(
  action: string,
  t: (key: string) => string
): string {
  const key = PARAM_OVERRIDE_ACTION_MAP[action.toLowerCase()]
  return key ? t(key) : action
}

/**
 * Parse a param override audit line into action and content
 */
export function parseAuditLine(
  line: string
): { action: string; content: string } | null {
  if (typeof line !== 'string') return null
  const firstSpace = line.indexOf(' ')
  if (firstSpace <= 0) return { action: line, content: line }
  return {
    action: line.slice(0, firstSpace),
    content: line.slice(firstSpace + 1),
  }
}

/**
 * Check if the log is a violation fee log
 */
export function isViolationFeeLog(other: LogOtherData | null): boolean {
  if (!other) return false
  return (
    other.violation_fee === true ||
    Boolean(other.violation_fee_code) ||
    Boolean(other.violation_fee_marker)
  )
}

function isPositiveFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0
}

function hasLegacySearchSurcharge(
  enabled: boolean | undefined,
  count: number | undefined,
  price: number | undefined
): boolean {
  return (
    enabled === true &&
    isPositiveFiniteNumber(count) &&
    isPositiveFiniteNumber(price)
  )
}

/**
 * Check whether a consume log includes an actual tool-call surcharge.
 * Structured surcharge items cover current logs, while the legacy fields keep
 * historical Web Search, File Search, and Image Generation logs visible.
 */
export function hasToolSurcharge(other: LogOtherData | null): boolean {
  if (!other) return false

  const hasStructuredSurcharge =
    Array.isArray(other.tool_surcharges) &&
    other.tool_surcharges.some(
      (item) =>
        typeof item?.name === 'string' &&
        item.name.trim() !== '' &&
        isPositiveFiniteNumber(item.count) &&
        isPositiveFiniteNumber(item.price)
    )
  if (hasStructuredSurcharge) return true

  if (
    hasLegacySearchSurcharge(
      other.web_search,
      other.web_search_call_count,
      other.web_search_price
    )
  ) {
    return true
  }

  if (
    hasLegacySearchSurcharge(
      other.file_search,
      other.file_search_call_count,
      other.file_search_price
    )
  ) {
    return true
  }

  return (
    other.image_generation_call === true &&
    isPositiveFiniteNumber(other.image_generation_call_price)
  )
}

/**
 * Parse the 'other' field from JSON string to object
 */
export function parseLogOther(other: string): LogOtherData | null {
  if (!other) return null
  try {
    return JSON.parse(other) as LogOtherData
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to parse log other field:', error)
    return null
  }
}

export function getReasoningEffortVariant(
  effort: string | undefined
): StatusBadgeProps['variant'] {
  switch (effort?.trim().toLowerCase()) {
    case 'max':
    case 'xhigh':
    case 'high':
      return 'orange'
    case 'medium':
      return 'yellow'
    case 'low':
    case 'minimal':
      return 'green'
    case 'none':
    default:
      return 'grey'
  }
}

/**
 * Get time color based on duration (in seconds)
 */
export function getTimeColor(
  seconds: number
): 'success' | 'warning' | 'danger' {
  if (seconds < 10) return 'success'
  if (seconds < 30) return 'warning'
  return 'danger'
}

/**
 * Get first-response-token color based on latency (in seconds)
 */
export function getFirstResponseTimeColor(
  seconds: number
): 'success' | 'warning' | 'danger' {
  if (seconds < 5) return 'success'
  if (seconds < 10) return 'warning'
  return 'danger'
}

/**
 * Get throughput color based on generated tokens per second
 */
export function getThroughputColor(
  tokensPerSecond: number
): 'success' | 'warning' | 'danger' {
  if (tokensPerSecond >= 30) return 'success'
  if (tokensPerSecond >= 15) return 'warning'
  return 'danger'
}

/**
 * Get response color using throughput only when enough output tokens exist.
 */
export function getResponseTimeColor(
  seconds: number,
  completionTokens: number
): 'success' | 'warning' | 'danger' {
  if (completionTokens < 100 || seconds <= 0) return getTimeColor(seconds)
  return getThroughputColor(completionTokens / seconds)
}

/**
 * Format model name with mapping indicator
 */
export function formatModelName(log: UsageLog): {
  name: string
  isMapped: boolean
  actualModel?: string
} {
  const other = parseLogOther(log.other)
  const isMapped = !!(
    other?.is_model_mapped &&
    other?.upstream_model_name &&
    other.upstream_model_name !== ''
  )

  return {
    name: log.model_name,
    isMapped,
    actualModel: isMapped ? other.upstream_model_name : undefined,
  }
}

/**
 * Decode a base64-encoded billing expression. Safely returns an empty string
 * when the input is missing or malformed (e.g. legacy logs without expr_b64).
 */
export function decodeBillingExprB64(exprB64: string | undefined): string {
  if (!exprB64) return ''
  try {
    const binaryString =
      typeof window !== 'undefined'
        ? window.atob(exprB64)
        : Buffer.from(exprB64, 'base64').toString('binary')
    const bytes = new Uint8Array(binaryString.length)

    for (let i = 0; i < binaryString.length; i++) {
      bytes[i] = binaryString.charCodeAt(i)
    }

    if (typeof TextDecoder !== 'undefined') {
      return new TextDecoder().decode(bytes)
    }

    return decodeURIComponent(
      Array.prototype.map
        .call(bytes, (byte: number) => `%${byte.toString(16).padStart(2, '0')}`)
        .join('')
    )
  } catch {
    return ''
  }
}

/**
 * Resolve which parsed tier corresponds to the matched_tier label in a log
 * entry. Missing or unknown labels do not fall back to another tier because
 * that would display guessed unit prices.
 */
export function resolveMatchedTier(
  tiers: ParsedTier[],
  matchedLabel: string | undefined
): ParsedTier | null {
  if (tiers.length === 0) return null
  if (!matchedLabel) return null
  const found = tiers.find((tier) => {
    const l1 = normalizeTierLabel(tier.label)
    const l2 = normalizeTierLabel(matchedLabel)
    return l1 === l2 && l1 !== ''
  })
  return found || null
}

/**
 * Tiered pricing summary derived from an `other` log payload using the
 * billing-expression library. Returns null when the entry is not a tiered
 * billing log or the expression failed to parse.
 */
export interface TieredBillingSummary {
  tiers: ParsedTier[]
  tier: ParsedTier
  priceEntries: Array<{ field: string; shortLabel: string; price: number }>
}

/**
 * Whether the request payload reports any cache-related token usage. Used to
 * suppress cache pricing rows from the tiered breakdown when the request did
 * not exercise the cache path.
 */
export function hasAnyCacheTokens(
  other: LogOtherData | null | undefined
): boolean {
  if (!other) return false
  return (
    (other.cache_tokens || 0) > 0 ||
    (other.cache_creation_tokens || 0) > 0 ||
    (other.cache_creation_tokens_5m || 0) > 0 ||
    (other.cache_creation_tokens_1h || 0) > 0 ||
    (other.cache_creation_tokens_12h || 0) > 0
  )
}

export function getTieredBillingSummary(
  other: LogOtherData | null
): TieredBillingSummary | null {
  if (!other || other.billing_mode !== 'tiered_expr') return null
  const exprStr = decodeBillingExprB64(other.expr_b64)
  if (!exprStr) return null
  const tiers = parseTiersFromExpr(exprStr)
  const tier = resolveMatchedTier(tiers, other.matched_tier)
  if (!tier) return null

  const cacheTokensPresent = hasAnyCacheTokens(other)

  const priceEntries: TieredBillingSummary['priceEntries'] = []
  for (const v of BILLING_PRICING_VARS) {
    if (
      !v.field ||
      (v.key === 'cc12h' && !tier.configuredPriceFields?.includes(v.field))
    ) {
      continue
    }
    if (v.group === 'cache' && !cacheTokensPresent) continue
    const raw = tier[v.field as keyof ParsedTier]
    const price = Number(raw)
    if (
      Number.isFinite(price) &&
      (price > 0 || (price === 0 && v.key === 'cc12h' && raw != null))
    ) {
      priceEntries.push({
        field: v.field,
        shortLabel: v.shortLabel,
        price,
      })
    }
  }
  return { tiers, tier, priceEntries }
}

/**
 * Calculate duration and return formatted result with color variant
 * @param submitTime - Submit timestamp
 * @param finishTime - Finish timestamp
 * @param unit - Unit of the timestamps ('seconds' or 'milliseconds')
 */
export function formatDuration(
  submitTime?: number,
  finishTime?: number,
  unit: 'seconds' | 'milliseconds' = 'milliseconds'
): { durationSec: number; variant: StatusBadgeProps['variant'] } | null {
  if (!submitTime || !finishTime) return null

  const durationSec =
    unit === 'milliseconds'
      ? (finishTime - submitTime) / 1000
      : finishTime - submitTime

  return { durationSec, variant: durationSec > 60 ? 'red' : 'green' }
}

/**
 * Maps a language-independent audit/login operation `action` to an i18n
 * template string (the template itself is the i18n key, with {{placeholders}}).
 *
 * The backend stores only `action` + structured `params` in `other.op`; the UI
 * renders localized content at display time so audit/login logs are fully
 * translatable instead of being frozen to whatever language was written to DB.
 */
const AUDIT_TEMPLATES: Record<string, string> = {
  login: 'Logged in successfully via {{method}}',
  // User management
  'user.create': 'Created user {{username}} (role {{role}})',
  'user.update': 'Updated user {{username}} (ID: {{id}})',
  'user.delete': 'Deleted user {{username}} (ID: {{id}})',
  'user.manage': 'Performed {{action}} on user {{username}} (ID: {{id}})',
  'user.quota_add': 'Increased user quota by {{quota}}',
  'user.quota_subtract': 'Decreased user quota by {{quota}}',
  'user.quota_override': 'Overrode user quota from {{from}} to {{to}}',
  'user.binding_clear': 'Cleared {{bindingType}} binding for user {{username}}',
  'user.2fa_disable': 'Force-disabled two-factor authentication for the user',
  'user.passkey_register': 'Registered a passkey',
  'user.passkey_delete': 'Deleted a passkey',
  'user.topup_complete': 'Completed top-up order for the user',
  'user.reset_passkey': 'Reset the user passkey',
  'user.oauth_unbind': 'Removed an OAuth binding for the user',
  // System settings
  'option.update': 'Updated system setting {{key}}',
  'option.payment_compliance': 'Confirmed payment compliance',
  'option.reset_ratio': 'Reset model ratios',
  'option.clear_affinity_cache': 'Cleared channel affinity cache',
  // Custom OAuth
  'custom_oauth.create': 'Created a custom OAuth provider',
  'custom_oauth.update': 'Updated a custom OAuth provider',
  'custom_oauth.delete': 'Deleted a custom OAuth provider',
  // Performance / cache
  'performance.clear_disk_cache': 'Cleared disk cache',
  'performance.gc': 'Triggered garbage collection',
  'performance.clear_logs': 'Cleared log files',
  // Channel
  'channel.create': 'Created channel {{name}} (type {{type}}, count {{count}})',
  'channel.update': 'Updated channel {{name}} (ID: {{id}})',
  'channel.delete': 'Deleted channel {{name}} (ID: {{id}})',
  'channel.delete_batch': 'Batch deleted {{count}} channels',
  'channel.delete_disabled': 'Deleted all disabled channels ({{count}})',
  'channel.key_view': 'Viewed channel key {{name}} (ID: {{id}})',
  'channel.tag_disable': 'Disabled channels with tag {{tag}}',
  'channel.tag_enable': 'Enabled channels with tag {{tag}}',
  'channel.tag_edit': 'Edited channels with tag {{tag}}',
  'channel.tag_batch_set': 'Batch set tag for {{count}} channels',
  'channel.copy':
    'Copied channel (source ID: {{sourceId}}) to {{name}} (new ID: {{id}})',
  'channel.multi_key_manage':
    'Multi-key management {{action}} on channel (ID: {{id}})',
  'channel.upstream_apply':
    'Applied upstream model changes to channel (ID: {{id}})',
  'channel.upstream_apply_all':
    'Applied upstream model changes to {{count}} channels',
  // Redemption codes
  'redemption.create':
    'Created {{count}} redemption codes named {{name}} ({{quota}} each)',
  'redemption.update': 'Updated a redemption code',
  'redemption.delete': 'Deleted a redemption code',
  'redemption.delete_invalid': 'Deleted invalid redemption codes',
  // Prefill groups
  'prefill_group.create': 'Created a prefill group',
  'prefill_group.update': 'Updated a prefill group',
  'prefill_group.delete': 'Deleted a prefill group',
  // Vendors
  'vendor.create': 'Created a vendor',
  'vendor.update': 'Updated a vendor',
  'vendor.delete': 'Deleted a vendor',
  // Model metadata
  'model.create': 'Created a model',
  'model.update': 'Updated a model',
  'model.delete': 'Deleted a model',
  'model.sync_upstream': 'Synced upstream models',
  // Deployments
  'deployment.create': 'Created a deployment',
  'deployment.update': 'Updated a deployment',
  'deployment.delete': 'Deleted a deployment',
  // Subscriptions
  'subscription.plan_create': 'Created a subscription plan',
  'subscription.plan_update': 'Updated a subscription plan',
  'subscription.bind': 'Bound a subscription',
  // Logs
  'log.clear': 'Cleared historical logs',
  'log.cleanup_start': 'Log cleanup task started.',
  // Structured administration records, including historical rows with other.op.
  'user.request_body_record':
    '{{state}} request body recording for user {{username}} (ID: {{id}})',
  'log.request_body_view':
    'Viewed the recorded request body for request {{request_id}}',
  'log.request_body_clear':
    'Cleared all recorded request bodies ({{deleted}} removed)',
  'site.create': 'Created sub-site {{name}} (ID: {{id}})',
  'site.update': 'Updated sub-site {{name}} (ID: {{id}})',
  'site.delete': 'Deleted sub-site (ID: {{id}})',
  'site.wallet_recharge':
    'Recharged sub-site {{id}} wallet by {{amount}} (1/1000 CNY)',
  'site.wallet_adjust':
    'Adjusted sub-site {{id}} wallet by {{amount}} (1/1000 CNY)',
  'redemption.void': 'Voided redemption (ID: {{id}}) and refunded its cost',
  'site_admin.branding_update':
    'Sub-site admin updated branding for site {{id}} (name {{name}})',
  'site_admin.redemption_create':
    'Sub-site admin created {{count}} redemption codes named {{name}} ({{quota}} each)',
  'site_admin.redemption_void': 'Sub-site admin voided redemption (ID: {{id}})',
  'site_admin.warn_threshold':
    'Sub-site admin set wallet warning threshold to {{threshold}} (1/1000 CNY) for site {{id}}',
  'site_admin.pay_config':
    'Sub-site admin updated payment collection settings for site {{id}}',
  'site_admin.model_pricing':
    'Sub-site admin set model pricing rate to {{model_price_rate}}/10000 for site {{id}}',
  'topup.retry_settlement':
    'Retried settlement of top-up {{trade_no}} (result {{status}})',
  'subscription.plan_reset': 'Reset active subscriptions for plan {{plan_id}}',
  'subscription.user_plan_reset':
    'Reset active plan {{plan_id}} subscriptions for user {{target_user_id}}',
  'channel.status_update': 'Updated channel {{id}} status to {{status}}',
  'channel.status_update_batch':
    'Updated {{count}} of {{total}} channels to {{status}}',
  'channel.upstream_detect_all':
    'Started upstream model detection for all channels (task: {{task_id}})',
  'email_campaign.create':
    'Created email campaign (ID: {{campaign_id}}, group: {{target_group}}, users: {{target_status}}, source: {{source}})',
  'email_suppression.add':
    'Added {{email}} to the email suppression list (reason: {{reason}})',
  'email_suppression.delete':
    'Removed email suppression entry (ID: {{suppression_id}})',
  'email_suppression.import':
    'Imported email suppression entries (imported: {{imported}}, failed: {{failed}})',
  'ticket.update':
    'Updated ticket {{ticket_id}} (status: {{status}}, priority: {{priority}})',
  'ticket.cleanup':
    'Cleaned ticket data (tickets: {{deleted_tickets}}, messages: {{deleted_messages}}, files: {{deleted_files}}, attachment records: {{deleted_rows}}, failed files: {{failed_files}})',
  'user.aff_cash_payout':
    'Recorded affiliate cash payout for inviter {{inviter_id}} (quota: {{amount}}, payout: {{payout_id}})',
  // Generic middleware fallback
  generic: '{{method}} {{route}}',
}

// Only documented enum fields are localized. User names, groups, IDs, and
// unknown future values must remain byte-for-byte intact.
const AUDIT_PARAM_LABELS: Record<
  string,
  Record<string, Record<string, string>>
> = {
  'user.manage': {
    action: {
      disable: 'Disable',
      enable: 'Enable',
      delete: 'Delete',
      promote: 'Promote',
      demote: 'Demote',
    },
  },
  'channel.multi_key_manage': {
    action: {
      disable_key: 'Disable',
      enable_key: 'Enable',
      enable_all_keys: 'Enable All',
      disable_all_keys: 'Disable All',
      delete_key: 'Delete',
      delete_disabled_keys: 'Delete Auto-Disabled',
    },
  },
  'user.binding_clear': {
    bindingType: {
      email: 'Email',
      github: 'GitHub',
      discord: 'Discord',
      oidc: 'OIDC',
      wechat: 'WeChat',
      telegram: 'Telegram',
      linuxdo: 'LinuxDO',
    },
  },
  'user.request_body_record': {
    state: {
      Enabled: 'Enabled',
      Disabled: 'Disabled',
    },
  },
  'topup.retry_settlement': {
    status: {
      pending: 'Pending',
      success: 'Success',
      failed: 'Failed',
      expired: 'Expired',
      refunded: 'Refunded',
      disputed: 'Disputed',
    },
  },
  'channel.status_update': {
    status: {
      '1': 'Enabled',
      '2': 'Manual Disabled',
      '3': 'Auto Disabled',
    },
  },
  'channel.status_update_batch': {
    status: {
      '1': 'Enabled',
      '2': 'Manual Disabled',
      '3': 'Auto Disabled',
    },
  },
  'email_campaign.create': {
    source: {
      manual: 'Manual',
      announcement: 'Announcement',
    },
    target_status: {
      '0': 'All users',
      '1': 'Enabled',
    },
    target_group: {
      '': 'All Groups',
    },
  },
  'email_suppression.add': {
    reason: {
      hard_bounce: 'Hard bounce',
      complaint: 'Spam complaint',
    },
  },
  'ticket.update': {
    status: {
      open: 'Awaiting administrator',
      awaiting_user: 'Awaiting Reply',
      closed: 'Closed',
      '': 'Unchanged',
    },
    priority: {
      low: 'Low',
      normal: 'Normal',
      high: 'High',
      urgent: 'Urgent',
      '': 'Unchanged',
    },
  },
}

const AUDIT_LOGIN_METHOD_LABELS: Record<string, string> = {
  password: 'Password',
  '2fa': 'Two-factor Authentication',
  passkey: 'Passkey',
  wechat: 'WeChat',
  telegram: 'Telegram',
  oauth: 'OAuth',
  unknown: 'Unknown',
  'oauth:github': 'GitHub',
  'oauth:discord': 'Discord',
  'oauth:oidc': 'OIDC',
  'oauth:linuxdo': 'LinuxDO',
}

/** Localize known audit login methods without normalizing custom identifiers. */
export function getAuditLoginMethodLabel(
  method: string,
  t: (key: string) => string
): string {
  if (!Object.hasOwn(AUDIT_LOGIN_METHOD_LABELS, method)) return method
  const label = t(AUDIT_LOGIN_METHOD_LABELS[method])
  return method.startsWith('oauth:') ? `${t('OAuth')} · ${label}` : label
}

/**
 * Render the localized content of an audit/login log from its structured
 * `other.op` descriptor. Returns null when the log has no recognized action,
 * letting callers fall back to the raw `content` field.
 */
export function renderAuditContent(
  other: LogOtherData | null | undefined,
  t: (key: string, opts?: Record<string, unknown>) => string
): string | null {
  const op = other?.op
  if (!op?.action || !Object.hasOwn(AUDIT_TEMPLATES, op.action)) return null
  const template = AUDIT_TEMPLATES[op.action]
  const params = { ...op.params }
  const fields = AUDIT_PARAM_LABELS[op.action]
  if (fields) {
    for (const [field, labels] of Object.entries(fields)) {
      const value = params[field]
      if (typeof value !== 'string' && typeof value !== 'number') continue
      const token = String(value)
      if (Object.hasOwn(labels, token)) params[field] = t(labels[token])
    }
  }
  if (op.action === 'login' && typeof params.method === 'string') {
    params.method = getAuditLoginMethodLabel(params.method, t)
  }
  return t(template, params)
}
