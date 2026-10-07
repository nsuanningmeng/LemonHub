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
/**
 * Billing expression parsing utilities.
 *
 * Parses the dynamic billing expression format so that the pricing breakdown
 * UI can be rendered from the same backend expressions.
 *
 * Structured prices require a complete parse of a deliberately narrow subset:
 * linear prices, token/time guards, and known request-rule multipliers.
 * Expressions outside that subset retain their raw representation.
 */

import { resolveBillingTimeZone } from './billing-time'

// ---------------------------------------------------------------------------
// Variable registry
// ---------------------------------------------------------------------------

export type BillingVar = {
  key: string
  field: string | null
  tierField: string | null
  label: string
  shortLabel: string
  side: 'input' | 'output' | 'condition'
  isBase?: boolean
  isConditionOnly?: boolean
  group?: string
}

export const BILLING_VARS: BillingVar[] = [
  {
    key: 'p',
    field: 'inputPrice',
    tierField: 'input_unit_cost',
    label: 'Input price',
    shortLabel: 'Input',
    side: 'input',
    isBase: true,
  },
  {
    key: 'c',
    field: 'outputPrice',
    tierField: 'output_unit_cost',
    label: 'Completion price',
    shortLabel: 'Output',
    side: 'output',
    isBase: true,
  },
  {
    key: 'len',
    field: null,
    tierField: null,
    label: 'Input length',
    shortLabel: 'Length',
    side: 'condition',
    isConditionOnly: true,
  },
  {
    key: 'cr',
    field: 'cacheReadPrice',
    tierField: 'cache_read_unit_cost',
    label: 'Cache read price',
    shortLabel: 'Cache Read',
    side: 'input',
    group: 'cache',
  },
  {
    key: 'cc',
    field: 'cacheCreatePrice',
    tierField: 'cache_create_unit_cost',
    label: 'Cache create price',
    shortLabel: 'Cache Write',
    side: 'input',
    group: 'cache',
  },
  {
    key: 'cc1h',
    field: 'cacheCreate1hPrice',
    tierField: 'cache_create_1h_unit_cost',
    label: 'Cache create (1h) price',
    shortLabel: 'Cache Write (1h)',
    side: 'input',
    group: 'cache',
  },
  {
    key: 'img',
    field: 'imagePrice',
    tierField: 'image_unit_cost',
    label: 'Image input price',
    shortLabel: 'Image In',
    side: 'input',
    group: 'media',
  },
  {
    key: 'img_o',
    field: 'imageOutputPrice',
    tierField: 'image_output_unit_cost',
    label: 'Image output price',
    shortLabel: 'Image Out',
    side: 'output',
    group: 'media',
  },
  {
    key: 'ai',
    field: 'audioInputPrice',
    tierField: 'audio_input_unit_cost',
    label: 'Audio input price',
    shortLabel: 'Audio In',
    side: 'input',
    group: 'media',
  },
  {
    key: 'ao',
    field: 'audioOutputPrice',
    tierField: 'audio_output_unit_cost',
    label: 'Audio output price',
    shortLabel: 'Audio Out',
    side: 'output',
    group: 'media',
  },
]

/** Vars that have real price fields (excludes condition-only vars like `len`) */
export const BILLING_PRICING_VARS: BillingVar[] = BILLING_VARS.filter(
  (v) => !v.isConditionOnly
)

/** Vars valid in tier conditions (`p`, `c`, `len`) */
export const BILLING_CONDITION_VARS: string[] = BILLING_VARS.filter(
  (v) => v.isBase || v.isConditionOnly
).map((v) => v.key)

const BILLING_VAR_KEY_TO_FIELD = Object.fromEntries(
  BILLING_PRICING_VARS.map((v) => [v.key, v.field as string])
) as Record<string, string>

export const BILLING_EXTRA_VARS: BillingVar[] = BILLING_VARS.filter(
  (v) => !v.isBase && !v.isConditionOnly
)

export const BILLING_CACHE_VAR_MAP = BILLING_EXTRA_VARS.map((v) => ({
  field: v.tierField as string,
  exprVar: v.key,
}))

// ---------------------------------------------------------------------------
// Request rule constants
// ---------------------------------------------------------------------------

export const SOURCE_PARAM = 'param'
export const SOURCE_HEADER = 'header'
export const SOURCE_TIME = 'time'
export const SOURCE_TOKEN = 'token'

export const MATCH_EQ = 'eq'
export const MATCH_CONTAINS = 'contains'
export const MATCH_GT = 'gt'
export const MATCH_GTE = 'gte'
export const MATCH_LT = 'lt'
export const MATCH_LTE = 'lte'
export const MATCH_EXISTS = 'exists'
export const MATCH_RANGE = 'range'

export const TIME_FUNCS = ['hour', 'minute', 'weekday', 'month', 'day'] as const
export type TimeFunc = (typeof TIME_FUNCS)[number]

