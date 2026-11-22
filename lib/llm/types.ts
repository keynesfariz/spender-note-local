import "server-only"
import * as v from "valibot"
import { DEFAULT_CATEGORIES } from "../categories"
import {
  CURRENCY_EXPONENTS,
  dateTimeInputToUtcIso,
  normalizeDecimalAmountString,
  utcToDateInputValue,
} from "../format"
import { getCountryPromptRules } from "./country-rules"

export type AccountType = "credit_card" | "debit_card" | "digital_wallet"

export class LlmRuntimeError extends Error {
  constructor(message: string, options?: { cause?: unknown }) {
    super(message, options)
    this.name = "LlmRuntimeError"
  }
}

export class LlmExtractionError extends Error {
  constructor(message: string, options?: { cause?: unknown }) {
    super(message, options)
    this.name = "LlmExtractionError"
  }
}

export type EmailKind = "card_charge" | "receipt" | "non_transaction"

export interface ExtractionAccountContext {
  id: string
  name: string
  type: AccountType
  identifiers: readonly string[]
}

export interface ExtractTransactionInput {
  emailId: string
  fromAddress: string
  subject: string
  cleanedText: string
  receivedAtUtc: string
  userCountry?: string | null
  countryPromptRules?: readonly string[]
  userTimezone: string
  userCurrency?: string | null
  categories?: readonly string[]
  accounts?: readonly ExtractionAccountContext[]
}

export interface RawLlmExtraction {
  email_kind: EmailKind
  amount_decimal: string | null
  currency: string | null
  transaction_date: string | null
  transaction_time: string | null
  timezone_offset: string | null
  merchant: string | null
  category: string | null
  card_last4: string | null
  payment_method?: string | null
  bank_account_hint?: string | null
  matched_account_id?: string | null
  account_type_guess?: AccountType | null
  reference_code: string | null
}

export type ExtractTransactionResult =
  | {
      emailKind: "non_transaction"
      raw: RawLlmExtraction
      rawJson: string
    }
  | {
      emailKind: "card_charge" | "receipt"
      amountMinor: number
      currency: string
      occurredAtUtc: string
      merchantRaw: string
      category: string
      cardLast4: string | null
      paymentMethodRaw?: string | null
      bankAccountHint?: string | null
      matchedAccountId?: string | null
      accountTypeGuess?: AccountType | null
      referenceCode: string | null
      raw: RawLlmExtraction
      rawJson: string
    }

export interface ModelCheckResult {
  ok: true
  modelPath: string
  contextSize: number
}

export interface ExtractTransactionCallOptions {
  timeoutMs?: number
  categories?: readonly string[]
}

export interface StructuredExtractor {
  extractTransaction(
    input: ExtractTransactionInput,
    signal?: AbortSignal,
    options?: ExtractTransactionCallOptions
  ): Promise<ExtractTransactionResult>
  checkModel(): Promise<ModelCheckResult>
  dispose(): Promise<void>
}

export interface LlamaContextSequenceLike {
  dispose(): Promise<void>
}

export interface LlamaContextLike {
  readonly disposed?: boolean
  getSequence(): LlamaContextSequenceLike
  dispose(): Promise<void>
}

export interface LlamaModelLike {
  readonly disposed?: boolean
  createContext(options: { contextSize: number }): Promise<LlamaContextLike>
  dispose(): Promise<void>
}

export interface LlamaChatSessionLike {
  prompt(
    prompt: string,
    options?: {
      grammar?: unknown
      temperature?: number
      signal?: AbortSignal
      maxTokens?: number
    }
  ): Promise<string>
  dispose?(): void
}

export interface LlamaLike {
  loadModel(options: { modelPath: string }): Promise<LlamaModelLike>
  createGrammarForJsonSchema(schema: ExtractionJsonSchema): Promise<unknown>
  createChatSession?(options: {
    contextSequence: LlamaContextSequenceLike
    systemPrompt: string
    autoDisposeSequence?: boolean
  }): LlamaChatSessionLike
}

