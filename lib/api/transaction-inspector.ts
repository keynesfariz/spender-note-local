import { queryOptions } from "@tanstack/react-query"
import { apiFetch } from "./client"
import type {
  TransactionInspectorView,
  TransactionSourceBody,
} from "./types.generated"

export const transactionInspectorQuery = (id: string) =>
  queryOptions({
    queryKey: ["transactions", "inspector", id] as const,
    queryFn: ({ signal }) =>
      apiFetch<TransactionInspectorView>(
        `/api/transactions/${encodeURIComponent(id)}?view=inspector`,
        { signal }
      ),
  })

export const transactionSourceBodyQuery = (
  transactionId: string,
  emailId: string
) =>
  queryOptions({
    queryKey: ["transactions", "source-body", transactionId, emailId] as const,
    queryFn: ({ signal }) =>
      apiFetch<TransactionSourceBody>(
        `/api/transactions/${encodeURIComponent(transactionId)}?sourceEmailId=${encodeURIComponent(emailId)}`,
        { signal }
      ),
  })
