import { PencilIcon, TagIcon, Trash2Icon } from "lucide-react"
import Link from "next/link"
import { memo } from "react"
import { Button, buttonVariants } from "@/components/ui/button"
import { type TagSummary } from "@/lib/api/types.generated"
import { formatTxCount } from "../categories/taxonomy-ui"

export const TagRows = memo(function TagRows({
  items,
  onEdit,
  onDelete,
}: {
  items: TagSummary[]
  onEdit: (item: TagSummary, trigger: HTMLButtonElement) => void
  onDelete: (item: TagSummary) => void
}) {
  return (
    <div className="divide-y divide-border border-t border-border">
      {items.map((tag) => (
        <div
          key={tag.id}
          className="flex flex-wrap items-center justify-between gap-3 px-6 py-3"
        >
          <div className="flex min-w-0 items-center gap-3">
            <div className="flex size-9 shrink-0 items-center justify-center rounded-md border bg-muted/40">
              <TagIcon className="size-4 text-muted-foreground" />
            </div>
            <span className="font-medium text-foreground">{tag.name}</span>
          </div>

          <div className="flex items-center gap-2">
            <Link
              href={`/transactions?tag=${encodeURIComponent(tag.name)}&range=all`}
              className={buttonVariants({
                variant: "ghost",
                size: "xs",
              })}
            >
              {formatTxCount(tag.transaction_count)}
            </Link>

            <Button
              type="button"
              variant="outline"
              size="xs"
              onClick={(event) => onEdit(tag, event.currentTarget)}
            >
              <PencilIcon className="mr-1 size-3" />
              Rename
            </Button>
            <Button
              type="button"
              variant="outline"
              size="xs"
              onClick={() => onDelete(tag)}
            >
              <Trash2Icon className="mr-1 size-3" />
              Delete
            </Button>
          </div>
        </div>
      ))}
    </div>
  )
})