export const COMMON_TIMEZONES: { value: string; label: string }[] = [
  { value: 'Asia/Shanghai', label: 'UTC+8 Shanghai (Asia/Shanghai)' },
  { value: 'UTC', label: 'UTC' },
  { value: 'America/New_York', label: 'UTC-5 New York (America/New_York)' },
  {
    value: 'America/Los_Angeles',
    label: 'UTC-8 Los Angeles (America/Los_Angeles)',
  },
  { value: 'America/Chicago', label: 'UTC-6 Chicago (America/Chicago)' },
  { value: 'Europe/London', label: 'UTC+0 London (Europe/London)' },
  { value: 'Europe/Berlin', label: 'UTC+1 Berlin (Europe/Berlin)' },
  { value: 'Asia/Tokyo', label: 'UTC+9 Tokyo (Asia/Tokyo)' },
  { value: 'Asia/Singapore', label: 'UTC+8 Singapore (Asia/Singapore)' },
  { value: 'Asia/Seoul', label: 'UTC+9 Seoul (Asia/Seoul)' },
  { value: 'Australia/Sydney', label: 'UTC+10 Sydney (Australia/Sydney)' },
]

const NUMERIC_LITERAL_REGEX = /^-?(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?$/

export type ParamHeaderCondition = {
  source: 'param' | 'header' | 'token'
  path: string
  mode: string
  value: string
}

export type TimeCondition = {
  source: 'time'
  timeFunc: TimeFunc
  timezone: string
  mode: string
  value: string
  rangeStart: string
  rangeEnd: string
}

export type RequestCondition = TimeCondition | ParamHeaderCondition

export type RequestRuleGroup = {
  conditions: RequestCondition[]
  multiplier: string
  conditionText?: string
  matched?: boolean
}

export type RequestRuleTrace = {
  cond: string
  multiplier: number
  matched: boolean
}

export type TierCondition = {
  var: 'p' | 'c' | 'len'
  op: '<' | '<=' | '>' | '>='
  value: number
}

export type ParsedTier = {
  label: string
  conditions: TierCondition[]
  /** The complete path to this tier, including preceding false branches. */
  conditionText?: string
  /** Present only when the whole guard is a lossless list of conditions. */
  displayConditions?: RequestCondition[]
  [field: string]: unknown
}

// ---------------------------------------------------------------------------
// Tier parser
// ---------------------------------------------------------------------------

function stripExprVersion(exprStr: string): { version: number; body: string } {
  if (!exprStr) return { version: 1, body: '' }
  const m = exprStr.match(/^v(\d+):([\s\S]*)$/)
  if (m) return { version: Number(m[1]), body: m[2] }
  return { version: 1, body: exprStr }
}

type PricingToken = { text: string; start: number; end: number }
type PricingNode = { start: number; end: number } & (
  | { kind: 'number'; value: number }
  | { kind: 'string'; value: string }
  | { kind: 'variable'; name: string }
  | { kind: 'call'; name: string; args: PricingNode[] }
  | { kind: 'unary'; op: string; value: PricingNode }
  | { kind: 'binary'; op: string; left: PricingNode; right: PricingNode }
  | {
      kind: 'conditional'
      condition: PricingNode
      yes: PricingNode
      no: PricingNode
    }
)

// This is a display-only subset, not an implementation of expr-lang. Unknown
// syntax remains a raw expression; no prefix or embedded tier may be extracted.
function pricingTokens(source: string): PricingToken[] {
  const tokens: PricingToken[] = []
  const token =
    /(?:"(?:[^"\\]|\\.)*"|(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?|[A-Za-z_]\w*|&&|\|\||<=|>=|==|!=|[()+*/?:,!<>-])/y
  let index = 0
  while (index < source.length) {
    if (/\s/.test(source[index])) {
      index += 1
      continue
    }
    token.lastIndex = index
    const match = token.exec(source)
    if (!match) throw new Error('Unsupported pricing syntax')
    tokens.push({ text: match[0], start: index, end: token.lastIndex })
    index = token.lastIndex
  }
  return tokens
}

const PRICING_PRECEDENCE: Record<string, number> = {
  '||': 1,
  '&&': 2,
  '==': 3,
  '!=': 3,
  '<': 4,
  '<=': 4,
  '>': 4,
  '>=': 4,
  '+': 5,
  '-': 5,
  '*': 6,
  '/': 6,
}

class PricingParser {
  private index = 0
  constructor(private tokens: PricingToken[]) {}

  parse(): PricingNode {
    const node = this.expression()
    if (this.index !== this.tokens.length) {
      throw new Error('Incomplete pricing expression')
    }
    return node
  }

  private take(expected?: string): PricingToken {
    const token = this.tokens[this.index++]
    if (!token || (expected && token.text !== expected)) {
      throw new Error('Incomplete pricing expression')
    }
    return token
  }

  private expression(): PricingNode {
    const condition = this.binary(1)
    if (this.tokens[this.index]?.text !== '?') return condition
    this.take('?')
    const yes = this.expression()
    this.take(':')
    const no = this.expression()
    return {
      kind: 'conditional',
      condition,
      yes,
      no,
      start: condition.start,
      end: no.end,
    }
  }

  private binary(minimum: number): PricingNode {
    let left = this.primary()
    while (true) {
      const op = this.tokens[this.index]?.text
      const precedence = PRICING_PRECEDENCE[op] || 0
      if (precedence < minimum) return left
      this.take()
      const right = this.binary(precedence + 1)
      left = {
        kind: 'binary',
        op,
        left,
        right,
        start: left.start,
        end: right.end,
      }
    }
  }

