import type { ImportManualTransactionRowInput } from "@/lib/api/transactions"
import { DEFAULT_CATEGORY } from "@/lib/categories"
import { dateTimeInputToUtcIso, parseDecimalToMinorClient } from "@/lib/format"

export type ManualImportPromptInput = {
  accountName: string
  accountType: string
  currency: string
  timezone: string
  categories: string[]
  tags: string[]
  customInstructions?: string
}

export type ParsedManualImportRow = {
  index: number
  valid: boolean
  error?: string
  kind: "expense" | "income"
  amountDisplay: string
  amountMinor: number
  currency: string
  dateDisplay: string
  occurredAtUtc: string
  merchant: string
  rawCategory: string
  resolvedCategory: string
  categoryMatched: boolean
  matchedTags: string[]
  droppedTags: string[]
  note: string
  payload?: ImportManualTransactionRowInput
}

export type ParseManualImportResult = {
  rows: ParsedManualImportRow[]
  error?: string
}

function buildManualImportExampleJson(currency: string): string {
  const sampleAmount = currency === "IDR" || currency === "JPY" ? 45000 : 12.5
  return JSON.stringify(
    {
      transactions: [
        {
          date: "2026-11-15",
          time: "14:30:00",
          amount: sampleAmount,
          currency,
          kind: "expense",
          merchant: "Coffee Shop",
          category: "Food & Drink",
          tags: ["coffee"],
          note: "Iced latte",
        },
        {
          date: "2026-11-16",
          amount: sampleAmount * 10,
          currency,
          kind: "income",
          merchant: "Client Refund",
          category: "Other",
          tags: [],
          note: "",
        },
      ],
    },
    null,
    2
  )
}

export function buildManualImportPrompt(input: ManualImportPromptInput): string {
  const categoriesList =
    input.categories.length > 0 ? input.categories.join(", ") : DEFAULT_CATEGORY
  const tagsList = input.tags.length > 0 ? input.tags.join(", ") : "(none)"
  const customBlock = input.customInstructions?.trim()
    ? `\nAdditional user instructions:\n${input.customInstructions.trim()}\n`
    : ""

  return `You are extracting bank/card statement transactions from the attached PDF into a strict JSON file for my personal finance app.

Account context:
- Account: ${input.accountName} (${input.accountType})
- Default currency: ${input.currency}
- Timezone: ${input.timezone}

Available categories (use ONLY exact names from this list, or "${DEFAULT_CATEGORY}" if unsure):
${categoriesList}

Available tags (use ONLY exact names from this list; omit if none apply):
${tagsList}

Extraction rules:
1. Extract every transaction from the statement as an object inside the top-level "transactions" array.
2. "amount": positive number in major currency units (no currency symbols, no negative sign).
3. "currency": 3-letter ISO currency code. Default to "${input.currency}" if not explicitly shown.
4. "date": transaction date in "YYYY-MM-DD" format.
5. "time": optional transaction time in "HH:MM:SS" (24-hour, in ${input.timezone}). Omit or leave empty if the PDF only shows a date.
6. "merchant": concise merchant name or cleaned remark.
7. "note": optional extra context from the statement description (empty string if none).
8. "kind": MUST be either "expense" or "income".
9. "category": best matching category from the available categories list above.
10. "tags": array of matching tag names from the available tags list above (empty array if none).
${customBlock}
Return ONLY valid JSON matching this exact structure (no extra commentary):

\`\`\`json
${buildManualImportExampleJson(input.currency)}
\`\`\``
}

function stripJsonCodeFences(raw: string): string {
  const trimmed = raw.trim()
  const fenceMatch = trimmed.match(/^```(?:json)?\s*\n?([\s\S]*?)\n?```$/i)
  if (fenceMatch) {
    return fenceMatch[1].trim()
  }
  return trimmed
}

function normalizeTimestampToUtc(
  rawDate: unknown,
  rawTime: unknown,
  timezone: string
): { dateDisplay: string; occurredAtUtc?: string; error?: string } {
  const dateStr = typeof rawDate === "string" ? rawDate.trim() : ""
  if (!dateStr) {
    return { dateDisplay: "", error: "Missing transaction date." }
  }

  if (/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$/.test(dateStr)) {
    return { dateDisplay: dateStr, occurredAtUtc: dateStr }
  }

  if (!/^\d{4}-\d{2}-\d{2}$/.test(dateStr)) {
    return {
      dateDisplay: dateStr,
      error: "Date must be YYYY-MM-DD or UTC ISO-8601.",
    }
  }

  const timeStr = typeof rawTime === "string" ? rawTime.trim() : ""
  let normalizedTime = "12:00"
  let display = dateStr
  if (timeStr) {
    const match = timeStr.match(/^(\d{2}):(\d{2})(?::(\d{2}))?$/)
    if (!match) {
      return {
        dateDisplay: `${dateStr} ${timeStr}`,
        error: "Time must be HH:MM or HH:MM:SS.",
      }
    }
    normalizedTime = `${match[1]}:${match[2]}`
    display = `${dateStr} ${timeStr}`
  }

  let utcIso: string
  try {
    utcIso = dateTimeInputToUtcIso(dateStr, normalizedTime, timezone)
  } catch {
    return {
      dateDisplay: display,
      error: "Could not convert date/time using configured timezone.",
    }
  }
  return { dateDisplay: display, occurredAtUtc: utcIso }
}

