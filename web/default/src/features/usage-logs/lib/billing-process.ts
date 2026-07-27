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

const BILLING_VARIABLES = [
  'p',
  'c',
  'len',
  'cr',
  'cc',
  'cc1h',
  'img',
  'img_o',
  'ai',
  'ao',
] as const

export type BillingVariable = (typeof BILLING_VARIABLES)[number]
export type BillingTermKind =
  | 'input'
  | 'output'
  | 'input_length'
  | 'cache_read'
  | 'cache_creation'
  | 'cache_creation_5m'
  | 'cache_creation_1h'
  | 'image_input'
  | 'image_output'
  | 'audio_input'
  | 'audio_output'

export interface BillingProcessTerm {
  kind: BillingTermKind
  variable: BillingVariable
  tokens: number
  unitPriceUsdPerMillion: number
  subtotalUsd: number
}

export interface BillingProcessLogInput {
  prompt_tokens?: unknown
  completion_tokens?: unknown
  quota?: unknown
}

export interface BillingProcessOtherInput {
  billing_mode?: unknown
  expr_b64?: unknown
  matched_tier?: unknown
  claude?: unknown
  model_price?: unknown
  model_ratio?: unknown
  completion_ratio?: unknown
  group_ratio?: unknown
  user_group_ratio?: unknown
  cache_tokens?: unknown
  cache_ratio?: unknown
  cache_creation_tokens?: unknown
  cache_creation_ratio?: unknown
  cache_creation_tokens_5m?: unknown
  cache_creation_ratio_5m?: unknown
  cache_creation_tokens_1h?: unknown
  cache_creation_ratio_1h?: unknown
  is_task?: unknown
  task_id?: unknown
  ws?: unknown
  audio?: unknown
  audio_input?: unknown
  audio_output?: unknown
  audio_input_seperate_price?: unknown
  audio_input_token_count?: unknown
  image?: unknown
  image_output?: unknown
  web_search?: unknown
  web_search_call_count?: unknown
  file_search?: unknown
  file_search_call_count?: unknown
  image_generation_call?: unknown
  image_generation_call_price?: unknown
  admin_info?: {
    local_count_tokens?: unknown
    quota_saturation?: unknown
  }
}

export interface BillingProcessInput {
  log: BillingProcessLogInput
  other: BillingProcessOtherInput
  quotaPerUnit: number
}

export type BillingFailureReason =
  | 'invalid_input'
  | 'invalid_quota_per_unit'
  | 'invalid_token_value'
  | 'invalid_group_ratio'
  | 'unsupported_task'
  | 'unsupported_audio'
  | 'unsupported_image'
  | 'unsupported_tool_surcharge'
  | 'unsupported_saturation'
  | 'invalid_expression_encoding'
  | 'unsupported_expression'
  | 'matched_tier_missing'
  | 'matched_tier_not_found'
  | 'matched_tier_ambiguous'
  | 'missing_token_dimension'
  | 'unsupported_per_call'
  | 'invalid_ratio'
  | 'unsupported_cache_layout'
  | 'invalid_calculation'
  | 'quota_mismatch'

export interface BillingProcessFailure {
  ok: false
  reason: BillingFailureReason
}

export interface BillingProcessSuccess {
  ok: true
  mode: 'tiered_expr' | 'ratio_openai' | 'ratio_claude'
  isClaude: boolean
  matchedTier: string | null
  expressionVersion: number | null
  usedVariables: BillingVariable[]
  tokens: Record<BillingVariable, number>
  terms: BillingProcessTerm[]
  expressionOutput: number
  totalUsdBeforeGroup: number
  effectiveGroupRatio: number
  groupRatioSource: 'user' | 'group'
  totalUsdAfterGroup: number
  quotaPerUnit: number
  quotaBeforeRound: number
  quota: number
}

export type BillingProcessResult = BillingProcessFailure | BillingProcessSuccess

interface ParsedTerm {
  variable: BillingVariable
  coefficient: number
}

interface ParsedTier {
  label: string
  terms: ParsedTerm[]
}

interface ParsedExpression {
  version: number
  tiers: ParsedTier[]
  usedVariables: BillingVariable[]
}

interface CommonInput {
  ok: true
  promptTokens: number
  completionTokens: number
  loggedQuota: number
  quotaPerUnit: number
  group: { ratio: number; source: 'user' | 'group' }
}