export type ExtractionJsonSchema = {
  readonly type: "object"
  readonly additionalProperties: false
  readonly properties: {
    readonly email_kind: {
      readonly enum: readonly ["card_charge", "receipt", "non_transaction"]
    }
    readonly amount_decimal: { readonly type: readonly ["string", "null"] }
    readonly currency: { readonly type: readonly ["string", "null"] }
    readonly transaction_date: { readonly type: readonly ["string", "null"] }
    readonly transaction_time: { readonly type: readonly ["string", "null"] }
    readonly timezone_offset: { readonly type: readonly ["string", "null"] }
    readonly merchant: { readonly type: readonly ["string", "null"] }
    readonly category: { readonly enum: readonly (string | null)[] }
    readonly card_last4: { readonly type: readonly ["string", "null"] }
    readonly payment_method: { readonly type: readonly ["string", "null"] }
    readonly bank_account_hint: { readonly type: readonly ["string", "null"] }
    readonly matched_account_id:
      | { readonly enum: readonly (string | null)[] }
      | { readonly type: readonly ["string", "null"] }
    readonly account_type_guess: {
      readonly enum: readonly (AccountType | null)[]
    }
    readonly reference_code: { readonly type: readonly ["string", "null"] }
  }
  readonly required: readonly string[]
}

function normalizeCurrencyCode(currency: string): string {
  const code = currency.trim().toUpperCase()
  if (!/^[A-Z]{3}$/.test(code)) {
    throw new Error(`Invalid ISO 4217 currency code: ${currency}`)
  }
  return code
}

function getCurrencyExponent(currency: string): number {
  const code = normalizeCurrencyCode(currency)
  return CURRENCY_EXPONENTS[code] ?? 2
}

function parseAmountToMinor(rawInput: string, currency: string): number {
  const exponent = getCurrencyExponent(currency)
  const raw = rawInput.trim()
  if (!raw) throw new Error("Amount cannot be empty")
  const cleaned = normalizeDecimalAmountString(raw, exponent)
  const match = /^(-)?(\d+)(?:\.(\d+))?$/.exec(cleaned)
  if (!match) throw new Error(`Invalid decimal amount: ${raw}`)
  if (match[1]) throw new Error("Amount cannot be negative")
  const fracPart = match[3] ?? ""
  if (fracPart.length > exponent) {
    throw new Error(`Too many decimal places for ${currency} (max ${exponent})`)
  }
  const paddedFrac = fracPart.padEnd(exponent, "0")
  const minor = Number(`${match[2]}${paddedFrac}`)
  if (!Number.isSafeInteger(minor) || minor <= 0) {
    throw new Error(`Amount must be greater than zero: ${raw}`)
  }
  return minor
}

function toUtcIsoSeconds(input: string | Date): string {
  const d = input instanceof Date ? input : new Date(input)
  if (Number.isNaN(d.getTime())) {
    throw new Error(`Invalid timestamp: ${String(input)}`)
  }
  return d.toISOString().slice(0, 19) + "Z"
}

function parseRawExtractedDateTimeToUtc(params: {
  transactionDate: string | null | undefined
  transactionTime?: string | null
  timezoneOffset?: string | null
  userTimezone: string
  receivedAtUtc: string
}): string {
  const {
    transactionDate,
    transactionTime,
    timezoneOffset,
    userTimezone,
    receivedAtUtc,
  } = params
  if (!transactionDate || !/^\d{4}-\d{2}-\d{2}$/.test(transactionDate.trim())) {
    return toUtcIsoSeconds(receivedAtUtc)
  }
  const cleanDate = transactionDate.trim()
  const cleanTime = transactionTime?.trim() || null
  const cleanOffset = timezoneOffset?.trim() || null

  if (cleanTime && /^\d{2}:\d{2}(:\d{2})?$/.test(cleanTime)) {
    const normalizedTime =
      cleanTime.length === 5 ? `${cleanTime}:00` : cleanTime
    if (cleanOffset && /^(Z|[+-]\d{2}:\d{2})$/.test(cleanOffset)) {
      return toUtcIsoSeconds(`${cleanDate}T${normalizedTime}${cleanOffset}`)
    }
    return dateTimeInputToUtcIso(cleanDate, normalizedTime, userTimezone)
  }

  const receivedLocalDate = utcToDateInputValue(receivedAtUtc, userTimezone)
  if (cleanDate === receivedLocalDate) {
    return toUtcIsoSeconds(receivedAtUtc)
  }
  return dateTimeInputToUtcIso(cleanDate, "12:00:00", userTimezone)
}