  private primary(): PricingNode {
    const token = this.take()
    const start = token.start
    if (token.text === '(') {
      const node = this.expression()
      const close = this.take(')')
      return { ...node, start, end: close.end }
    }
    if (['+', '-', '!'].includes(token.text)) {
      const value = this.primary()
      return { kind: 'unary', op: token.text, value, start, end: value.end }
    }
    if (token.text.startsWith('"')) {
      const value: unknown = JSON.parse(token.text)
      if (typeof value !== 'string') throw new Error('Invalid pricing string')
      return { kind: 'string', value, start, end: token.end }
    }
    if (NUMERIC_LITERAL_REGEX.test(token.text)) {
      const value = Number(token.text)
      if (!Number.isFinite(value)) throw new Error('Invalid pricing number')
      return { kind: 'number', value, start, end: token.end }
    }
    if (!/^[A-Za-z_]\w*$/.test(token.text)) {
      throw new Error('Invalid pricing operand')
    }
    if (this.tokens[this.index]?.text !== '(') {
      return { kind: 'variable', name: token.text, start, end: token.end }
    }
    this.take('(')
    const args: PricingNode[] = []
    if (this.tokens[this.index]?.text !== ')') {
      args.push(this.expression())
      while (this.tokens[this.index]?.text === ',') {
        this.take(',')
        args.push(this.expression())
      }
    }
    const close = this.take(')')
    return { kind: 'call', name: token.text, args, start, end: close.end }
  }
}

type LinearPrice = { constant: number; coefficients: Record<string, number> }

function linearPrice(node: PricingNode): LinearPrice | null {
  if (node.kind === 'number') return { constant: node.value, coefficients: {} }
  if (
    node.kind === 'variable' &&
    Object.hasOwn(BILLING_VAR_KEY_TO_FIELD, node.name)
  ) {
    return { constant: 0, coefficients: { [node.name]: 1 } }
  }
  if (node.kind === 'unary' && node.op === '+') return linearPrice(node.value)
  if (node.kind !== 'binary') return null
  const left = linearPrice(node.left)
  const right = linearPrice(node.right)
  if (!left || !right) return null
  if (node.op === '+') {
    const coefficients = { ...left.coefficients }
    for (const [key, value] of Object.entries(right.coefficients)) {
      coefficients[key] = (coefficients[key] ?? 0) + value
    }
    return { constant: left.constant + right.constant, coefficients }
  }
  let scale: number
  let price: LinearPrice
  if (node.op === '*' && Object.keys(left.coefficients).length === 0) {
    scale = left.constant
    price = right
  } else if (node.op === '*' && Object.keys(right.coefficients).length === 0) {
    scale = right.constant
    price = left
  } else if (
    node.op === '/' &&
    Object.keys(right.coefficients).length === 0 &&
    right.constant > 0
  ) {
    scale = 1 / right.constant
    price = left
  } else {
    return null
  }
  return {
    constant: price.constant * scale,
    coefficients: Object.fromEntries(
      Object.entries(price.coefficients).map(([key, value]) => [
        key,
        value * scale,
      ])
    ),
  }
}

function tierPrice(node: PricingNode): LinearPrice | null {
  if (
    node.kind === 'call' &&
    node.name === 'tier' &&
    node.args.length === 2 &&
    node.args[0].kind === 'string'
  ) {
    return linearPrice(node.args[1])
  }
  return linearPrice(node)
}

function validLinearPrice(price: LinearPrice | null): price is LinearPrice {
  return (
    price !== null &&
    [price.constant, ...Object.values(price.coefficients)].every(
      (value) => Number.isFinite(value) && value >= 0
    )
  )
}

function supportedTierCondition(node: PricingNode): boolean {
  if (node.kind === 'unary' && node.op === '!') {
    return supportedTierCondition(node.value)
  }
  if (node.kind !== 'binary') return false
  if (node.op === '&&' || node.op === '||') {
    return (
      supportedTierCondition(node.left) && supportedTierCondition(node.right)
    )
  }
  if (
    !['<', '<=', '>', '>=', '==', '!='].includes(node.op) ||
    node.right.kind !== 'number'
  ) {
    return false
  }
  const value = node.left
  if (value.kind === 'variable') {
    return BILLING_CONDITION_VARS.includes(value.name)
  }
  if (
    value.kind !== 'call' ||
    !TIME_FUNCS.includes(value.name as TimeFunc) ||
    value.args.length !== 1 ||
    value.args[0].kind !== 'string'
  ) {
    return false
  }
  if (!isTimeValueInRange(value.name as TimeFunc, String(node.right.value))) {
    return false
  }
  return resolveBillingTimeZone(value.args[0].value).kind === 'supported'
}

function legacyTierConditions(
  node: PricingNode,
  negate = false
): TierCondition[] | null {
  if (node.kind === 'unary' && node.op === '!') {
    return legacyTierConditions(node.value, !negate)
  }
  if (node.kind !== 'binary') return null
  if ((!negate && node.op === '&&') || (negate && node.op === '||')) {
    const left = legacyTierConditions(node.left, negate)
    const right = legacyTierConditions(node.right, negate)
    return left && right ? [...left, ...right] : null
  }
  if (
    node.left.kind !== 'variable' ||
    !BILLING_CONDITION_VARS.includes(node.left.name) ||
    node.right.kind !== 'number'
  ) {
    return null
  }
  const inverse: Record<string, TierCondition['op']> = {
    '<': '>=',
    '<=': '>',
    '>': '<=',
    '>=': '<',
  }
  if (!Object.hasOwn(inverse, node.op)) return null
  const op = negate ? inverse[node.op] : (node.op as TierCondition['op'])
  return [
    {
      var: node.left.name as TierCondition['var'],
      op,
      value: node.right.value,
    },
  ]
}