const BILLING_VARIABLE_SET = new Set<string>(BILLING_VARIABLES)
const NUMBER_SOURCE = '[+-]?(?:\\d+(?:\\.\\d*)?|\\.\\d+)(?:[eE][+-]?\\d+)?'
const VARIABLE_SOURCE = `(?:${[...BILLING_VARIABLES]
  .sort((left, right) => right.length - left.length)
  .join('|')})(?![A-Za-z0-9_])`
const MAX_QUOTA = 2147483647
const MIN_QUOTA = -2147483648

const TERM_KIND: Record<BillingVariable, BillingTermKind> = {
  p: 'input',
  c: 'output',
  len: 'input_length',
  cr: 'cache_read',
  cc: 'cache_creation',
  cc1h: 'cache_creation_1h',
  img: 'image_input',
  img_o: 'image_output',
  ai: 'audio_input',
  ao: 'audio_output',
}

function failure(reason: BillingFailureReason): BillingProcessFailure {
  return { ok: false, reason }
}

function cleanNumber(value: number): number {
  if (!Number.isFinite(value)) return value
  return Number(value.toPrecision(15))
}

function isNonNegativeInteger(value: unknown): value is number {
  return typeof value === 'number' && Number.isSafeInteger(value) && value >= 0
}

function readOptionalToken(value: unknown): number | null {
  if (value == null) return 0
  return isNonNegativeInteger(value) ? value : null
}

function readRequiredToken(value: unknown): number | null {
  return isNonNegativeInteger(value) ? value : null
}

function readNonNegativeNumber(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
    ? value
    : null
}

function resolveGroupRatio(
  other: BillingProcessOtherInput
): CommonInput['group'] | null {
  const userRatio = other.user_group_ratio
  if (
    typeof userRatio === 'number' &&
    Number.isFinite(userRatio) &&
    userRatio !== -1
  ) {
    if (userRatio < 0) return null
    return { ratio: userRatio, source: 'user' }
  }

  const groupRatio = readNonNegativeNumber(other.group_ratio)
  if (groupRatio == null) return null
  return { ratio: groupRatio, source: 'group' }
}

function quotaRound(value: number): number {
  const rounded = value < 0 ? Math.ceil(value - 0.5) : Math.floor(value + 0.5)
  if (rounded >= MAX_QUOTA) return MAX_QUOTA
  if (rounded <= MIN_QUOTA) return MIN_QUOTA
  return rounded
}

function skipWhitespace(source: string, start: number): number {
  let index = start
  while (index < source.length && /\s/.test(source[index])) index += 1
  return index
}

function scanQuotedString(source: string, start: number): number {
  if (source[start] !== '"') return -1
  let index = start + 1
  while (index < source.length) {
    if (source[index] === '\\') {
      index += 2
      continue
    }
    if (source[index] === '"') return index + 1
    index += 1
  }
  return -1
}

function parseLinearTerms(source: string): ParsedTerm[] | null {
  const terms: ParsedTerm[] = []
  const termRegex = new RegExp(
    `(?:(${VARIABLE_SOURCE})\\s*\\*\\s*(${NUMBER_SOURCE})|(${NUMBER_SOURCE})\\s*\\*\\s*(${VARIABLE_SOURCE}))`,
    'y'
  )
  let index = skipWhitespace(source, 0)

  while (index < source.length) {
    termRegex.lastIndex = index
    const match = termRegex.exec(source)
    if (!match) return null

    const variable = match[1] ?? match[4]
    const coefficient = Number(match[2] ?? match[3])
    if (
      !variable ||
      !BILLING_VARIABLE_SET.has(variable) ||
      !Number.isFinite(coefficient) ||
      coefficient < 0
    ) {
      return null
    }
    terms.push({ variable: variable as BillingVariable, coefficient })

    index = skipWhitespace(source, termRegex.lastIndex)
    if (index === source.length) break
    if (source[index] !== '+') return null
    index = skipWhitespace(source, index + 1)
    if (index === source.length) return null
  }

  return terms.length > 0 ? terms : null
}

