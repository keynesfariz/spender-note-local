import type { ComponentProps } from "react"
import { cn } from "@/lib/utils"

export function Textarea({ className, ...props }: ComponentProps<"textarea">) {
  return (
    <textarea
      data-slot="textarea"
      className={cn(
        "max-h-[55dvh] min-h-24 w-full resize-y rounded-md border border-input bg-transparent px-2.5 py-2 text-base shadow-xs outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:opacity-50 md:text-sm dark:bg-input/30",
        className
      )}
      {...props}
    />
  )
}
