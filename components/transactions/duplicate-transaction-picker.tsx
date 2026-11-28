import { Badge } from "@/components/ui/badge"
import { type TransactionListItem } from "@/lib/api/types.generated"
import { formatMinorCurrency, formatUtcInTimezone } from "@/lib/format"
import { cn } from "@/lib/utils"

export function DuplicateTransactionPicker({
  transactions,
  keepId,
  setKeepId,
  timezone,
}: {
  transactions: TransactionListItem[]
  keepId: string
  setKeepId: (id: string) => void
  timezone: string | null | undefined
}) {
  return (
    <div
      role="radiogroup"
      aria-label="Choose transaction to keep"
      className="space-y-2"
    >
      {transactions.map((tx) => {
        const selected = keepId === tx.id
        return (
          <label
            key={tx.id}
            className={cn(
              "flex cursor-pointer items-start gap-3 rounded-lg border p-3 text-sm transition-colors",
              selected
                ? "border-primary bg-primary/5 ring-1 ring-primary"
                : "hover:bg-muted/50"
            )}
          >
            <input
              type="radio"
              name="keep-transaction"
              value={tx.id}
              checked={selected}
              onChange={() => setKeepId(tx.id)}
              className="mt-1 size-4 accent-primary"
            />
            <div className="min-w-0 flex-1 space-y-1">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <span className="truncate font-medium">
                  {tx.remark ?? tx.merchant}
                </span>
                <span className="font-medium tabular-nums">
                  {formatMinorCurrency(tx.amount_minor, tx.currency)}
                </span>
              </div>
              <div className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
                <Badge variant="secondary">{tx.category}</Badge>
                <Badge variant="outline">
                  {tx.sourceCount > 0
                    ? `${tx.sourceCount} ${tx.sourceCount === 1 ? "email" : "emails"}`
                    : "Manual"}
                </Badge>
                <span>
                  {tx.account_name ?? "Unassigned"} ·{" "}
                  {formatUtcInTimezone(tx.occurred_at_utc, timezone)}
                </span>
              </div>
              {tx.note && (
                <p className="text-xs text-muted-foreground">Note: {tx.note}</p>
              )}
            </div>
          </label>
        )
      })}
    </div>
  )
}