function parseTierCall(
  source: string,
  start: number
): (ParsedTier & { end: number }) | null {
  let index = skipWhitespace(source, start + 4)
  if (source[index] !== '(') return null
  index = skipWhitespace(source, index + 1)

  const labelEnd = scanQuotedString(source, index)
  if (labelEnd < 0) return null

  let label: unknown
  try {
    label = JSON.parse(source.slice(index, labelEnd))
  } catch {
    return null
  }
  if (typeof label !== 'string') return null

  index = skipWhitespace(source, labelEnd)
  if (source[index] !== ',') return null
  const bodyStart = index + 1
  let depth = 1
  index = bodyStart

  while (index < source.length) {
    if (source[index] === '"') {
      const stringEnd = scanQuotedString(source, index)
      if (stringEnd < 0) return null
      index = stringEnd
      continue
    }
    if (source[index] === '(') depth += 1
    if (source[index] === ')') {
      depth -= 1
      if (depth === 0) {
        const terms = parseLinearTerms(source.slice(bodyStart, index))
        if (!terms) return null
        return { label, terms, end: index + 1 }
      }
    }
    index += 1
  }

  return null
}

function parseCondition(source: string): Set<BillingVariable> | null {
  const variables = new Set<BillingVariable>()
  const comparisonRegex = new RegExp(
    `(?:(${VARIABLE_SOURCE})\\s*(?:<=|>=|==|!=|<|>)\\s*${NUMBER_SOURCE}|${NUMBER_SOURCE}\\s*(?:<=|>=|==|!=|<|>)\\s*(${VARIABLE_SOURCE}))`,
    'y'
  )
  let index = skipWhitespace(source, 0)

  while (index < source.length) {
    comparisonRegex.lastIndex = index
    const match = comparisonRegex.exec(source)
    if (!match) return null
    const variable = match[1] ?? match[2]
    if (!variable || !BILLING_VARIABLE_SET.has(variable)) return null
    variables.add(variable as BillingVariable)

    index = skipWhitespace(source, comparisonRegex.lastIndex)
    if (index === source.length) break
    const operator = source.slice(index, index + 2)
    if (operator !== '&&' && operator !== '||') return null
    index = skipWhitespace(source, index + 2)
    if (index === source.length) return null
  }

  return variables.size > 0 ? variables : null
}

function findMatchingTernaryColon(
  source: string,
  questionIndex: number
): number {
  let depth = 0
  for (let index = questionIndex + 1; index < source.length; index += 1) {
    if (source[index] === '?') depth += 1
    if (source[index] === ':') {
      if (depth === 0) return index
      depth -= 1
    }
  }
  return -1
}

function validateTierSkeleton(source: string): Set<BillingVariable> | null {
  const trimmed = source.trim()
  if (trimmed === 'T') return new Set()

  const questionIndex = trimmed.indexOf('?')
  if (questionIndex <= 0) return null
  const colonIndex = findMatchingTernaryColon(trimmed, questionIndex)
  if (colonIndex < 0) return null

  const conditionVars = parseCondition(trimmed.slice(0, questionIndex))
  const truthyVars = validateTierSkeleton(
    trimmed.slice(questionIndex + 1, colonIndex)
  )
  const falsyVars = validateTierSkeleton(trimmed.slice(colonIndex + 1))
  if (!conditionVars || !truthyVars || !falsyVars) return null

  return new Set([...conditionVars, ...truthyVars, ...falsyVars])
}

function stripExpressionVersion(
  expression: string
): { version: number; body: string } | null {
  const trimmed = expression.trim()
  const match = /^v(\d+):/.exec(trimmed)
  if (!match) return { version: 1, body: trimmed }
  if (Number(match[1]) !== 1) return null
  return { version: 1, body: trimmed.slice(match[0].length).trim() }
}

function parseTieredExpression(expression: string): ParsedExpression | null {
  const versioned = stripExpressionVersion(expression)
  if (!versioned || !versioned.body) return null

  const tiers: ParsedTier[] = []
  let skeleton = ''
  let lastIndex = 0
  let index = 0

  while (index < versioned.body.length) {
    if (versioned.body[index] === '"') {
      const stringEnd = scanQuotedString(versioned.body, index)
      if (stringEnd < 0) return null
      index = stringEnd
      continue
    }

    if (/[A-Za-z_]/.test(versioned.body[index])) {
      let identifierEnd = index + 1
      while (
        identifierEnd < versioned.body.length &&
        /[A-Za-z0-9_]/.test(versioned.body[identifierEnd])
      ) {
        identifierEnd += 1
      }
      const identifier = versioned.body.slice(index, identifierEnd)
      if (identifier === 'tier') {
        const tier = parseTierCall(versioned.body, index)
        if (!tier) return null
        skeleton += `${versioned.body.slice(lastIndex, index)}T`
        tiers.push({ label: tier.label, terms: tier.terms })
        lastIndex = tier.end
        index = tier.end
        continue
      }
      index = identifierEnd
      continue
    }
    index += 1
  }

  skeleton += versioned.body.slice(lastIndex)
  if (tiers.length === 0) return null

  const conditionVars = validateTierSkeleton(skeleton)
  if (!conditionVars) return null

  const used = new Set<BillingVariable>(conditionVars)
  for (const tier of tiers) {
    for (const term of tier.terms) used.add(term.variable)
  }

  return {
    version: versioned.version,
    tiers,
    usedVariables: BILLING_VARIABLES.filter((variable) => used.has(variable)),
  }
}

