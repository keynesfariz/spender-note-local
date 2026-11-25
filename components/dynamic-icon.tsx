"use client"

import { Tag } from "lucide-react"
import { DynamicIcon as LucideDynamicIcon } from "lucide-react/dynamic"

import { resolveCategoryIconName } from "@/lib/category-icons"
import { cn } from "@/lib/utils"

export function DynamicIcon({
  name,
  className,
}: {
  name?: string | null
  className?: string
}) {
  const iconName = resolveCategoryIconName(name)

  return (
    <span
      aria-hidden="true"
      className={cn("inline-flex size-4 shrink-0", className)}
    >
      {iconName ? (
        <LucideDynamicIcon
          key={iconName}
          name={iconName}
          className="size-full"
        />
      ) : (
        <Tag className="size-full" />
      )}
    </span>
  )
}
