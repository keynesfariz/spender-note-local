import Link from "next/link"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { buttonVariants } from "@/components/ui/button"
import { cn } from "@/lib/utils"

export function LocaleSetupAlert({
  children,
  showTitle = true,
}: {
  children: string
  showTitle?: boolean
}) {
  return (
    <Alert>
      {showTitle && <AlertTitle>Finish initial setup</AlertTitle>}
      <AlertDescription
        className={cn(
          "flex flex-wrap items-center justify-between gap-3",
          showTitle && "mt-2"
        )}
      >
        <span>{children}</span>
        <Link href="/settings" className={buttonVariants({ size: "sm" })}>
          Go to Settings
        </Link>
      </AlertDescription>
    </Alert>
  )
}