export function parseManualImportJson(
  rawInput: string,
  options: {
    defaultCurrency: string
    timezone: string
    categories: string[]
    tags: string[]
  }
): ParseManualImportResult {
  const cleaned = stripJsonCodeFences(rawInput)
  if (!cleaned) {
    return { rows: [], error: "Paste JSON or upload a .json file to preview." }
  }

  let parsed: unknown
  try {
    parsed = JSON.parse(cleaned)
  } catch {
    return {
      rows: [],
      error: "Invalid JSON syntax. Check that the LLM returned complete JSON.",
    }
  }

  let items: unknown[]
  if (Array.isArray(parsed)) {
    items = parsed
  } else if (
    parsed &&
    typeof parsed === "object" &&
    Array.isArray((parsed as { transactions?: unknown }).transactions)
  ) {
    items = (parsed as { transactions: unknown[] }).transactions
  } else {
    return {
      rows: [],
      error:
        'Expected a JSON object with a "transactions" array (or a top-level array).',
    }
  }

  if (items.length === 0) {
    return { rows: [], error: "No transactions found in the JSON payload." }
  }

  const categoryByNorm = new Map<string, string>()
  for (const c of options.categories) {
    categoryByNorm.set(c.trim().toLowerCase(), c)
  }
  const tagByNorm = new Map<string, string>()
  for (const t of options.tags) {
    tagByNorm.set(t.trim().toLowerCase(), t)
  }

  const rows: ParsedManualImportRow[] = items.map((item, idx) => {
    const obj =
      item && typeof item === "object" ? (item as Record<string, unknown>) : {}

    const rawKind =
      typeof obj.kind === "string" && obj.kind.trim()
        ? obj.kind.trim().toLowerCase()
        : "expense"
    const kindValid = rawKind === "expense" || rawKind === "income"
    const kind: "expense" | "income" = kindValid ? rawKind : "expense"

    const rawCurrency =
      typeof obj.currency === "string" && obj.currency.trim()
        ? obj.currency.trim().toUpperCase()
        : options.defaultCurrency
    const currencyValid = /^[A-Z]{3}$/.test(rawCurrency)

    const rawAmountStr =
      typeof obj.amount === "number"
        ? String(obj.amount)
        : typeof obj.amount === "string"
          ? obj.amount.trim()
          : ""
    const amountMinor = currencyValid
      ? parseDecimalToMinorClient(rawAmountStr, rawCurrency)
      : null

    const rawDate = obj.date ?? obj.occurred_at_utc ?? obj.timestamp
    const rawTime = obj.time
    const ts = normalizeTimestampToUtc(rawDate, rawTime, options.timezone)

    const rawMerchant =
      typeof obj.merchant === "string"
        ? obj.merchant.trim()
        : typeof obj.remark === "string"
          ? obj.remark.trim()
          : ""
    const merchant =
      !rawMerchant && kind === "income" ? "Income" : rawMerchant

    const rawCategory =
      typeof obj.category === "string" ? obj.category.trim() : ""
    const matchedCategory = rawCategory
      ? categoryByNorm.get(rawCategory.toLowerCase())
      : undefined
    const resolvedCategory = matchedCategory ?? DEFAULT_CATEGORY

    const rawTags = Array.isArray(obj.tags) ? obj.tags : []
    const matchedTags: string[] = []
    const droppedTags: string[] = []
    const seenMatched = new Set<string>()
    for (const rawTag of rawTags) {
      if (typeof rawTag !== "string" || !rawTag.trim()) continue
      const trimmed = rawTag.trim()
      const canonical = tagByNorm.get(trimmed.toLowerCase())
      if (canonical) {
        if (!seenMatched.has(canonical)) {
          seenMatched.add(canonical)
          matchedTags.push(canonical)
        }
      } else {
        droppedTags.push(trimmed)
      }
    }

    const note = typeof obj.note === "string" ? obj.note.trim() : ""

    const errors: string[] = []
    if (!kindValid) {
      errors.push(`Unsupported kind "${String(obj.kind)}" (use expense or income).`)
    }
    if (!currencyValid) {
      errors.push(`Invalid currency "${rawCurrency}".`)
    }
    if (amountMinor === null || amountMinor <= 0) {
      errors.push("Amount must be greater than zero.")
    }
    if (ts.error || !ts.occurredAtUtc) {
      errors.push(ts.error ?? "Invalid date.")
    }
    if (!merchant) {
      errors.push("Merchant is required.")
    }

    const valid = errors.length === 0
    return {
      index: idx,
      valid,
      error: valid ? undefined : errors.join(" "),
      kind,
      amountDisplay: rawAmountStr || "—",
      amountMinor: amountMinor ?? 0,
      currency: rawCurrency,
      dateDisplay: ts.dateDisplay || "—",
      occurredAtUtc: ts.occurredAtUtc ?? "",
      merchant: merchant || "—",
      rawCategory,
      resolvedCategory,
      categoryMatched: Boolean(matchedCategory),
      matchedTags,
      droppedTags,
      note,
      payload: valid
        ? {
            kind,
            amount_minor: amountMinor!,
            currency: rawCurrency,
            occurred_at_utc: ts.occurredAtUtc!,
            merchant,
            category: resolvedCategory,
            tag_names: matchedTags,
            note: note || undefined,
          }
        : undefined,
    }
  })

  return { rows }
}
