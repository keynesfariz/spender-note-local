import { Suspense } from "react"

import { CategoriesTagsView } from "@/components/categories-tags-view"

export default function CategoriesPage() {
  return (
    <Suspense
      fallback={<div className="h-48 animate-pulse rounded-lg bg-muted/40" />}
    >
      <CategoriesTagsView />
    </Suspense>
  )
}
