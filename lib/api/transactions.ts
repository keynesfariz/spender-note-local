import { queryOptions, type QueryClient } from "@tanstack/react-query"
import { apiFetch } from "./client"
import type {
  MonthlySpendingSummary,
  PaginatedTransactionsResponse,
  TransactionEnrichmentPreview,
  TransactionListItem,
} from "./types.generated"

export const monthlySummaryQuery = (
  month?: string,
  { category, tag }: { category?: string; tag?: string } = {}
) =>
  queryOptions({
    queryKey: [
      "spending",
      "summary",
      month ?? "current",
      category,
      tag,
    ] as const,
    queryFn: ({ signal }) => {
      const params = new URLSearchParams()
      if (month) params.set("month", month)
      if (category) params.set("category", category)
      if (tag) params.set("tag", tag)
      const qs = params.toString()
      return apiFetch<MonthlySpendingSummary>(
        `/api/spending/summary${qs ? `?${qs}` : ""}`,
        { signal }
      )
    },
  })

export const TRANSACTIONS_PAGE_SIZE = 25

export const DEFAULT_LEDGER_PARAMS = {
  ledgerRange: "dates",
  page: 1,
  pageSize: TRANSACTIONS_PAGE_SIZE,
} as const

export const transactionsQuery = (params?: {
  month?: string
  ledgerRange?: "all" | "dates"
  startDate?: string
  endDate?: string
  category?: string
  tag?: string
  accountId?: string
  kind?: "expense" | "income" | "transfer"
  sourceType?: TransactionListItem["source_type"]
  search?: string
  unreviewedOnly?: boolean
  page?: number
  pageSize?: number
}) =>
  queryOptions({
    queryKey: ["transactions", "list", params ?? {}] as const,
    queryFn: ({ signal }) => {
      const sp = new URLSearchParams()
      if (params?.ledgerRange) sp.set("ledgerRange", params.ledgerRange)
      if (params?.month) sp.set("month", params.month)
      if (params?.startDate) sp.set("startDate", params.startDate)
      if (params?.endDate) sp.set("endDate", params.endDate)
      if (params?.category) sp.set("category", params.category)
      if (params?.tag) sp.set("tag", params.tag)
      if (params?.accountId) sp.set("accountId", params.accountId)
      if (params?.kind) sp.set("kind", params.kind)
      if (params?.sourceType) sp.set("sourceType", params.sourceType)
      if (params?.search) sp.set("search", params.search)
      if (params?.unreviewedOnly) sp.set("unreviewedOnly", "true")
      if (params?.page) sp.set("page", String(params.page))
      if (params?.pageSize) sp.set("pageSize", String(params.pageSize))
      const qs = sp.toString()
      return apiFetch<PaginatedTransactionsResponse>(
        `/api/transactions${qs ? `?${qs}` : ""}`,
        { signal }
      )
    },
  })

export const addManualTransactionApi = (input: {
  kind?: "expense" | "income"
  amount_minor: number
  occurred_at_utc: string
  merchant: string
  category?: string
  tag_ids?: string[]
  account_id?: string | null
  note?: string | null
}) =>
  apiFetch<TransactionListItem>("/api/transactions", {
    method: "POST",
    body: JSON.stringify(input),
  })

type TransferAction = "record" | "match" | "non_transaction"

export type TransferMutationItem = {
  action?: TransferAction
  id?: string
  request_id?: string
  account_id?: string
  destination_account_id?: string
  amount_minor?: number
  destination_amount_minor?: number
  occurred_at_utc?: string
  source_posted_at_utc?: string | null
  destination_posted_at_utc?: string | null
  note?: string | null
  email_ids?: string[]
  expected_transaction_id?: string
  match_transaction_id?: string
  card_payment?: boolean
}

export const saveTransfersApi = (items: TransferMutationItem[]) =>
  apiFetch<{ items: TransactionListItem[] }>("/api/transactions/transfers", {
    method: "POST",
    body: JSON.stringify({ action: "record", items }),
  })

export const updateTransactionApi = (input: {
  id: string
  amount_minor?: number
  occurred_at_utc?: string
  merchant?: string
  remark?: string
  remark_user_edited?: boolean
  category?: string
  tag_ids?: string[]
  tag_names?: string[]
  account_id?: string | null
  note?: string | null
  convertToLocalCurrency?: boolean
}) => {
  const { id, ...patch } = input
  return apiFetch<TransactionListItem>(
    `/api/transactions/${encodeURIComponent(id)}`,
    {
      method: "PATCH",
      body: JSON.stringify(patch),
    }
  )
}

export const deleteTransactionApi = (id: string) =>
  apiFetch(`/api/transactions/${encodeURIComponent(id)}`, { method: "DELETE" })

export const deleteTransactionsApi = (transactionIds: string[]) =>
  apiFetch<{ deletedCount: number }>("/api/transactions/delete", {
    method: "POST",
    body: JSON.stringify({ transactionIds }),
  })

export const splitTransactionSourceApi = (input: {
  transactionId: string
  emailId: string
}) =>
  apiFetch<{ transactionId: string }>(
    `/api/transactions/${encodeURIComponent(input.transactionId)}/split`,
    {
      method: "POST",
      body: JSON.stringify({ emailId: input.emailId }),
    }
  )

export const duplicateMergeTransactionsApi = (input: {
  keepTransactionId: string
  dropTransactionId: string
}) =>
  apiFetch<TransactionListItem>("/api/transactions/merge", {
    method: "POST",
    body: JSON.stringify({ mode: "duplicate", ...input }),
  })

export const combineTransactionsApi = (input: {
  transactionIds: string[]
  merchant?: string
  remark?: string
  remark_user_edited?: boolean
  category?: string
  tag_ids?: string[]
  tag_names?: string[]
  account_id?: string | null
  occurred_at_utc: string
  note?: string | null
}) =>
  apiFetch<TransactionListItem>("/api/transactions/merge", {
    method: "POST",
    body: JSON.stringify({ mode: "combine", ...input }),
  })

export const enrichTransactionsApi = (
  input: { transactionIds: string[] },
  options?: { signal?: AbortSignal }
) =>
  apiFetch<TransactionEnrichmentPreview>("/api/transactions/enrich", {
    method: "POST",
    body: JSON.stringify(input),
    signal: options?.signal,
  })

export const markTransactionsReviewedApi = (transactionIds: string[]) =>
  apiFetch<{ updatedCount: number }>("/api/transactions/review", {
    method: "POST",
    body: JSON.stringify({ transactionIds }),
  })

export function invalidateTransactionRelatedQueries(queryClient: QueryClient) {
  void queryClient.invalidateQueries({ queryKey: ["transactions"] })
  void queryClient.invalidateQueries({ queryKey: ["spending"] })
  void queryClient.invalidateQueries({ queryKey: ["accounts"] })
  void queryClient.invalidateQueries({ queryKey: ["categories"] })
  void queryClient.invalidateQueries({ queryKey: ["tags"] })
}

export type ImportManualTransactionRowInput = {
  kind: "expense" | "income"
  amount_minor: number
  currency?: string
  occurred_at_utc: string
  merchant: string
  category?: string
  tag_names?: string[]
  note?: string
}

export type ImportManualTransactionsResponse = {
  importedCount: number
  ids: string[]
}

export const importManualTransactionsApi = (input: {
  account_id: string
  transactions: ImportManualTransactionRowInput[]
}) =>
  apiFetch<ImportManualTransactionsResponse>("/api/transactions/import", {
    method: "POST",
    body: JSON.stringify(input),
  })