type TierGuard = { node: PricingNode; negate: boolean }

function collectPricingTiers(
  node: PricingNode,
  source: string,
  guards: TierGuard[],
  tiers: ParsedTier[]
): boolean {
  if (node.kind === 'conditional') {
    if (!supportedTierCondition(node.condition)) return false
    return (
      collectPricingTiers(
        node.yes,
        source,
        [...guards, { node: node.condition, negate: false }],
        tiers
      ) &&
      collectPricingTiers(
        node.no,
        source,
        [...guards, { node: node.condition, negate: true }],
        tiers
      )
    )
  }
  const price = tierPrice(node)
  if (
    !validLinearPrice(price) ||
    price.constant !== 0 ||
    Object.keys(price.coefficients).length === 0
  ) {
    return false
  }
  const tier: ParsedTier = { label: '', conditions: [] }
  if (node.kind === 'call' && node.args[0].kind === 'string') {
    tier.label = node.args[0].value
  }
  for (const [key, field] of Object.entries(BILLING_VAR_KEY_TO_FIELD)) {
    tier[field] = price.coefficients[key] ?? 0
  }
  const parts = guards.map((guard) => {
    const text = source.slice(guard.node.start, guard.node.end)
    if (guard.negate) return `!(${text})`
    return guards.length > 1 ? `(${text})` : text
  })
  if (parts.length > 0) {
    tier.conditionText = parts.join(' && ')
    const conditions = guards.map((guard) =>
      legacyTierConditions(guard.node, guard.negate)
    )
    if (conditions.every((condition) => condition !== null)) {
      tier.conditions = conditions.flat() as TierCondition[]
    }
    // The existing request-condition parser knows safe flat AND/time ranges.
    // Complex OR/else guards retain their complete text, never partial rows.
    const display = tryParseRequestConditions(tier.conditionText)
    if (display) tier.displayConditions = display
  }
  tiers.push(tier)
  return true
}

export function parseTiersFromExpr(exprStr: string): ParsedTier[] {
  if (!exprStr) return []
  try {
    const { version, body } = stripExprVersion(exprStr.trim())
    if (version !== 1) return []
    const split = splitBillingExprAndRequestRules(body)
    const source = split.billingExpr
    const node = new PricingParser(pricingTokens(source)).parse()
    const tiers: ParsedTier[] = []
    return collectPricingTiers(node, source, [], tiers) ? tiers : []
  } catch {
    return []
  }
}

export function classifyBillingExpression(
  expr: string
): 'token' | 'request' | 'dynamic' {
  try {
    const { version, body } = stripExprVersion(expr.trim())
    if (version !== 1) return 'dynamic'
    const node = new PricingParser(pricingTokens(body)).parse()
    const price = tierPrice(node)
    if (!validLinearPrice(price)) return 'dynamic'
    if (Object.keys(price.coefficients).length === 0) return 'request'
    return price.constant === 0 ? 'token' : 'dynamic'
  } catch {
    return 'dynamic'
  }
}

export function normalizeTierLabel(label: string | undefined): string {
  if (!label) return ''
  return label
    .replaceAll(/<[=＝]?|≤|＜[=＝]?/g, '<')
    .replaceAll(/>[=＝]?|≥|＞[=＝]?/g, '>')
    .replaceAll(/\s+/g, '')
    .toLowerCase()
}

// ---------------------------------------------------------------------------
// Request rule parser
// ---------------------------------------------------------------------------

function splitTopLevelOperator(expr: string, operator: string): string[] {
  try {
    const parts: string[] = []
    let start = 0
    let depth = 0
    for (const token of pricingTokens(expr)) {
      if (token.text === '(') depth += 1
      if (token.text === ')') depth -= 1
      if (depth < 0) return [expr]
      if (depth === 0 && token.text === operator) {
        parts.push(expr.slice(start, token.start).trim())
        start = token.end
      }
    }
    parts.push(expr.slice(start).trim())
    if (depth !== 0 || parts.some((part) => !part)) return [expr]
    return parts
  } catch {
    return [expr]
  }
}

function splitTopLevelMultiply(expr: string): string[] {
  return splitTopLevelOperator(expr, '*')
}

function splitTopLevelAnd(expr: string): string[] {
  return splitTopLevelOperator(expr, '&&')
}

function parseExprLiteral(raw: string): string | null {
  const text = raw.trim()
  if (text === 'true' || text === 'false') return text
  if (NUMERIC_LITERAL_REGEX.test(text)) {
    return Number.isFinite(Number(text)) ? text : null
  }
  try {
    const value: unknown = JSON.parse(text)
    return typeof value === 'string' ? value : null
  } catch {
    return null
  }
}

// Time function value domains. Values outside these ranges are invalid for
// the corresponding time function (e.g. hour() is 0-23) and would otherwise
// produce always-true conditions like hour >= -1 || hour < -5.
const TIME_FUNC_RANGES: Record<TimeFunc, [number, number]> = {
  hour: [0, 23],
  minute: [0, 59],
  weekday: [0, 6],
  month: [1, 12],
  day: [1, 31],
}

