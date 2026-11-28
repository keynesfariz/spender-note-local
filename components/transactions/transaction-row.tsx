import { Clock3Icon, PencilIcon } from "lucide-react"
import { memo } from "react"
import { CategoryBadge } from "@/components/categories/category-badge"
import { IconTooltip } from "@/components/icon-tooltip"
import { ReviewedIcon } from "@/components/transactions/reviewed"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Separator } from "@/components/ui/separator"
import { type TransactionListItem } from "@/lib/api/types.generated"
import {
  formatTransactionAmount,
  formatUtcInTimezone,
  transactionKindLabel,
} from "@/lib/format"

function sourceBadgeLabel(tx: TransactionListItem): string {
  if (tx.sourceCount > 1) return `${tx.sourceCount} emails`
  if (tx.sourceCount === 1) return "1 email"
  if (tx.source_type === "manual") return "Manual"
  return "0 emails"
}

export const TransactionRow = memo(function TransactionRow({
  tx,
  categoryIcon,
  timezone,
  isSelected,
  separated,
  actionsDisabled,
  onToggleRow,
  onSelectTransaction,
  onMarkReviewed,
}: {
  tx: TransactionListItem
  categoryIcon: string | undefined
  timezone: string | null | undefined
  isSelected: boolean
  separated: boolean
  actionsDisabled: boolean
  onToggleRow: (id: string, checked: boolean) => void
  onSelectTransaction: (id: string) => void
  onMarkReviewed: (ids: string[]) => void
}) {
  const title = tx.remark ?? tx.merchant
  const isTransfer = tx.kind === "transfer"
  const kindLabel = transactionKindLabel(tx.kind)
  const formattedAmount = formatTransactionAmount(tx)
  return (
    <li>
      {separated && <Separator className="my-1" />}
      <div
        data-selected={isSelected}
        className="flex items-center gap-2 rounded-lg p-3 transition-colors hover:bg-muted data-[selected=true]:bg-secondary/60 data-[selected=true]:hover:bg-secondary"
      >
        <Checkbox
          aria-label={`Select ${kindLabel.toLowerCase()} ${title} (${formattedAmount})`}
          checked={isSelected}
          onCheckedChange={(checked) => onToggleRow(tx.id, Boolean(checked))}
        />
        <Button
          type="button"
          variant="ghost"
          aria-label={`Inspect ${kindLabel.toLowerCase()} ${title} (${formattedAmount})${tx.manual_override === 1 ? ", edited" : ""}`}
          onClick={() => onSelectTransaction(tx.id)}
          className="grid h-auto min-w-0 flex-1 grid-cols-1 items-center gap-x-4 gap-y-2 px-2 text-left font-normal whitespace-normal hover:bg-transparent sm:grid-cols-[minmax(0,1fr)_auto] dark:hover:bg-transparent"
        >
          <span className="min-w-0 space-y-1">
            <span className="flex items-center gap-2">
              <span className="truncate font-semibold">{title}</span>
              {tx.manual_override === 1 && (
                <IconTooltip label="Edited">
                  <span className="text-muted-foreground">
                    <PencilIcon className="size-3" aria-hidden="true" />
                  </span>
                </IconTooltip>
              )}
            </span>
            <span className="block text-xs text-muted-foreground">
              {tx.account_name ??
                (tx.payment_method_raw
                  ? `Unassigned (${tx.payment_method_raw})`
                  : "Unassigned")}{" "}
              {isTransfer && (
                <> → {tx.destination_account_name ?? "Unknown account"}</>
              )}
              {" · "}
              {formatUtcInTimezone(tx.occurred_at_utc, timezone)}
            </span>
          </span>
          <span className="flex flex-wrap items-center gap-1.5 sm:col-start-1 sm:row-start-2">
            <Badge variant="outline">{kindLabel}</Badge>
            <CategoryBadge category={tx.category} icon={categoryIcon} />
            {(tx.tags ?? []).map((tag) => (
              <Badge
                key={tag.id}
                variant="outline"
                className="text-muted-foreground"
              >
                #{tag.name}
              </Badge>
            ))}
          </span>
          <span className="min-w-0 sm:col-start-2 sm:row-span-2 sm:row-start-1 sm:text-right">
            <span className="block font-semibold break-all tabular-nums">
              {formattedAmount}
            </span>
            <span className="block text-xs text-muted-foreground">
              {sourceBadgeLabel(tx)}
              {tx.isForeignCurrency ? ` · Foreign (${tx.currency})` : ""}
            </span>
          </span>
        </Button>
        {tx.reviewed === 1 ? (
          <ReviewedIcon />
        ) : (
          <IconTooltip label="Needs review · Mark reviewed">
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={`Mark ${title} as reviewed`}
              disabled={actionsDisabled}
              onClick={() => onMarkReviewed([tx.id])}
              className="rounded-full bg-amber-100 text-amber-800 hover:bg-amber-200 hover:text-amber-950 dark:bg-amber-950 dark:text-amber-300 dark:hover:bg-amber-900"
            >
              <Clock3Icon className="size-4" aria-hidden="true" />
            </Button>
          </IconTooltip>
        )}
      </div>
    </li>
  )
})
