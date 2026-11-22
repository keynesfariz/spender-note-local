import { queryOptions } from "@tanstack/react-query"
import { apiFetch } from "./client"
import type {
  AccountIdentifierInput,
  AccountSummary,
  AccountType,
  ReconciliationRow,
} from "./types.generated"

export const accountsQuery = () =>
  queryOptions({
    queryKey: ["accounts"] as const,
    queryFn: () => apiFetch<AccountSummary[]>("/api/accounts"),
  })

export const accountReconciliationsQuery = (accountId: string) =>
  queryOptions({
    queryKey: ["accounts", "reconciliations", accountId] as const,
    queryFn: () =>
      apiFetch<ReconciliationRow[]>(
        `/api/accounts/${encodeURIComponent(accountId)}/reconciliations`
      ),
  })

export const createAccountApi = (input: {
  name: string
  type: AccountType
  initial_balance_minor: number
  initial_balance_as_of_utc?: string
  last4?: string | null
  statement_day?: number | null
  identifiers?: AccountIdentifierInput[]
}) =>
  apiFetch<AccountSummary>("/api/accounts", {
    method: "POST",
    body: JSON.stringify(input),
  })

export const updateAccountApi = (input: {
  id: string
  name?: string
  type?: AccountType
  initial_balance_minor?: number | null
  initial_balance_as_of_utc?: string
  last4?: string | null
  statement_day?: number | null
  identifiers?: AccountIdentifierInput[]
}) => {
  const { id, ...patch } = input
  return apiFetch<AccountSummary>(`/api/accounts/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: JSON.stringify(patch),
  })
}

export const confirmAccountApi = (id: string) =>
  apiFetch<AccountSummary>(`/api/accounts/${encodeURIComponent(id)}/confirm`, {
    method: "POST",
  })

export const deleteAccountApi = (id: string) =>
  apiFetch(`/api/accounts/${encodeURIComponent(id)}`, { method: "DELETE" })

export const reconcileAccountApi = (input: {
  account_id: string
  actual_balance_minor: number
  note?: string | null
  recorded_at_utc?: string
}) =>
  apiFetch(
    `/api/accounts/${encodeURIComponent(input.account_id)}/reconciliations`,
    {
      method: "POST",
      body: JSON.stringify(input),
    }
  )

export const mergeAccountsApi = (input: {
  sourceAccountId: string
  targetAccountId: string
}) =>
  apiFetch("/api/accounts/merge", {
    method: "POST",
    body: JSON.stringify(input),
  })