function isTimeValueInRange(timeFunc: TimeFunc, text: string): boolean {
  if (!NUMERIC_LITERAL_REGEX.test(text)) return false
  const value = Number(text)
  if (!Number.isInteger(value)) return false
  const [min, max] = TIME_FUNC_RANGES[timeFunc]
  return value >= min && value <= max
}

function tryParseTimeCondition(expr: string): RequestCondition | null {
  let m = expr.match(
    /^(hour|minute|weekday|month|day)\("([^"]*)"\) >= ([\d.eE+-]+) (&&|\|\|) \1\("\2"\) < ([\d.eE+-]+)$/
  )
  if (!m) {
    m = expr.match(
      /^\((hour|minute|weekday|month|day)\("([^"]*)"\) >= ([\d.eE+-]+) (&&|\|\|) \1\("\2"\) < ([\d.eE+-]+)\)$/
    )
  }
  if (m) {
    // Reject invalid bounds at parse time too: an unparseable rule keeps the
    // editor in raw mode, while a leniently parsed one would be silently
    // dropped when the visual editor rebuilds the expression.
    if (
      !isTimeValueInRange(m[1] as TimeFunc, m[3]) ||
      !isTimeValueInRange(m[1] as TimeFunc, m[5])
    ) {
      return null
    }
    // Only recognize ranges whose operator matches the visual builder.
    // Stored expressions are billing contracts: never reinterpret an explicit
    // OR as AND (or vice versa) merely by opening and saving the editor.
    if ((m[4] === '||') !== Number(m[3]) > Number(m[5])) return null
    const zone = resolveBillingTimeZone(m[2])
    if (zone.kind !== 'supported') return null
    return {
      source: 'time',
      timeFunc: m[1] as TimeFunc,
      timezone: zone.timeZone,
      mode: MATCH_RANGE,
      value: '',
      rangeStart: m[3],
      rangeEnd: m[5],
    }
  }
  m = expr.match(
    /^(hour|minute|weekday|month|day)\("([^"]*)"\) (==|>=|<) ([\d.eE+-]+)$/
  )
  if (m) {
    if (!isTimeValueInRange(m[1] as TimeFunc, m[4])) return null
    const opMap: Record<string, string> = {
      '==': MATCH_EQ,
      '>=': MATCH_GTE,
      '<': MATCH_LT,
    }
    const zone = resolveBillingTimeZone(m[2])
    if (zone.kind !== 'supported') return null
    return {
      source: 'time',
      timeFunc: m[1] as TimeFunc,
      timezone: zone.timeZone,
      mode: opMap[m[3]] || MATCH_EQ,
      value: m[4],
      rangeStart: '',
      rangeEnd: '',
    }
  }
  return null
}

function tryParseRequestCondition(expr: string): RequestCondition | null {
  const tc = tryParseTimeCondition(expr)
  if (tc) return tc

  let m = expr.match(/^(\w+) (==|>|>=|<|<=) (.+)$/)
  if (m && BILLING_CONDITION_VARS.includes(m[1])) {
    if (!NUMERIC_LITERAL_REGEX.test(m[3]) || !Number.isFinite(Number(m[3]))) {
      return null
    }
    const opMap: Record<string, string> = {
      '==': MATCH_EQ,
      '>': MATCH_GT,
      '>=': MATCH_GTE,
      '<': MATCH_LT,
      '<=': MATCH_LTE,
    }
    return { source: SOURCE_TOKEN, path: m[1], mode: opMap[m[2]], value: m[3] }
  }

  m = expr.match(/^header\("([^"]+)"\) != ""$/)
  if (m) return { source: 'header', path: m[1], mode: MATCH_EXISTS, value: '' }

  m = expr.match(/^param\("([^"]+)"\) != nil$/)
  if (m) return { source: 'param', path: m[1], mode: MATCH_EXISTS, value: '' }

  m = expr.match(/^has\(header\("([^"]+)"\), ((?:"(?:[^"\\]|\\.)*"))\)$/)
  if (m) {
    return {
      source: 'header',
      path: m[1],
      mode: MATCH_CONTAINS,
      value: JSON.parse(m[2]) as string,
    }
  }

  m = expr.match(
    /^param\("([^"]+)"\) != nil && has\(param\("([^"]+)"\), ((?:"(?:[^"\\]|\\.)*"))\)$/
  )
  if (m && m[1] === m[2]) {
    return {
      source: 'param',
      path: m[1],
      mode: MATCH_CONTAINS,
      value: JSON.parse(m[3]) as string,
    }
  }

  m = expr.match(
    /^param\("([^"]+)"\) != nil && param\("([^"]+)"\) (>|>=|<|<=) ([\d.eE+-]+)$/
  )
  if (m && m[1] === m[2]) {
    const opMap: Record<string, string> = {
      '>': MATCH_GT,
      '>=': MATCH_GTE,
      '<': MATCH_LT,
      '<=': MATCH_LTE,
    }
    return { source: 'param', path: m[1], mode: opMap[m[3]], value: m[4] }
  }

  m = expr.match(/^(param|header)\("([^"]+)"\) == (.+)$/)
  if (m) {
    const parsedValue = parseExprLiteral(m[3])
    if (parsedValue === null) return null
    return {
      source: m[1] as 'param' | 'header',
      path: m[2],
      mode: MATCH_EQ,
      value: String(parsedValue),
    }
  }

  return null
}

