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
import type { PricingNode } from './billing-expr'

type PricingValue = number | string | boolean
type PricingValueType = 'number' | 'string' | 'boolean'

export type PricingFunction = {
  parameters: ('number' | 'string')[]
  variadic?: boolean
  evaluate: (args: PricingValue[]) => number
}

type PricingEnvironment = {
  variables: ReadonlyMap<string, number>
  functions: ReadonlyMap<string, PricingFunction>
}

function validatePricingNode(
  node: PricingNode,
  env: PricingEnvironment,
  depth = 0
): PricingValueType {
  if (depth > 128) throw new Error('Pricing expression is too deep')
  switch (node.kind) {
    case 'number':
    case 'string':
      return node.kind
    case 'variable':
      if (node.name === 'true' || node.name === 'false') return 'boolean'
      if (env.variables.has(node.name)) return 'number'
      throw new Error(`${node.name} is not defined`)
    case 'call': {
      const fn = env.functions.get(node.name)
      if (!fn) throw new Error(`${node.name} is not defined`)
      const expectedCount = fn.parameters.length
      if (
        (fn.variadic && node.args.length < expectedCount) ||
        (!fn.variadic && node.args.length !== expectedCount)
      ) {
        throw new Error(`Invalid arguments for ${node.name}`)
      }
      node.args.forEach((arg, index) => {
        const expected = fn.variadic ? fn.parameters[0] : fn.parameters[index]
        if (validatePricingNode(arg, env, depth + 1) !== expected) {
          throw new Error(`Invalid arguments for ${node.name}`)
        }
      })
      return 'number'
    }
    case 'unary': {
      const expected = node.op === '!' ? 'boolean' : 'number'
      if (validatePricingNode(node.value, env, depth + 1) !== expected) {
        throw new Error(`Invalid operand for ${node.op}`)
      }
      return expected
    }
    case 'binary': {
      const left = validatePricingNode(node.left, env, depth + 1)
      const right = validatePricingNode(node.right, env, depth + 1)
      if (node.op === '&&' || node.op === '||') {
        if (left !== 'boolean' || right !== 'boolean') {
          throw new Error(`Invalid operands for ${node.op}`)
        }
        return 'boolean'
      }
      if (node.op === '==' || node.op === '!=') {
        if (left !== right) throw new Error(`Invalid operands for ${node.op}`)
        return 'boolean'
      }
      if (left !== 'number' || right !== 'number') {
        throw new Error(`Invalid operands for ${node.op}`)
      }
      if (['<', '<=', '>', '>='].includes(node.op)) return 'boolean'
      return 'number'
    }
    case 'conditional': {
      if (validatePricingNode(node.condition, env, depth + 1) !== 'boolean') {
        throw new Error('Pricing condition must be boolean')
      }
      const yes = validatePricingNode(node.yes, env, depth + 1)
      const no = validatePricingNode(node.no, env, depth + 1)
      if (yes !== no) {
        throw new Error('Pricing branches must have the same type')
      }
      return yes
    }
  }
}

function evaluatePricingNode(
  node: PricingNode,
  env: PricingEnvironment
): PricingValue {
  switch (node.kind) {
    case 'number':
    case 'string':
      return node.value
    case 'variable':
      if (node.name === 'true') return true
      if (node.name === 'false') return false
      return env.variables.get(node.name) as number
    case 'call': {
      const fn = env.functions.get(node.name) as PricingFunction
      return fn.evaluate(node.args.map((arg) => evaluatePricingNode(arg, env)))
    }
    case 'unary': {
      const value = evaluatePricingNode(node.value, env)
      if (node.op === '!') return !value
      if (node.op === '-') return -(value as number)
      return value
    }
    case 'conditional':
      return evaluatePricingNode(
        evaluatePricingNode(node.condition, env) ? node.yes : node.no,
        env
      )
    case 'binary': {
      const left = evaluatePricingNode(node.left, env)
      // Preserve expr-lang's lazy branches, including time functions and tier().
      if (node.op === '&&') return left && evaluatePricingNode(node.right, env)
      if (node.op === '||') return left || evaluatePricingNode(node.right, env)
      const right = evaluatePricingNode(node.right, env)
      if (node.op === '==') return left === right
      if (node.op === '!=') return left !== right
      const a = left as number
      const b = right as number
      switch (node.op) {
        case '+':
          return a + b
        case '-':
          return a - b
        case '*':
          return a * b
        case '/':
          return a / b
        case '%':
          return a % b
        case '<':
          return a < b
        case '<=':
          return a <= b
        case '>':
          return a > b
        case '>=':
          return a >= b
        default:
          throw new Error('Unsupported pricing operator')
      }
    }
  }
}

/** Interpret a complete validated AST. There is no JavaScript evaluation or object access. */
export function evaluatePricingAst(
  node: PricingNode,
  env: PricingEnvironment
): number {
  // Validate *both* branches before executing anything, even if one is unused.
  if (validatePricingNode(node, env) !== 'number') {
    throw new Error('Pricing expression must return a number')
  }
  const result = evaluatePricingNode(node, env) as number
  if (!Number.isFinite(result) || result < 0) {
    throw new Error(
      'Pricing expression must return a finite non-negative number'
    )
  }
  return result
}