export function resolveCategories(
  categories?: readonly string[]
): readonly string[] {
  const filtered = (categories ?? []).map((c) => c.trim()).filter(Boolean)
  return filtered.length > 0 ? filtered : DEFAULT_CATEGORIES
}

export function buildExtractionJsonSchema(
  categories?: readonly string[],
  accountIds?: readonly string[]
): ExtractionJsonSchema {
  const validCategories = resolveCategories(categories)
  const validAccountIds = (accountIds ?? [])
    .map((id) => id.trim())
    .filter(Boolean)
  return {
    type: "object",
    additionalProperties: false,
    properties: {
      email_kind: {
        enum: ["card_charge", "receipt", "non_transaction"],
      },
      amount_decimal: { type: ["string", "null"] },
      currency: { type: ["string", "null"] },
      transaction_date: { type: ["string", "null"] },
      transaction_time: { type: ["string", "null"] },
      timezone_offset: { type: ["string", "null"] },
      merchant: { type: ["string", "null"] },
      category: {
        enum: [...validCategories, null],
      },
      card_last4: { type: ["string", "null"] },
      payment_method: { type: ["string", "null"] },
      bank_account_hint: { type: ["string", "null"] },
      matched_account_id:
        validAccountIds.length > 0
          ? { enum: [...validAccountIds, null] }
          : { type: ["string", "null"] },
      account_type_guess: {
        enum: ["credit_card", "debit_card", "digital_wallet", null],
      },
      reference_code: { type: ["string", "null"] },
    },
    required: [
      "email_kind",
      "amount_decimal",
      "currency",
      "transaction_date",
      "transaction_time",
      "timezone_offset",
      "merchant",
      "category",
      "card_last4",
      "payment_method",
      "bank_account_hint",
      "matched_account_id",
      "account_type_guess",
      "reference_code",
    ],
  }
}