function decodeBase64Utf8(value: unknown): string | null {
  if (typeof value !== 'string' || value === '') return null
  try {
    const binary = globalThis.atob(value)
    const bytes = Uint8Array.from(binary, (character) =>
      character.charCodeAt(0)
    )
    return new TextDecoder().decode(bytes)
  } catch {
    return null
  }
}

function readCommonInput(
  input: BillingProcessInput | null | undefined
): CommonInput | BillingProcessFailure {
  if (!input || !input.log || !input.other) return failure('invalid_input')
  if (!Number.isFinite(input.quotaPerUnit) || input.quotaPerUnit <= 0) {
    return failure('invalid_quota_per_unit')
  }

  const promptTokens = readRequiredToken(input.log.prompt_tokens)
  const completionTokens = readRequiredToken(input.log.completion_tokens)
  const loggedQuota = readRequiredToken(input.log.quota)
  if (promptTokens == null || completionTokens == null || loggedQuota == null) {
    return failure('invalid_token_value')
  }

  const group = resolveGroupRatio(input.other)
  if (!group) return failure('invalid_group_ratio')

  return {
    ok: true,
    promptTokens,
    completionTokens,
    loggedQuota,
    quotaPerUnit: input.quotaPerUnit,
    group,
  }
}

function unsupportedPath(
  other: BillingProcessOtherInput
): BillingFailureReason | null {
  if (other.is_task === true || other.task_id != null) return 'unsupported_task'
  if (
    other.ws === true ||
    other.audio === true ||
    other.audio_input_seperate_price === true ||
    (typeof other.audio_input === 'number' && other.audio_input > 0) ||
    (typeof other.audio_output === 'number' && other.audio_output > 0) ||
    (typeof other.audio_input_token_count === 'number' &&
      other.audio_input_token_count > 0)
  ) {
    return 'unsupported_audio'
  }
  if (
    other.image === true ||
    (typeof other.image_output === 'number' && other.image_output > 0)
  ) {
    return 'unsupported_image'
  }
  if (
    other.web_search === true ||
    (typeof other.web_search_call_count === 'number' &&
      other.web_search_call_count > 0) ||
    other.file_search === true ||
    (typeof other.file_search_call_count === 'number' &&
      other.file_search_call_count > 0) ||
    other.image_generation_call === true ||
    (typeof other.image_generation_call_price === 'number' &&
      other.image_generation_call_price > 0)
  ) {
    return 'unsupported_tool_surcharge'
  }
  if (other.admin_info?.quota_saturation) return 'unsupported_saturation'
  return null
}

function readCacheTokens(other: BillingProcessOtherInput): {
  cr: number
  totalCreation: number
  creation5m: number
  creation1h: number
} | null {
  const cr = readOptionalToken(other.cache_tokens)
  const totalCreation = readOptionalToken(other.cache_creation_tokens)
  const creation5m = readOptionalToken(other.cache_creation_tokens_5m)
  const creation1h = readOptionalToken(other.cache_creation_tokens_1h)
  if (
    cr == null ||
    totalCreation == null ||
    creation5m == null ||
    creation1h == null
  ) {
    return null
  }
  return { cr, totalCreation, creation5m, creation1h }
}

function makeTerm(
  kind: BillingTermKind,
  variable: BillingVariable,
  tokens: number,
  unitPriceUsdPerMillion: number
): BillingProcessTerm {
  return {
    kind,
    variable,
    tokens,
    unitPriceUsdPerMillion,
    subtotalUsd: cleanNumber((tokens * unitPriceUsdPerMillion) / 1_000_000),
  }
}

