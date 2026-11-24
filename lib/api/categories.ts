import { queryOptions } from "@tanstack/react-query"
import { DEFAULT_CATEGORY } from "@/lib/categories"
import { apiFetch } from "./client"
import type { CategorySummary } from "./types.generated"

export const categoriesQuery = () =>
  queryOptions({
    queryKey: ["categories"] as const,
    queryFn: () => apiFetch<CategorySummary[]>("/api/categories"),
  })

export const createCategoryApi = (input: { name: string; icon: string }) =>
  apiFetch<CategorySummary>("/api/categories", {
    method: "POST",
    body: JSON.stringify(input),
  })

export const updateCategoryApi = (input: {
  id: string
  name?: string
  icon?: string
}) => {
  const { id, ...patch } = input
  return apiFetch<CategorySummary>(
    `/api/categories/${encodeURIComponent(id)}`,
    {
      method: "PATCH",
      body: JSON.stringify(patch),
    }
  )
}

export const deleteCategoryApi = (input: {
  id: string
  replaceWithCategory?: string
}) =>
  apiFetch<{ ok: boolean }>(`/api/categories/${encodeURIComponent(input.id)}`, {
    method: "DELETE",
    body: JSON.stringify({
      replaceWithCategory: input.replaceWithCategory ?? DEFAULT_CATEGORY,
    }),
  })