export function buildExtractionPrompt(
  input: ExtractTransactionInput,
  categories?: readonly string[]
): { systemPrompt: string; userPrompt: string } {
  const validCategories = resolveCategories(categories ?? input.categories)
  const accounts = input.accounts ?? []
  const countryAndTimezoneRules = getCountryPromptRules({
    userCountry: input.userCountry,
    countryPromptRules: input.countryPromptRules,
    userTimezone: input.userTimezone,
    referenceUtc: input.receivedAtUtc,
  })

  const systemPrompt = [
    "You are a strict financial email extraction engine. Output ONLY a single JSON object matching the required schema.",
    "Classify email_kind as:",
    '- "card_charge": bank/card authorization or charge alert.',
    '- "receipt": merchant purchase receipt or order confirmation.',
    '- "non_transaction": credit card or bank billing statements, e-statements, monthly bill summaries (emails showing Total Billing, Minimum Payment, Statement Balance, or a future Payment Due Date), bill payments, shipping updates, OTPs, security alerts, promotional emails, and refunds. Never classify a billing statement or its due date as a card_charge.',
    "Rules:",
    '- Return amount_decimal as a plain unformatted number string without thousand separators (e.g. "12.50" for "$12.50", or "1250" for "1,250").',
    '- Return currency as a 3-letter ISO 4217 code (e.g. "USD", "EUR", "GBP").',
    ...countryAndTimezoneRules,
    `- Choose category strictly from: ${validCategories.join(", ")} (or null for non_transaction).`,
    "- Return card_last4 as 4 digits if present, otherwise null.",
    '- Return payment_method as the raw payment method or wallet name shown in the email (e.g. "Apple Pay", "PayPal") if present, otherwise null.',
    '- Return bank_account_hint as the bank account number or masked suffix (e.g. "****5678") if present, otherwise null.',
    "- If Known-Accounts are provided and the email clearly identifies one of them, return its id in matched_account_id; otherwise return null.",
    '- Return account_type_guess as "credit_card", "debit_card", or "digital_wallet" if the email indicates the payment account type, otherwise null.',
    "- Return reference_code if present, otherwise null.",
  ].join("\n")

  const knownAccountsLines =
    accounts.length > 0
      ? [
          "Known-Accounts:",
          ...accounts.map((a) => {
            const idList =
              a.identifiers.length > 0
                ? ` | identifiers: ${a.identifiers.join(", ")}`
                : ""
            return `- id=${a.id} | name=${a.name} | type=${a.type}${idList}`
          }),
        ]
      : []

  const userPrompt = [
    `From: ${input.fromAddress}`,
    `Subject: ${input.subject}`,
    `Received-At-UTC: ${input.receivedAtUtc}`,
    input.userCountry ? `User-Country: ${input.userCountry}` : null,
    `User-Timezone: ${input.userTimezone}`,
    input.userCurrency ? `Default-Currency: ${input.userCurrency}` : null,
    ...knownAccountsLines,
    "",
    "Email-Body:",
    input.cleanedText,
  ]
    .filter((line): line is string => line !== null)
    .join("\n")

  return { systemPrompt, userPrompt }
}

export function stripThinkBlocks(text: string): string {
  return text.replace(/<think>[\s\S]*?<\/think>/gi, "").trim()
}

function createExtractionValibotSchema(categories: readonly string[]) {
  const categorySet = new Set(categories)
  return v.strictObject({
    email_kind: v.picklist(["card_charge", "receipt", "non_transaction"]),
    amount_decimal: v.nullable(v.string()),
    currency: v.nullable(v.string()),
    transaction_date: v.nullable(v.string()),
    transaction_time: v.nullable(v.string()),
    timezone_offset: v.nullable(v.string()),
    merchant: v.nullable(v.string()),
    category: v.nullable(
      v.pipe(
        v.string(),
        v.check(
          (val) => categorySet.has(val),
          "Category must be in allowed category list"
        )
      )
    ),
    card_last4: v.nullable(v.string()),
    payment_method: v.optional(v.nullable(v.string()), null),
    bank_account_hint: v.optional(v.nullable(v.string()), null),
    matched_account_id: v.optional(v.nullable(v.string()), null),
    account_type_guess: v.optional(
      v.nullable(v.picklist(["credit_card", "debit_card", "digital_wallet"])),
      null
    ),
    reference_code: v.nullable(v.string()),
  })
}

export function parseAndValidateExtraction(
  rawText: string,
  categories?: readonly string[]
): RawLlmExtraction {
  const cleaned = stripThinkBlocks(rawText)
  let parsedJson: unknown
  try {
    parsedJson = JSON.parse(cleaned)
  } catch (err) {
    throw new LlmExtractionError("LLM output is not valid JSON", {
      cause: err,
    })
  }

  const validCategories = resolveCategories(categories)
  const schema = createExtractionValibotSchema(validCategories)
  const result = v.safeParse(schema, parsedJson)
  if (!result.success) {
    const issues = result.issues.map((i) => i.message).join("; ")
    throw new LlmExtractionError(
      `LLM output failed schema validation: ${issues}`
    )
  }

  return result.output
}