function finalizeResult(input: {
  common: CommonInput
  mode: BillingProcessSuccess['mode']
  isClaude: boolean
  matchedTier: string | null
  expressionVersion: number | null
  usedVariables: BillingVariable[]
  tokens: Record<BillingVariable, number>
  terms: BillingProcessTerm[]
  minimumOne: boolean
  nonZeroBillingRatio?: boolean
  localCountTokens?: boolean
}): BillingProcessResult {
  let expressionOutput = 0
  for (const term of input.terms) {
    expressionOutput += term.tokens * term.unitPriceUsdPerMillion
  }
  if (!Number.isFinite(expressionOutput) || expressionOutput < 0) {
    return failure('invalid_calculation')
  }

  const totalUsdBeforeGroup = expressionOutput / 1_000_000
  const quotaBeforeGroup = totalUsdBeforeGroup * input.common.quotaPerUnit
  const quotaBeforeRound = quotaBeforeGroup * input.common.group.ratio
  let quota = quotaRound(quotaBeforeRound)
  if (
    input.minimumOne &&
    input.common.promptTokens + input.common.completionTokens > 0 &&
    input.nonZeroBillingRatio === true &&
    quota === 0 &&
    input.localCountTokens !== true
  ) {
    quota = 1
  }

  if (quota !== input.common.loggedQuota) return failure('quota_mismatch')

  return {
    ok: true,
    mode: input.mode,
    isClaude: input.isClaude,
    matchedTier: input.matchedTier,
    expressionVersion: input.expressionVersion,
    usedVariables: input.usedVariables,
    tokens: input.tokens,
    terms: input.terms.map((term) => ({
      ...term,
      unitPriceUsdPerMillion: cleanNumber(term.unitPriceUsdPerMillion),
    })),
    expressionOutput: cleanNumber(expressionOutput),
    totalUsdBeforeGroup: cleanNumber(totalUsdBeforeGroup),
    effectiveGroupRatio: input.common.group.ratio,
    groupRatioSource: input.common.group.source,
    totalUsdAfterGroup: cleanNumber(
      totalUsdBeforeGroup * input.common.group.ratio
    ),
    quotaPerUnit: input.common.quotaPerUnit,
    quotaBeforeRound: cleanNumber(quotaBeforeRound),
    quota,
  }
}

function reconstructTiered(
  input: BillingProcessInput,
  common: CommonInput
): BillingProcessResult {
  const expression = decodeBase64Utf8(input.other.expr_b64)
  if (expression == null) return failure('invalid_expression_encoding')

  const parsed = parseTieredExpression(expression)
  if (!parsed) return failure('unsupported_expression')
  if (
    typeof input.other.matched_tier !== 'string' ||
    input.other.matched_tier === ''
  ) {
    return failure('matched_tier_missing')
  }

  const matchedTier = input.other.matched_tier
  const matches = parsed.tiers.filter((tier) => tier.label === matchedTier)
  if (matches.length === 0) return failure('matched_tier_not_found')
  if (matches.length > 1) return failure('matched_tier_ambiguous')

  if (
    parsed.usedVariables.some((variable) =>
      ['img', 'img_o', 'ai', 'ao'].includes(variable)
    )
  ) {
    return failure('missing_token_dimension')
  }

  const cache = readCacheTokens(input.other)
  if (!cache) return failure('invalid_token_value')
  const isClaude = input.other.claude === true
  const hasSplitCreation = cache.creation5m > 0 || cache.creation1h > 0
  const cc =
    isClaude && hasSplitCreation ? cache.creation5m : cache.totalCreation
  const cc1h = isClaude ? cache.creation1h : 0

  let p = common.promptTokens
  let c = common.completionTokens
  if (!isClaude) {
    if (parsed.usedVariables.includes('cr')) p -= cache.cr
    if (parsed.usedVariables.includes('cc')) p -= cc
    if (parsed.usedVariables.includes('cc1h')) p -= cc1h
  }
  p = Math.max(0, p)
  c = Math.max(0, c)

  const tokens: Record<BillingVariable, number> = {
    p,
    c,
    len: isClaude
      ? common.promptTokens + cache.cr + cc + cc1h
      : common.promptTokens,
    cr: cache.cr,
    cc,
    cc1h,
    img: 0,
    img_o: 0,
    ai: 0,
    ao: 0,
  }
  const terms = matches[0].terms.map((term) =>
    makeTerm(
      TERM_KIND[term.variable],
      term.variable,
      tokens[term.variable],
      term.coefficient
    )
  )

  return finalizeResult({
    common,
    mode: 'tiered_expr',
    isClaude,
    matchedTier,
    expressionVersion: parsed.version,
    usedVariables: parsed.usedVariables,
    tokens,
    terms,
    minimumOne: false,
  })
}

function requiredRatio(value: unknown): number | null {
  return readNonNegativeNumber(value)
}

