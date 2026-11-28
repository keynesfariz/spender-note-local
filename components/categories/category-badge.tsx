import { DynamicIcon } from "@/components/dynamic-icon"
import { Badge } from "@/components/ui/badge"

export function CategoryBadge({
  category,
  icon,
}: {
  category: string
  icon: string | undefined
}) {
  return (
    <Badge variant="secondary">
      <DynamicIcon name={icon} className="size-3" />
      {category}
    </Badge>
  )
}