function normalizeCardLast4(raw: string | null): string | null {
  if (!raw) return null
  const digits = raw.replace(/\D/g, "")
  return digits.length === 4 ? digits : null
}

function recoverTruncatedZeroDecimalAmount(
  rawAmountDecimal: string,
  currency: string,
  cleanedText: string
): string {
  if (getCurrencyExponent(currency) !== 0) {
    return rawAmountDecimal
  }
  const trimmed = rawAmountDecimal.trim()
  const match = /^(\d{1,3}(?:[.,]\d{3})*)\.(\d{1,2})$/.exec(trimmed)
  if (!match) return rawAmountDecimal

  const intDigits = match[1].replace(/[.,]/g, "")
  const fracThreeDigits = match[2].padEnd(3, "0")
  const candidateInt = `${intDigits}${fracThreeDigits}`

  const dotFormatted = `${intDigits}.${fracThreeDigits}`
  const commaFormatted = `${intDigits},${fracThreeDigits}`
  const escapedCandidates = [dotFormatted, commaFormatted, candidateInt].map(
    (s) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")
  )
  const boundaryPattern = new RegExp(
    `(?<!\\d)(?:${escapedCandidates.join("|")})(?!\\d)`
  )
  if (boundaryPattern.test(cleanedText)) {
    return candidateInt
  }
  return rawAmountDecimal
}

export function normalizeExtractedResult(
  raw: RawLlmExtraction,
  rawJson: string,
  input: ExtractTransactionInput
): ExtractTransactionResult {
  if (raw.email_kind === "non_transaction") {
    return {
      emailKind: "non_transaction",
      raw,
      rawJson,
    }
  }

  const rawCurrency = raw.currency?.trim() || input.userCurrency?.trim() || ""
  if (!rawCurrency) {
    throw new LlmExtractionError("Missing currency for transaction extraction")
  }

  let currency: string
  try {
    currency = normalizeCurrencyCode(rawCurrency)
  } catch (err) {
    throw new LlmExtractionError(
      `Invalid currency in extraction: ${rawCurrency}`,
      {
        cause: err,
      }
    )
  }

  if (!raw.amount_decimal || !raw.amount_decimal.trim()) {
    throw new LlmExtractionError(
      "Missing amount_decimal for transaction extraction"
    )
  }

  const resolvedAmountDecimal = recoverTruncatedZeroDecimalAmount(
    raw.amount_decimal,
    currency,
    input.cleanedText
  )

  let amountMinor: number
  try {
    amountMinor = parseAmountToMinor(resolvedAmountDecimal, currency)
  } catch (err) {
    throw new LlmExtractionError(
      `Invalid amount_decimal "${raw.amount_decimal}" for ${currency}`,
      { cause: err }
    )
  }

  const occurredAtUtc = parseRawExtractedDateTimeToUtc({
    transactionDate: raw.transaction_date,
    transactionTime: raw.transaction_time,
    timezoneOffset: raw.timezone_offset,
    userTimezone: input.userTimezone,
    receivedAtUtc: input.receivedAtUtc,
  })

  const merchantRaw = raw.merchant?.trim() || "Unknown Merchant"
  const validCategories = resolveCategories(input.categories)
  const category =
    raw.category && validCategories.includes(raw.category)
      ? raw.category
      : validCategories.includes("Other")
        ? "Other"
        : validCategories[0]

  return {
    emailKind: raw.email_kind,
    amountMinor,
    currency,
    occurredAtUtc,
    merchantRaw,
    category,
    cardLast4: normalizeCardLast4(raw.card_last4),
    paymentMethodRaw: raw.payment_method?.trim() || null,
    bankAccountHint: raw.bank_account_hint?.trim() || null,
    matchedAccountId: raw.matched_account_id?.trim() || null,
    accountTypeGuess: raw.account_type_guess ?? null,
    referenceCode: raw.reference_code?.trim() || null,
    raw,
    rawJson,
  }
}
