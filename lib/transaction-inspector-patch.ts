import type { updateTransactionApi } from "./api/transactions"
import { dateTimeInputToUtcIso, parseDecimalToMinorClient } from "./format"

export type InspectorDraftField =
  | "remark"
  | "merchant"
  | "amount"
  | "category"
  | "tags"
  | "account"
  | "date"
  | "note"
  | "conversion"

export function buildInspectorPatch(
  {
    baseline,
    edits,
    remark,
    merchant,
    remarkUserEdited,
    amount,
    category,
    selectedTagIds,
    draftTagNames,
    accountId,
    date,
    time,
    note,
    convertToLocalCurrency,
  }: {
    baseline: {
      currency: string
      date: string
      time: string
      timezone?: string | null
    }
    edits: ReadonlySet<InspectorDraftField>
    remark: string
    merchant: string
    remarkUserEdited: boolean
    amount: string
    category: string
    selectedTagIds: string[]
    draftTagNames: string[]
    accountId: string | null
    date: string
    time: string
    note: string
    convertToLocalCurrency: boolean
  },
  tx: {
    id: string
    currency: string
    isIncome: boolean
    receivingCurrency: string
    localCurrency: string
  }
): Parameters<typeof updateTransactionApi>[0] {
  const { isIncome, receivingCurrency, localCurrency } = tx
  const saveAmount = edits.has("amount") || convertToLocalCurrency
  if (saveAmount && baseline.currency !== tx.currency) {
    throw new Error(
      "The transaction currency changed. Reopen it before editing the amount."
    )
  }
  const targetCurrency = isIncome
    ? receivingCurrency
    : convertToLocalCurrency
      ? localCurrency
      : tx.currency
  const saveDate =
    edits.has("date") && (date !== baseline.date || time !== baseline.time)

  if (saveDate && (!date.trim() || !time.trim())) {
    throw new Error("Enter the occurred date and time before saving.")
  }

  const saveMerchant = edits.has("merchant")
  const saveRemark = edits.has("remark") || saveMerchant
  return {
    id: tx.id,
    remark: saveRemark ? remark.trim() : undefined,
    merchant: saveMerchant ? merchant : undefined,
    remark_user_edited: saveRemark ? remarkUserEdited : undefined,
    amount_minor: saveAmount
      ? parseDecimalToMinorClient(amount, targetCurrency)
      : undefined,
    category: edits.has("category") ? category : undefined,
    tag_ids: edits.has("tags") ? selectedTagIds : undefined,
    tag_names: edits.has("tags") ? draftTagNames : undefined,
    account_id: edits.has("account") ? accountId : undefined,
    occurred_at_utc: saveDate
      ? dateTimeInputToUtcIso(date, time, baseline.timezone)
      : undefined,
    note: edits.has("note") ? note.trim() || null : undefined,
    convertToLocalCurrency: saveAmount ? convertToLocalCurrency : undefined,
  }
}
