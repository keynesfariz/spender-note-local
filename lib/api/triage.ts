import { queryOptions, type QueryClient } from "@tanstack/react-query"
import { apiFetch } from "./client"
import { invalidateTransactionRelatedQueries } from "./transactions"
import type {
  ApplyTriageVerdictInput,
  DeleteTriagePolicyResult,
  PaginatedIgnoredEmailsResponse,
  PaginatedSkippedTransactionsResponse,
  PromoteIgnoredEmailResult,
  SenderTriagePolicyView,
  TriageUndoInput,
  TriageUndoResult,
  TriageVerdictResult,
  UpdateTriagePolicyInput,
} from "./types.generated"

export const triagePoliciesQuery = (params?: {
  senderEmail?: string
  search?: string
}) =>
  queryOptions({
    queryKey: ["triage", "policies", params ?? {}] as const,
    queryFn: () => {
      const sp = new URLSearchParams()
      if (params?.senderEmail) sp.set("senderEmail", params.senderEmail)
      if (params?.search) sp.set("search", params.search)
      const qs = sp.toString()
      return apiFetch<SenderTriagePolicyView[]>(
        `/api/triage/policies${qs ? `?${qs}` : ""}`
      )
    },
  })

export const triageIgnoredEmailsQuery = (params?: {
  senderEmail?: string
  search?: string
  page?: number
  pageSize?: number
}) =>
  queryOptions({
    queryKey: ["triage", "ignored", params ?? {}] as const,
    queryFn: () => {
      const sp = new URLSearchParams()
      if (params?.senderEmail) sp.set("senderEmail", params.senderEmail)
      if (params?.search) sp.set("search", params.search)
      if (params?.page) sp.set("page", String(params.page))
      if (params?.pageSize) sp.set("pageSize", String(params.pageSize))
      const qs = sp.toString()
      return apiFetch<PaginatedIgnoredEmailsResponse>(
        `/api/triage/ignored${qs ? `?${qs}` : ""}`
      )
    },
  })

export const triageSkippedTransactionsQuery = (
  page: number,
  pageSize: number
) =>
  queryOptions({
    queryKey: ["triage", "skipped", page, pageSize] as const,
    queryFn: () => {
      const sp = new URLSearchParams({
        page: String(page),
        pageSize: String(pageSize),
      })
      return apiFetch<PaginatedSkippedTransactionsResponse>(
        `/api/triage/skipped?${sp}`
      )
    },
  })

export const applyTriageVerdictApi = (input: ApplyTriageVerdictInput) =>
  apiFetch<TriageVerdictResult>("/api/triage/verdict", {
    method: "POST",
    body: JSON.stringify(input),
  })

export const undoTriageVerdictApi = (input: TriageUndoInput) =>
  apiFetch<TriageUndoResult>("/api/triage/undo", {
    method: "POST",
    body: JSON.stringify(input),
  })

export const updateTriagePolicyApi = (
  id: string,
  input: UpdateTriagePolicyInput
) =>
  apiFetch<SenderTriagePolicyView>(
    `/api/triage/policies/${encodeURIComponent(id)}`,
    {
      method: "PATCH",
      body: JSON.stringify(input),
    }
  )

export const deleteTriagePolicyApi = (id: string) =>
  apiFetch<DeleteTriagePolicyResult>(
    `/api/triage/policies/${encodeURIComponent(id)}`,
    {
      method: "DELETE",
    }
  )

export const promoteIgnoredEmailApi = (emailId: string) =>
  apiFetch<PromoteIgnoredEmailResult>(
    `/api/triage/ignored/${encodeURIComponent(emailId)}/promote`,
    {
      method: "POST",
    }
  )

export function invalidateTriageRelatedQueries(queryClient: QueryClient) {
  void queryClient.invalidateQueries({ queryKey: ["triage"] })
  invalidateTransactionRelatedQueries(queryClient)
}
