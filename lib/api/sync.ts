import { queryOptions } from "@tanstack/react-query"
import { apiFetch } from "./client"
import type { SyncStatusView } from "./types.generated"

export const syncStatusQuery = () =>
  queryOptions({
    queryKey: ["sync-status"] as const,
    queryFn: () => apiFetch<SyncStatusView>("/api/sync"),
  })

export const triggerSyncApi = () =>
  apiFetch<{ runId: string }>("/api/sync", {
    method: "POST",
    body: JSON.stringify({ action: "trigger" }),
  })

export const retryFailedExtractionsApi = () =>
  apiFetch<{ runId: string }>("/api/sync", {
    method: "POST",
    body: JSON.stringify({ action: "retry" }),
  })

export const cancelSyncApi = () =>
  apiFetch<{ cancelled: boolean }>("/api/sync", {
    method: "POST",
    body: JSON.stringify({ action: "cancel" }),
  })

export const dismissPendingQueueApi = () =>
  apiFetch<{ dismissedCount: number }>("/api/sync", {
    method: "POST",
    body: JSON.stringify({ action: "dismiss" }),
  })
