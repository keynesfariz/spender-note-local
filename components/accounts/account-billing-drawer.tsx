import { DetailDrawerContent } from "@/components/detail-drawer-content"
import { IconTooltip } from "@/components/icon-tooltip"
import { useQuery } from "@tanstack/react-query"
import { CalendarRangeIcon } from "lucide-react"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Drawer, DrawerTrigger } from "@/components/ui/drawer"
import { accountReconciliationsQuery } from "@/lib/api/accounts"
import { type AccountSummary } from "@/lib/api/types.generated"
import {
  formatDateRangeLabel,
  formatMinorCurrency,
  formatSignedMinor,
  formatUtcInTimezone,
} from "@/lib/format"

export function AccountBillingDrawer({
  account,
  cycle,
}: {
  account: AccountSummary
  cycle: NonNullable<AccountSummary["billing_cycle"]>
}) {
  const cycles = [
    { label: "Unbilled", period: cycle.current_cycle },
    { label: "Last cycle", period: cycle.last_cycle },
  ]

  return (
    <Drawer swipeDirection="right">
      <IconTooltip label="Billing cycles · Unbilled and last cycle">
        <DrawerTrigger
          render={<Button variant="outline" size="icon-sm" />}
          aria-label={`Billing cycles for ${account.name}`}
        >
          <CalendarRangeIcon />
        </DrawerTrigger>
      </IconTooltip>
      <DetailDrawerContent title="Billing cycles" description={account.name}>
        <dl className="grid gap-3 sm:grid-cols-2">
          {cycles.map(({ label, period }) => (
            <div key={label} className="rounded-xl border bg-muted/50 p-4">
              <dt className="font-medium">{label}</dt>
              <dd className="mt-1 text-xl font-semibold tabular-nums">
                {formatMinorCurrency(period.spent_minor, account.currency)}
              </dd>
              <dd className="mt-1 text-xs text-muted-foreground">
                {formatDateRangeLabel(period.start_date, period.end_date)}
              </dd>
            </div>
          ))}
        </dl>
      </DetailDrawerContent>
    </Drawer>
  )
}

export function AccountReconciliationHistory({
  accountId,
  timezone,
}: {
  accountId: string
  timezone?: string | null
}) {
  const history = useQuery(accountReconciliationsQuery(accountId))

  return (
    <div className="space-y-3">
      {history.isPending ? (
        <p className="text-xs text-muted-foreground">
          Loading reconciliation history...
        </p>
      ) : history.isError ? (
        <Alert variant="destructive">
          <AlertDescription>
            Could not load reconciliation history.
          </AlertDescription>
        </Alert>
      ) : (history.data ?? []).length === 0 ? (
        <p className="text-xs text-muted-foreground">
          No reconciliations recorded yet.
        </p>
      ) : (
        <ul className="space-y-2">
          {(history.data ?? []).map((rec) => (
            <li
              key={rec.id}
              className="flex flex-wrap items-center justify-between gap-2 rounded-md border px-3 py-2 text-xs"
            >
              <div>
                <p className="font-medium">
                  Actual:{" "}
                  {formatMinorCurrency(rec.actual_balance_minor, rec.currency)}{" "}
                  <span className="text-muted-foreground">
                    (computed{" "}
                    {formatMinorCurrency(
                      rec.computed_balance_minor,
                      rec.currency
                    )}
                    , adj{" "}
                    {formatSignedMinor(rec.adjustment_minor, rec.currency)})
                  </span>
                </p>
                {rec.note && (
                  <p className="text-muted-foreground">{rec.note}</p>
                )}
              </div>
              <span className="text-muted-foreground">
                {formatUtcInTimezone(rec.recorded_at_utc, timezone)}
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