function tryParseTimeRangePair(
  lower: string,
  upper: string
): RequestCondition | null {
  const a = tryParseTimeCondition(lower)
  const b = tryParseTimeCondition(upper)
  if (!a || !b || a.source !== 'time' || b.source !== 'time') return null
  const ta = a as TimeCondition
  const tb = b as TimeCondition
  if (ta.timeFunc !== tb.timeFunc || ta.timezone !== tb.timezone) return null
  if (ta.mode !== MATCH_GTE || tb.mode !== MATCH_LT) return null
  if (Number(ta.value) > Number(tb.value)) return null
  return {
    source: 'time',
    timeFunc: ta.timeFunc,
    timezone: ta.timezone,
    mode: MATCH_RANGE,
    value: '',
    rangeStart: ta.value,
    rangeEnd: tb.value,
  }
}

function tryParseRequestConditions(
  conditionStr: string
): RequestCondition[] | null {
  // A single time range like hour(tz) >= 9 && hour(tz) < 12 must stay one
  // MATCH_RANGE condition instead of being split into two scalar conditions.
  const wholeTimeCond = tryParseTimeCondition(conditionStr.trim())
  if (wholeTimeCond) return [wholeTimeCond]

  const andParts = splitTopLevelAnd(conditionStr)
  const conditions: RequestCondition[] = []
  for (let i = 0; i < andParts.length; i += 1) {
    const part = andParts[i].trim()
    // Adjacent matching time bounds (fn >= X && fn < Y) form one range; merge
    // them so the visual editor keeps a single MATCH_RANGE row even when
    // other conditions follow in the same group.
    const next = i + 1 < andParts.length ? andParts[i + 1].trim() : ''
    const merged = next ? tryParseTimeRangePair(part, next) : null
    if (merged) {
      conditions.push(merged)
      i += 1
      continue
    }
    const condition = tryParseRequestCondition(part)
    if (!condition) return null
    // A || B && C is not (A || B) && C. Mixed overnight ranges must have
    // explicit parentheses before they can be represented as visual rows.
    if (
      andParts.length > 1 &&
      condition.source === SOURCE_TIME &&
      condition.mode === MATCH_RANGE &&
      Number(condition.rangeStart) > Number(condition.rangeEnd) &&
      !hasFullOuterParens(part)
    ) {
      return null
    }
    conditions.push(condition)
  }
  return conditions.length > 0 ? conditions : null
}

function tryParseRuleGroupFactor(part: string): RequestRuleGroup | null {
  const m = part.match(/^\((.+) \? ([\d.eE+-]+) : 1\)$/s)
  if (!m) return null
  if (!Number.isFinite(Number(m[2])) || Number(m[2]) < 0) return null

  const conditions = tryParseRequestConditions(m[1])
  if (!conditions) return null
  return { conditions, multiplier: m[2] }
}

export function requestRuleGroupsFromTrace(
  requestRules: RequestRuleTrace[]
): RequestRuleGroup[] {
  return requestRules.map((rule) => {
    const conditionText = rule.cond.trim()
    return {
      conditions: tryParseRequestConditions(conditionText) || [],
      multiplier: String(rule.multiplier),
      conditionText,
      matched: rule.matched,
    }
  })
}

export function tryParseRequestRuleExpr(
  expr: string
): RequestRuleGroup[] | null {
  const trimmed = (expr || '').trim()
  if (!trimmed) return []

  const parts = splitTopLevelMultiply(trimmed)
  const groups: RequestRuleGroup[] = []
  for (const part of parts) {
    const group = tryParseRuleGroupFactor(part)
    if (!group) return null
    groups.push(group)
  }
  return groups
}

// ---------------------------------------------------------------------------
// Combine / split billing expr and request rules
// ---------------------------------------------------------------------------

function hasFullOuterParens(expr: string): boolean {
  try {
    const tokens = pricingTokens(expr)
    if (tokens[0]?.text !== '(' || tokens.at(-1)?.text !== ')') return false
    let depth = 0
    for (const [index, token] of tokens.entries()) {
      if (token.text === '(') depth += 1
      if (token.text === ')') depth -= 1
      if (depth <= 0 && index < tokens.length - 1) return false
    }
    return depth === 0
  } catch {
    return false
  }
}

function unwrapOuterParens(expr: string): string {
  let current = (expr || '').trim()
  while (hasFullOuterParens(current)) {
    current = current.slice(1, -1).trim()
  }
  return current
}

export function splitBillingExprAndRequestRules(expr: string): {
  billingExpr: string
  requestRuleExpr: string
} {
  const trimmed = (expr || '').trim()
  if (!trimmed) return { billingExpr: '', requestRuleExpr: '' }

  const { version, body } = stripExprVersion(trimmed)
  if (version !== 1) return { billingExpr: trimmed, requestRuleExpr: '' }
  const parts: string[] = []
  try {
    const remaining = [new PricingParser(pricingTokens(body)).parse()]
    for (let node = remaining.pop(); node; node = remaining.pop()) {
      if (node.kind === 'binary' && node.op === '*') {
        remaining.push(node.right, node.left)
      } else {
        parts.push(body.slice(node.start, node.end))
      }
    }
  } catch {
    return { billingExpr: trimmed, requestRuleExpr: '' }
  }
  if (parts.length <= 1) return { billingExpr: trimmed, requestRuleExpr: '' }

  const ruleParts: string[] = []
  const baseParts: string[] = []

  parts.forEach((part) => {
    const parsed = tryParseRequestRuleExpr(part)
    if (parsed && parsed.length > 0) {
      ruleParts.push(part)
    } else {
      baseParts.push(part)
    }
  })

  if (ruleParts.length === 0 || baseParts.length !== 1) {
    return { billingExpr: trimmed, requestRuleExpr: '' }
  }

  return {
    billingExpr: `${trimmed.startsWith('v1:') ? 'v1:' : ''}${unwrapOuterParens(baseParts[0])}`,
    requestRuleExpr: ruleParts.join(' * '),
  }
}

