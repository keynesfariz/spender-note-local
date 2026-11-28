import { useQueryClient } from "@tanstack/react-query"
import { type RefObject } from "react"

export function formatTxCount(count: number): string {
  return `${count} transaction${count === 1 ? "" : "s"}`
}

export function getErrorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback
}

export type EditorFocus = {
  trigger: HTMLButtonElement
  fallback: RefObject<HTMLButtonElement | null>
}

export function useTaxonomyInvalidation(key: "categories" | "tags") {
  const queryClient = useQueryClient()
  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: [key] }),
      queryClient.invalidateQueries({ queryKey: ["transactions"] }),
      queryClient.invalidateQueries({ queryKey: ["spending"] }),
    ])
}
