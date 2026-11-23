import { queryOptions } from "@tanstack/react-query"
import { apiFetch } from "./client"
import type { TagSummary } from "./types.generated"

export const tagsQuery = () =>
  queryOptions({
    queryKey: ["tags"] as const,
    queryFn: () => apiFetch<TagSummary[]>("/api/tags"),
  })

export const createTagApi = (input: { name: string }) =>
  apiFetch<TagSummary>("/api/tags", {
    method: "POST",
    body: JSON.stringify(input),
  })

export const updateTagApi = (input: { id: string; name: string }) =>
  apiFetch<TagSummary>(`/api/tags/${encodeURIComponent(input.id)}`, {
    method: "PATCH",
    body: JSON.stringify({ name: input.name }),
  })

export const deleteTagApi = (id: string) =>
  apiFetch<{ ok: boolean }>(`/api/tags/${encodeURIComponent(id)}`, {
    method: "DELETE",
  })