export function combineBillingExpr(
  baseExpr: string,
  requestRuleExpr: string
): string {
  const base = (baseExpr || '').trim()
  const rules = (requestRuleExpr || '').trim()
  if (!base) return ''
  if (!rules) return base
  return `(${base}) * ${rules}`
}

// ---------------------------------------------------------------------------
// Editor: empty constructors
// ---------------------------------------------------------------------------

export function createEmptyCondition(): ParamHeaderCondition {
  return { source: 'param', path: '', mode: MATCH_EQ, value: '' }
}

export function createEmptyTimeCondition(): TimeCondition {
  return {
    source: 'time',
    timeFunc: 'hour',
    timezone: 'Asia/Shanghai',
    mode: MATCH_GTE,
    value: '',
    rangeStart: '',
    rangeEnd: '',
  }
}

export function createEmptyRuleGroup(): RequestRuleGroup {
  return { conditions: [createEmptyCondition()], multiplier: '' }
}

export function createEmptyTimeRuleGroup(): RequestRuleGroup {
  return { conditions: [createEmptyTimeCondition()], multiplier: '' }
}

// ---------------------------------------------------------------------------
// Editor: match option helpers
// ---------------------------------------------------------------------------

export type MatchOption = { value: string; labelKey: string }

export function getRequestRuleMatchOptions(source: string): MatchOption[] {
  if (source === SOURCE_TIME) {
    return [
      { value: MATCH_EQ, labelKey: 'Equals' },
      { value: MATCH_GTE, labelKey: 'Greater than or equal' },
      { value: MATCH_LT, labelKey: 'Less than' },
      { value: MATCH_RANGE, labelKey: 'Time range' },
    ]
  }
  if (source === SOURCE_TOKEN) {
    return [
      { value: MATCH_EQ, labelKey: 'Equals' },
      { value: MATCH_GT, labelKey: 'Greater than' },
      { value: MATCH_GTE, labelKey: 'Greater than or equal' },
      { value: MATCH_LT, labelKey: 'Less than' },
      { value: MATCH_LTE, labelKey: 'Less than or equal' },
    ]
  }
  const base: MatchOption[] = [
    { value: MATCH_EQ, labelKey: 'Equals' },
    { value: MATCH_CONTAINS, labelKey: 'Contains' },
    { value: MATCH_EXISTS, labelKey: 'Exists' },
  ]
  if (source === SOURCE_HEADER) return base
  return [
    ...base,
    { value: MATCH_GT, labelKey: 'Greater than' },
    { value: MATCH_GTE, labelKey: 'Greater than or equal' },
    { value: MATCH_LT, labelKey: 'Less than' },
    { value: MATCH_LTE, labelKey: 'Less than or equal' },
  ]
}

// ---------------------------------------------------------------------------
// Editor: normalize a single condition
// ---------------------------------------------------------------------------

function isTimeFunc(value: unknown): value is TimeFunc {
  return typeof value === 'string' && TIME_FUNCS.includes(value as TimeFunc)
}

export function normalizeCondition(
  cond: Partial<RequestCondition> | null | undefined
): RequestCondition {
  let source: RequestCondition['source'] = 'param'
  if (cond?.source === 'time') {
    source = 'time'
  } else if (cond?.source === 'header') {
    source = 'header'
  } else if (cond?.source === SOURCE_TOKEN) {
    source = SOURCE_TOKEN
  }

  if (source === 'time') {
    const timeCond = cond as Partial<TimeCondition> | null | undefined
    const timeFunc: TimeFunc = isTimeFunc(timeCond?.timeFunc)
      ? timeCond.timeFunc
      : 'hour'
    const options = getRequestRuleMatchOptions(SOURCE_TIME)
    const mode = options.some((item) => item.value === timeCond?.mode)
      ? (timeCond?.mode as string)
      : MATCH_GTE
    return {
      source: 'time',
      timeFunc,
      timezone: timeCond?.timezone || 'Asia/Shanghai',
      mode,
      value: timeCond?.value == null ? '' : String(timeCond.value),
      rangeStart:
        timeCond?.rangeStart == null ? '' : String(timeCond.rangeStart),
      rangeEnd: timeCond?.rangeEnd == null ? '' : String(timeCond.rangeEnd),
    }
  }

  const phCond = cond as Partial<ParamHeaderCondition> | null | undefined
  const options = getRequestRuleMatchOptions(source)
  const mode = options.some((item) => item.value === phCond?.mode)
    ? (phCond?.mode as string)
    : MATCH_EQ
  return {
    source,
    path: phCond?.path || '',
    mode,
    value: phCond?.value == null ? '' : String(phCond.value),
  }
}

