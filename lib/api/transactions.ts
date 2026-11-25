import { queryOptions, type QueryClient } from "@tanstack/react-query"
import { apiFetch } from "./client"
import type {
  MonthlySpendingSummary,
  PaginatedTransactionsResponse,
  TransactionDetailView,
  TransactionEnrichmentPreview,
  TransactionListItem,
} from "./types.generated"

export const monthlySummaryQuery = (month?: string) =>
  queryOptions({
    queryKey: ["spending", "summary", month ?? "current"] as const,
    queryFn: () => {
      const qs = month ? `?month=${encodeURIComponent(month)}` : ""
      return apiFetch<MonthlySpendingSummary>(`/api/spending/summary${qs}`)
    },
  })

export const transactionsQuery = (params?: {
  month?: string
  startDate?: string
  endDate?: string
  category?: string
  tag?: string
  accountId?: string
  search?: string
  unreviewedOnly?: boolean
  showSkipped?: boolean
  page?: number
  pageSize?: number
}) =>
  queryOptions({
    queryKey: ["transactions", "list", params ?? {}] as const,
    queryFn: () => {
      const sp = new URLSearchParams()
      if (params?.month) sp.set("month", params.month)
      if (params?.startDate) sp.set("startDate", params.startDate)
      if (params?.endDate) sp.set("endDate", params.endDate)
      if (params?.category) sp.set("category", params.category)
      if (params?.tag) sp.set("tag", params.tag)
      if (params?.accountId) sp.set("accountId", params.accountId)
      if (params?.search) sp.set("search", params.search)
      if (params?.unreviewedOnly) sp.set("unreviewedOnly", "true")
      if (params?.showSkipped === false) sp.set("showSkipped", "false")
      if (params?.page) sp.set("page", String(params.page))
      if (params?.pageSize) sp.set("pageSize", String(params.pageSize))
      const qs = sp.toString()
      return apiFetch<PaginatedTransactionsResponse>(
        `/api/transactions${qs ? `?${qs}` : ""}`
      )
    },
  })

export const transactionDetailQuery = (id: string) =>
  queryOptions({
    queryKey: ["transactions", "detail", id] as const,
    queryFn: () =>
      apiFetch<TransactionDetailView>(
        `/api/transactions/${encodeURIComponent(id)}`
      ),
  })

export const addManualTransactionApi = (input: {
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
