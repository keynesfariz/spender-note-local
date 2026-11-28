import { PencilIcon, Trash2Icon } from "lucide-react"
import Link from "next/link"
import { memo } from "react"
import { DynamicIcon } from "@/components/dynamic-icon"
import { Badge } from "@/components/ui/badge"
import { Button, buttonVariants } from "@/components/ui/button"
import { type CategorySummary } from "@/lib/api/types.generated"
import { formatTxCount } from "./taxonomy-ui"

export const CategoryRows = memo(function CategoryRows({
  items,
  onEdit,
  onDelete,
}: {
  items: CategorySummary[]
  onEdit: (item: CategorySummary, trigger: HTMLButtonElement) => void
  onDelete: (item: CategorySummary) => void
}) {
  return (
    <div className="divide-y divide-border border-t border-border">
      {items.map((cat) => {
        const isSystem = cat.is_system === 1
        return (
          <div
            key={cat.id}
            className="flex flex-wrap items-center justify-between gap-3 px-6 py-3"
          >
            <div className="flex min-w-0 items-center gap-3">
              <div className="flex size-9 shrink-0 items-center justify-center rounded-md border bg-muted/40">
                <DynamicIcon
                  name={cat.icon}
                  className="size-4 text-foreground"
                />
              </div>
              <div className="flex min-w-0 flex-wrap items-center gap-2">
                <span className="font-medium text-foreground">{cat.name}</span>
                {isSystem ? (
                  <Badge variant="secondary">System default</Badge>
                ) : null}
              </div>
            </div>

            <div className="flex items-center gap-2">
              <Link
                href={`/transactions?category=${encodeURIComponent(cat.name)}&range=all`}
                className={buttonVariants({
                  variant: "ghost",
                  size: "xs",
                })}
              >
                {formatTxCount(cat.transaction_count)}
              </Link>

              {isSystem ? (
                <span className="px-2 text-xs text-muted-foreground">
                  Read-only
                </span>
              ) : (
                <>
                  <Button
                    type="button"
                    variant="outline"
                    size="xs"
                    onClick={(event) => onEdit(cat, event.currentTarget)}
                  >
                    <PencilIcon className="mr-1 size-3" />
                    Edit
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    size="xs"
                    onClick={() => onDelete(cat)}
                  >
                    <Trash2Icon className="mr-1 size-3" />
                    Delete
                  </Button>
                </>
              )}
            </div>
          </div>
        )
      })}
    </div>
  )
})
