import { queryOptions } from "@tanstack/react-query"
import { apiFetch } from "./client"
import type {
  AppSettingsView,
  GmailStatusView,
  SenderRecord,
} from "./types.generated"

export const settingsQuery = () =>
  queryOptions({
    queryKey: ["settings"] as const,
    queryFn: () => apiFetch<AppSettingsView>("/api/settings"),
  })

export const sendersQuery = () =>
  queryOptions({
    queryKey: ["senders"] as const,
    queryFn: () => apiFetch<SenderRecord[]>("/api/senders"),
  })

export const gmailStatusQuery = () =>
  queryOptions({
    queryKey: ["gmail-status"] as const,
    queryFn: () => apiFetch<GmailStatusView>("/api/gmail/status"),
  })

export const updateLocaleSettingsApi = (input: {
  country_code: string
  timezone?: string
  currency?: string
}) =>
  apiFetch("/api/settings", {
    method: "PATCH",
    body: JSON.stringify({ section: "locale", ...input }),
  })

export const probeLlmModelsApi = (baseUrl: string) => {
  const qs = baseUrl.trim()
    ? `?llm_base_url=${encodeURIComponent(baseUrl.trim())}`
    : ""
  return apiFetch<AppSettingsView>(`/api/settings${qs}`)
}

export const updateLlmSettingsApi = (input: {
  llm_base_url: string
  llm_model: string | null
  llm_timeout_ms?: number
}) =>
  apiFetch("/api/settings", {
    method: "PATCH",
    body: JSON.stringify({ section: "llm", ...input }),
  })

export const updateSyncSettingsApi = (input: { sync_batch_limit: number }) =>
  apiFetch("/api/settings", {
    method: "PATCH",
    body: JSON.stringify({ section: "sync", ...input }),
  })

export const testLlmModelApi = (input?: {
  llm_base_url?: string
  llm_model?: string
  llm_timeout_ms?: number
}) =>
  apiFetch<{ ok: true; baseUrl: string; model: string }>("/api/settings", {
    method: "POST",
    body: input ? JSON.stringify(input) : undefined,
  })

export const createSenderApi = (input: {
  email: string
  role_hint?: "auto" | "card_charge" | "receipt"
  default_account_id?: string | null
  enabled?: boolean
  backfill_from_utc?: string
}) =>
  apiFetch<SenderRecord>("/api/senders", {
    method: "POST",
    body: JSON.stringify(input),
  })

export const updateSenderApi = (input: {
  id: string
  email?: string
  role_hint?: "auto" | "card_charge" | "receipt"
  default_account_id?: string | null
  enabled?: boolean
  backfill_from_utc?: string
}) => {
  const { id, ...patch } = input
  return apiFetch<SenderRecord>(`/api/senders/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: JSON.stringify(patch),
  })
}

export const deleteSenderApi = (id: string) =>
  apiFetch(`/api/senders/${encodeURIComponent(id)}`, { method: "DELETE" })

export const disconnectGmailApi = () =>
  apiFetch("/api/gmail/status", { method: "DELETE" })
