import { Suspense } from "react"

import { SettingsView } from "@/components/settings-view"
import { Skeleton } from "@/components/ui/skeleton"

export default function SettingsPage() {
  return (
    <Suspense fallback={<Skeleton className="h-48 w-full" />}>
      <SettingsView />
    </Suspense>
  )
}
