import { type TransferMutationItem } from "@/lib/api/transactions"
import type {
  AccountSummary,
  TransactionListItem,
} from "@/lib/api/types.generated"
import {
  dateTimeInputToUtcIso,
  getTodayDateString,
  minorToDecimalInput,
  parseDecimalToMinorClient,
  utcToDateInputValue,
  utcToTimeInputValue,
} from "@/lib/format"

export type TransferTarget = {
  title?: string
  transaction?: TransactionListItem
  emailIds?: string[]
  skippedEmailId?: string
  amountMinor?: number
  currency?: string
  occurredAtUtc?: string
  cardPayment?: boolean
}

type LocalDateTime = {
  date: string
  time: string
}

export type Draft = {
  requestId: string
  mode: "record" | "match"
  from: string
  to: string
  amount: string
  received: string
  dateTime: LocalDateTime
  sourcePosted: LocalDateTime
  destinationPosted: LocalDateTime
  note: string
  matchId: string
}

function localInput(
  instant: string | null | undefined,
  timezone?: string | null
): LocalDateTime {
  if (!instant) return { date: "", time: "" }
  const date = utcToDateInputValue(instant, timezone)
  const time = utcToTimeInputValue(instant, timezone, true)
  return date && time ? { date, time } : { date: "", time: "" }
}

function toUtc(
  value: LocalDateTime,
  original: string | null | undefined,
  timezone?: string | null
) {
  if (original) {
    const loaded = localInput(original, timezone)
    if (loaded.date === value.date && loaded.time === value.time)
      return original
  }
  return dateTimeInputToUtcIso(value.date, value.time, timezone)
}

export function initialDraft(
  target: TransferTarget,
  timezone?: string | null
): Draft {
  const tx = target.transaction
  const original = localInput(
    tx?.occurred_at_utc ?? target.occurredAtUtc,
    timezone
  )
  const amount = tx?.amount_minor ?? target.amountMinor
  const currency = tx?.currency ?? target.currency
  return {
    requestId: crypto.randomUUID(),
    mode: "record",
    from: tx?.account_id ?? "",
    to: tx?.destination_account_id ?? "",
    amount: amount && currency ? minorToDecimalInput(amount, currency) : "",
    received:
      tx?.destination_amount_minor != null && tx.destination_currency
        ? minorToDecimalInput(
            tx.destination_amount_minor,
            tx.destination_currency
          )
        : "",
    dateTime: original.date
      ? original
      : { date: getTodayDateString(timezone), time: "00:00" },
    sourcePosted: localInput(tx?.source_posted_at_utc, timezone),
    destinationPosted: localInput(tx?.destination_posted_at_utc, timezone),
    note: tx?.note ?? "",
    matchId: "",
  }
}

function optionalPostingUtc(
  value: LocalDateTime,
  original: string | null | undefined,
  timezone?: string | null
) {
  return value.date || value.time ? toUtc(value, original, timezone) : undefined
}

export function buildTransferItem(
  target: TransferTarget,
  draft: Draft,
  accounts: AccountSummary[] | undefined,
  emailIds: string[] | undefined,
  timezone?: string | null
): TransferMutationItem {
  const tx = target.transaction
  const editing = tx?.kind === "transfer"
  if (tx && !editing && !emailIds?.length) {
    throw new Error("This transaction has no attached email.")
  }
  const common = {
    email_ids: emailIds,
    expected_transaction_id: emailIds?.length ? tx?.id : undefined,
    card_payment: target.cardPayment || undefined,
  }
  if (draft.mode === "match") {
    if (!draft.matchId) throw new Error("Select the transfer to match.")
    return {
      ...common,
      action: "match",
      match_transaction_id: draft.matchId,
    }
  }
  const from = accounts?.find((account) => account.id === draft.from)
  const to = accounts?.find((account) => account.id === draft.to)
  if (!from || !to || from.id === to.id)
    throw new Error("Choose two different accounts.")
  const amount = parseDecimalToMinorClient(draft.amount, from.currency)
  return {
    ...common,
    action: "record",
    id: editing ? tx.id : undefined,
    request_id: !editing && !emailIds?.length ? draft.requestId : undefined,
    account_id: from.id,
    destination_account_id: to.id,
    amount_minor: amount,
    destination_amount_minor:
      from.currency === to.currency
        ? amount
        : parseDecimalToMinorClient(draft.received, to.currency),
    occurred_at_utc: toUtc(
      draft.dateTime,
      tx?.occurred_at_utc ?? target.occurredAtUtc,
      timezone
    ),
    source_posted_at_utc: optionalPostingUtc(
      draft.sourcePosted,
      tx?.source_posted_at_utc,
      timezone
    ),
    destination_posted_at_utc: optionalPostingUtc(
      draft.destinationPosted,
      tx?.destination_posted_at_utc,
      timezone
    ),
    note: draft.note.trim() || null,
  }
}