function reconstructRatio(
  input: BillingProcessInput,
  common: CommonInput
): BillingProcessResult {
  if (input.other.model_price !== -1) return failure('unsupported_per_call')

  const modelRatio = requiredRatio(input.other.model_ratio)
  const completionRatio = requiredRatio(input.other.completion_ratio)
  if (modelRatio == null || completionRatio == null) {
    return failure('invalid_ratio')
  }

  const cache = readCacheTokens(input.other)
  if (!cache) return failure('invalid_token_value')
  const isClaude = input.other.claude === true
  if (!isClaude && (cache.creation5m > 0 || cache.creation1h > 0)) {
    return failure('unsupported_cache_layout')
  }

  const cacheRatio = cache.cr > 0 ? requiredRatio(input.other.cache_ratio) : 0
  const creationRatio =
    cache.totalCreation > 0
      ? requiredRatio(input.other.cache_creation_ratio)
      : 0
  if (cacheRatio == null || creationRatio == null) {
    return failure('invalid_ratio')
  }

  const hasSplitCreation =
    isClaude && (cache.creation5m > 0 || cache.creation1h > 0)
  const creation5mRatio =
    cache.creation5m > 0
      ? requiredRatio(input.other.cache_creation_ratio_5m)
      : 0
  const creation1hRatio =
    cache.creation1h > 0
      ? requiredRatio(input.other.cache_creation_ratio_1h)
      : 0
  if (creation5mRatio == null || creation1hRatio == null) {
    return failure('invalid_ratio')
  }

  const genericCreationTokens = hasSplitCreation
    ? Math.max(0, cache.totalCreation - cache.creation5m - cache.creation1h)
    : cache.totalCreation
  const p = isClaude
    ? common.promptTokens
    : Math.max(0, common.promptTokens - cache.cr - cache.totalCreation)
  const cc = hasSplitCreation ? cache.creation5m : cache.totalCreation
  const cc1h = isClaude ? cache.creation1h : 0
  const tokens: Record<BillingVariable, number> = {
    p,
    c: common.completionTokens,
    len: isClaude
      ? common.promptTokens + cache.cr + cc + cc1h
      : common.promptTokens,
    cr: cache.cr,
    cc,
    cc1h,
    img: 0,
    img_o: 0,
    ai: 0,
    ao: 0,
  }

  const baseInputPrice = (modelRatio * 1_000_000) / common.quotaPerUnit
  const terms: BillingProcessTerm[] = [
    makeTerm('input', 'p', p, baseInputPrice),
  ]
  if (cache.cr > 0) {
    terms.push(
      makeTerm('cache_read', 'cr', cache.cr, baseInputPrice * cacheRatio)
    )
  }
  if (genericCreationTokens > 0) {
    terms.push(
      makeTerm(
        'cache_creation',
        'cc',
        genericCreationTokens,
        baseInputPrice * creationRatio
      )
    )
  }
  if (hasSplitCreation && cache.creation5m > 0) {
    terms.push(
      makeTerm(
        'cache_creation_5m',
        'cc',
        cache.creation5m,
        baseInputPrice * creation5mRatio
      )
    )
  }
  if (hasSplitCreation && cache.creation1h > 0) {
    terms.push(
      makeTerm(
        'cache_creation_1h',
        'cc1h',
        cache.creation1h,
        baseInputPrice * creation1hRatio
      )
    )
  }
  terms.push(
    makeTerm(
      'output',
      'c',
      common.completionTokens,
      baseInputPrice * completionRatio
    )
  )

  const used = new Set(terms.map((term) => term.variable))
  return finalizeResult({
    common,
    mode: isClaude ? 'ratio_claude' : 'ratio_openai',
    isClaude,
    matchedTier: null,
    expressionVersion: null,
    usedVariables: BILLING_VARIABLES.filter((variable) => used.has(variable)),
    tokens,
    terms,
    minimumOne: true,
    nonZeroBillingRatio: modelRatio * common.group.ratio !== 0,
    localCountTokens: input.other.admin_info?.local_count_tokens === true,
  })
}

export function reconstructBillingProcess(
  input: BillingProcessInput | null | undefined
): BillingProcessResult {
  const common = readCommonInput(input)
  if (!common.ok) return common
  if (!input) return failure('invalid_input')

  const unsupportedReason = unsupportedPath(input.other)
  if (unsupportedReason) return failure(unsupportedReason)

  if (input.other.billing_mode === 'tiered_expr') {
    return reconstructTiered(input, common)
  }
  return reconstructRatio(input, common)
}