// ---------------------------------------------------------------------------
// Editor: build expression strings
// ---------------------------------------------------------------------------

function buildExprLiteral(mode: string, value: string): string {
  const text = String(value || '').trim()
  if (mode === MATCH_CONTAINS) return JSON.stringify(text)
  if (text === 'true' || text === 'false') return text
  if (NUMERIC_LITERAL_REGEX.test(text)) return text
  return JSON.stringify(text)
}

function buildTimeConditionExpr(cond: TimeCondition): string {
  const normalized = normalizeCondition(cond) as TimeCondition
  const { timeFunc, timezone, mode } = normalized
  const tz = JSON.stringify(timezone)
  const fn = `${timeFunc}(${tz})`

  if (mode === MATCH_RANGE) {
    const s = normalized.rangeStart.trim()
    const e = normalized.rangeEnd.trim()
    if (!isTimeValueInRange(timeFunc, s) || !isTimeValueInRange(timeFunc, e)) {
      return ''
    }
    // Overnight range (start > end) crosses the day boundary, e.g. 21-6.
    // A within-day range (start <= end), e.g. 9-12, must use && so the
    // condition is not a tautology that always applies the multiplier.
    const sNum = Number(s)
    const eNum = Number(e)
    if (sNum > eNum) {
      return `${fn} >= ${s} || ${fn} < ${e}`
    }
    return `${fn} >= ${s} && ${fn} < ${e}`
  }
  const v = normalized.value.trim()
  if (!isTimeValueInRange(timeFunc, v)) return ''
  const opMap: Record<string, string> = {
    [MATCH_EQ]: '==',
    [MATCH_GTE]: '>=',
    [MATCH_LT]: '<',
  }
  return `${fn} ${opMap[mode] || '=='} ${v}`
}

function buildRequestConditionExpr(cond: RequestCondition): string {
  if (cond.source === 'time') return buildTimeConditionExpr(cond)
  const normalized = normalizeCondition(cond) as ParamHeaderCondition
  const path = normalized.path.trim()
  if (!path) return ''

  if (normalized.source === SOURCE_TOKEN) {
    const numText = normalized.value.trim()
    const opMap: Record<string, string> = {
      [MATCH_EQ]: '==',
      [MATCH_GT]: '>',
      [MATCH_GTE]: '>=',
      [MATCH_LT]: '<',
      [MATCH_LTE]: '<=',
    }
    if (
      !BILLING_CONDITION_VARS.includes(path) ||
      !Object.hasOwn(opMap, cond.mode) ||
      !NUMERIC_LITERAL_REGEX.test(numText) ||
      !Number.isFinite(Number(numText))
    ) {
      return ''
    }
    return `${path} ${opMap[cond.mode]} ${numText}`
  }

  const sourceExpr =
    normalized.source === 'header'
      ? `header(${JSON.stringify(path)})`
      : `param(${JSON.stringify(path)})`

  switch (normalized.mode) {
    case MATCH_EXISTS:
      return normalized.source === 'header'
        ? `${sourceExpr} != ""`
        : `${sourceExpr} != nil`
    case MATCH_CONTAINS:
      return normalized.source === 'header'
        ? `has(${sourceExpr}, ${buildExprLiteral(normalized.mode, normalized.value)})`
        : `${sourceExpr} != nil && has(${sourceExpr}, ${buildExprLiteral(normalized.mode, normalized.value)})`
    case MATCH_GT:
    case MATCH_GTE:
    case MATCH_LT:
    case MATCH_LTE: {
      const opMap: Record<string, string> = {
        [MATCH_GT]: '>',
        [MATCH_GTE]: '>=',
        [MATCH_LT]: '<',
        [MATCH_LTE]: '<=',
      }
      const numText = String(normalized.value).trim()
      if (!NUMERIC_LITERAL_REGEX.test(numText)) return ''
      return `${sourceExpr} != nil && ${sourceExpr} ${opMap[normalized.mode]} ${numText}`
    }
    case MATCH_EQ:
    default:
      return `${sourceExpr} == ${buildExprLiteral(normalized.mode, normalized.value)}`
  }
}

function buildRuleGroupFactor(group: RequestRuleGroup): string {
  const multiplier = (group.multiplier || '').trim()
  if (!NUMERIC_LITERAL_REGEX.test(multiplier)) return ''
  const conditions = group.conditions || []
  const builtConditions = conditions.map(buildRequestConditionExpr)
  // Invalid time or token restrictions must not leave the remaining
  // conditions applying a multiplier outside its intended range.
  if (
    conditions.some(
      (condition, index) =>
        (condition.source === SOURCE_TIME ||
          condition.source === SOURCE_TOKEN) &&
        !builtConditions[index]
    )
  ) {
    return ''
  }
  const condExprs = builtConditions.filter(Boolean)
  if (condExprs.length === 0) return ''

  const combined =
    condExprs.length === 1
      ? condExprs[0]
      : condExprs.map((e) => (e.includes(' || ') ? `(${e})` : e)).join(' && ')
  return `(${combined} ? ${multiplier} : 1)`
}

export function buildRequestRuleExpr(groups: RequestRuleGroup[]): string {
  return (groups || []).map(buildRuleGroupFactor).filter(Boolean).join(' * ')
}
