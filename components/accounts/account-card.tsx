import { DetailDrawerContent } from "@/components/detail-drawer-content"
import { IconTooltip } from "@/components/icon-tooltip"
import { CreditCardIcon, HistoryIcon, WalletIcon } from "lucide-react"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Drawer, DrawerTrigger } from "@/components/ui/drawer"
import { type AccountSummary } from "@/lib/api/types.generated"
import {
  formatMinorCurrency,
  formatSignedMinor,
  formatUtcInTimezone,
} from "@/lib/format"
import {
  AccountBillingDrawer,
  AccountReconciliationHistory,
} from "./account-billing-drawer"
import { cn } from "@/lib/utils"
import { ACCOUNT_TYPE_LABELS } from "./account-type-labels"

export function AccountCard({
  account,
  timezone,
  confirming,
  onConfirm,
  preloadAccountFormDialog,
  preloadReconcileAccountDialog,
  preloadMergeAccountDialog,
  openAccountForm,
  setReconcilingAccount,
  setMergingAccount,
  deleting,
  onDelete,
}: {
  account: AccountSummary
  timezone: string | null | undefined
  confirming: boolean
  onConfirm: (id: string) => void
  preloadAccountFormDialog: () => void
  preloadReconcileAccountDialog: () => void
  preloadMergeAccountDialog: () => void
  openAccountForm: (account: AccountSummary, trigger: HTMLButtonElement) => void
  setReconcilingAccount: (account: AccountSummary) => void
  setMergingAccount: (account: AccountSummary) => void
  deleting: boolean
  onDelete: (id: string) => void
}) {
  const AccountIcon =
    account.type === "digital_wallet" ? WalletIcon : CreditCardIcon
  return (
    <Card className="min-w-0 gap-4">
      <CardHeader className="gap-3">
        <div className="flex min-w-0 items-start gap-3">
          <span
            className={cn(
              "grid size-10 shrink-0 place-items-center rounded-lg",
              {
                credit_card: "bg-credit text-credit-foreground",
                debit_card: "bg-secondary text-secondary-foreground",
                digital_wallet: "bg-wallet text-wallet-foreground",
              }[account.type]
            )}
          >
            <AccountIcon className="size-5" />
          </span>
          <div className="min-w-0">
            <CardTitle className="wrap-break-word">{account.name}</CardTitle>
            <p className="text-xs text-muted-foreground">
              {ACCOUNT_TYPE_LABELS[account.type]}
            </p>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-1.5 empty:hidden">
          {account.status === "provisional" && (
            <Badge variant="outline">Provisional</Badge>
          )}
          {(account.identifiers ?? []).map((ident) => (
            <Badge key={ident.id} variant="outline" className="font-mono">
              {ident.value}
            </Badge>
          ))}
          {account.uncounted_foreign_count > 0 && (
            <Badge variant="outline">
              {account.uncounted_foreign_count} foreign{" "}
              {account.uncounted_foreign_count === 1
                ? "transaction"
                : "transactions"}{" "}
              excluded
            </Badge>
          )}
          {account.future_transaction_count > 0 && (
            <Badge variant="destructive">
              {account.future_transaction_count} future-dated{" "}
              {account.future_transaction_count === 1
                ? "transaction"
                : "transactions"}{" "}
              excluded
            </Badge>
          )}
        </div>
      </CardHeader>

      <CardContent>
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border bg-muted/55 p-3">
          <div>
            {account.computed_balance_minor !== null ? (
              <>
                <p className="text-xs text-muted-foreground">
                  {account.type === "credit_card"
                    ? "Balance owed"
                    : "Available balance"}
                </p>
                <p className="text-xl font-semibold tabular-nums">
                  {formatMinorCurrency(
                    account.computed_balance_minor,
                    account.currency
                  )}
                </p>
              </>
            ) : (
              <>
                <p className="text-xs text-muted-foreground">
                  Net flow (balance unknown)
                </p>
                <p className="text-xl font-semibold tabular-nums">
                  {formatSignedMinor(account.net_flow_minor, account.currency)}
                </p>
              </>
            )}
          </div>
          <div className="flex items-center gap-1">
            <Drawer swipeDirection="right">
              <IconTooltip label="Reconciliation history">
                <DrawerTrigger
                  render={<Button variant="outline" size="icon-sm" />}
                  aria-label={`Reconciliation history for ${account.name}`}
                >
                  <HistoryIcon />
                </DrawerTrigger>
              </IconTooltip>
              <DetailDrawerContent
                title="Reconciliation history"
                description={account.name}
              >
                <AccountReconciliationHistory
                  accountId={account.id}
                  timezone={timezone}
                />
              </DetailDrawerContent>
            </Drawer>
            {account.billing_cycle && (
              <AccountBillingDrawer
                account={account}
                cycle={account.billing_cycle}
              />
            )}
          </div>
        </div>
        {account.latest_reconciliation && (
          <p className="text-xs text-muted-foreground">
            Last reconciled{" "}
            {formatUtcInTimezone(
              account.latest_reconciliation.recorded_at_utc,
              timezone
            )}
          </p>
        )}
      </CardContent>

      <CardFooter className="mt-auto flex flex-wrap items-center gap-2 border-t [&.border-t]:pt-4">
        {account.status === "provisional" && (
          <Button
            type="button"
            size="sm"
            disabled={confirming}
            onClick={() => onConfirm(account.id)}
          >
            Confirm
          </Button>
        )}
        <Button
          type="button"
          variant="outline"
          size="sm"
          onPointerEnter={preloadAccountFormDialog}
          onFocus={preloadAccountFormDialog}
          onClick={(event) => openAccountForm(account, event.currentTarget)}
        >
          Edit
        </Button>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onPointerEnter={preloadReconcileAccountDialog}
          onFocus={preloadReconcileAccountDialog}
          onClick={() => setReconcilingAccount(account)}
        >
          {account.computed_balance_minor === null
            ? "Set balance"
            : "Reconcile"}
        </Button>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onPointerEnter={preloadMergeAccountDialog}
          onFocus={preloadMergeAccountDialog}
          onClick={() => setMergingAccount(account)}
        >
          Merge into...
        </Button>
        <AlertDialog>
          <AlertDialogTrigger
            render={<Button type="button" variant="destructive" size="sm" />}
          >
            Delete
          </AlertDialogTrigger>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>Delete {account.name}?</AlertDialogTitle>
              <AlertDialogDescription>
                This permanently deletes {account.name} and removes its
                identifiers and reconciliation history. This action cannot be
                undone.
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>Cancel</AlertDialogCancel>
              <AlertDialogAction
                variant="destructive"
                disabled={deleting}
                onClick={() => onDelete(account.id)}
              >
                {deleting ? "Deleting..." : "Delete account permanently"}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </CardFooter>
    </Card>
  )
}
