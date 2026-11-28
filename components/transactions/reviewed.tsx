import { IconTooltip } from "@/components/icon-tooltip"
import { CheckCircle2Icon } from "lucide-react"

export const ReviewedIcon = () => (
  <IconTooltip label="Reviewed">
    <span className="grid size-9 shrink-0 place-items-center rounded-full bg-emerald-100 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300">
      <CheckCircle2Icon className="size-4" aria-hidden="true" />
      <span className="sr-only">Reviewed</span>
    </span>
  </IconTooltip>
)
