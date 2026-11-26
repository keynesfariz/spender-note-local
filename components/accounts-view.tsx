"use client"

import { DetailDrawerContent } from "@/components/detail-drawer-content"
import { IconTooltip } from "@/components/icon-tooltip"
import { LocaleSetupAlert } from "@/components/locale-setup-alert"
import { TransferFormDialog } from "@/components/transfer-form-dialog"
import { useHasOpened } from "@/hooks/use-has-opened"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  CalendarRangeIcon,
  CreditCardIcon,
  HistoryIcon,
  WalletIcon,
} from "lucide-react"
import dynamic from "next/dynamic"
import { useRef, useState } from "react"

import { PageLayout } from "@/components/PageLayout"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
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
import { Skeleton } from "@/components/ui/skeleton"
import {
  accountReconciliationsQuery,
  accountsQuery,
  confirmAccountApi,
  deleteAccountApi,
} from "@/lib/api/accounts"
import { localeQuery } from "@/lib/api/settings"
import {
  type AccountSummary,
  type AccountType,
} from "@/lib/api/types.generated"
import {
  formatDateRangeLabel,
  formatMinorCurrency,
  formatSignedMinor,
  formatUtcInTimezone,
} from "@/lib/format"
import { cn } from "@/lib/utils"

const AccountFormDialog = dynamic(() =>
  import("@/components/account-form-dialog").then(
    (mod) => mod.AccountFormDialog
  )
)
const ReconcileAccountDialog = dynamic(() =>
  import("@/components/reconcile-account-dialog").then(
    (mod) => mod.ReconcileAccountDialog
  )
)
const MergeAccountDialog = dynamic(() =>
  import("@/components/merge-account-dialog").then(
    (mod) => mod.MergeAccountDialog
  )
)

function preloadAccountFormDialog() {
  void import("@/components/account-form-dialog").catch(() => {})
}

function preloadReconcileAccountDialog() {
  void import("@/components/reconcile-account-dialog").catch(() => {})
}

function preloadMergeAccountDialog() {
  void import("@/components/merge-account-dialog").catch(() => {})
}

const ACCOUNT_TYPE_LABELS: Record<AccountType, string> = {
  credit_card: "Credit Card",
  debit_card: "Debit Card",
  digital_wallet: "Digital Wallet",
}

function AccountReconciliationHistory({
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

function AccountBillingDrawer({
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

export function AccountsView() {
  const queryClient = useQueryClient()
  const accounts = useQuery(accountsQuery())
  const settings = useQuery(localeQuery())

  const formTrigger = useRef<HTMLButtonElement>(null)
  const [formOpen, setFormOpen] = useState(false)
  const [editingAccount, setEditingAccount] = useState<AccountSummary | null>(
    null
  )
  const [reconcilingAccount, setReconcilingAccount] =
    useState<AccountSummary | null>(null)
  const [mergingAccount, setMergingAccount] = useState<AccountSummary | null>(
    null
  )
  const hasOpenedForm = useHasOpened(formOpen)
  const hasOpenedReconcile = useHasOpened(reconcilingAccount !== null)
  const hasOpenedMerge = useHasOpened(mergingAccount !== null)
  const [actionError, setActionError] = useState<string | null>(null)

  function openAccountForm(
    account: AccountSummary | null,
    trigger: HTMLButtonElement
  ) {
    formTrigger.current = trigger
    setEditingAccount(account)
    setFormOpen(true)
  }

  const hasCurrency = Boolean(settings.data?.currency)
  const provisionalCount = (accounts.data ?? []).reduce(
    (count, account) => count + Number(account.status === "provisional"),
    0
  )

  const confirmMutation = useMutation({
    mutationFn: confirmAccountApi,
    onSuccess: () => {
      setActionError(null)
      void queryClient.invalidateQueries({ queryKey: ["accounts"] })
    },
    onError: (err) => {
      setActionError(
        err instanceof Error ? err.message : "Could not confirm account."
      )
    },
  })

  const deleteMutation = useMutation({
    mutationFn: deleteAccountApi,
    onSuccess: () => {
      setActionError(null)
      void queryClient.invalidateQueries({ queryKey: ["accounts"] })
      void queryClient.invalidateQueries({ queryKey: ["senders"] })
      void queryClient.invalidateQueries({ queryKey: ["spending"] })
      void queryClient.invalidateQueries({ queryKey: ["transactions"] })
    },
    onError: (err) => {
      setActionError(
        err instanceof Error ? err.message : "Could not delete account."
      )
    },
  })

  return (
    <PageLayout
      title="Accounts"
      subtitle="Manage credit cards, debit cards, and digital wallets and reconcile
            their balances."
      actions={
        <>
          <TransferFormDialog
            targets={[{}]}
            timezone={settings.data?.timezone}
            trigger={
              <DrawerTrigger
                render={<Button type="button" variant="outline" />}
                disabled={!hasCurrency || (accounts.data?.length ?? 0) < 2}
              >
                Transfer
              </DrawerTrigger>
            }
          />
          <Button
            type="button"
            disabled={!hasCurrency}
            onPointerEnter={preloadAccountFormDialog}
            onFocus={preloadAccountFormDialog}
            onClick={(event) => openAccountForm(null, event.currentTarget)}
          >
            Add account
          </Button>
        </>
      }
    >
      {!settings.isPending && !hasCurrency && (
        <LocaleSetupAlert showTitle={false}>
          Configure your country and currency in Settings before adding
          accounts.
        </LocaleSetupAlert>
      )}

      {provisionalCount > 0 && (
        <Alert>
          <AlertTitle>
            {provisionalCount} provisional{" "}
            {provisionalCount === 1 ? "account was" : "accounts were"}{" "}
            auto-created from synced emails.
          </AlertTitle>
          <AlertDescription>
            Confirm each account, set its balance, or merge duplicates into an
            existing account.
          </AlertDescription>
        </Alert>
      )}

      {actionError && (
        <Alert variant="destructive">
          <AlertDescription>{actionError}</AlertDescription>
        </Alert>
      )}

      {accounts.isPending ? (
        <div className="grid gap-4 md:grid-cols-2">
          <Skeleton className="h-56 w-full" />
          <Skeleton className="h-56 w-full" />
        </div>
      ) : (accounts.data ?? []).length === 0 ? (
        <Card>
          <CardContent className="py-10 text-center">
            <p className="font-medium">No accounts yet</p>
            <p className="mt-1 text-sm text-muted-foreground">
              Add a card or wallet to track balances and match transactions.
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 md:grid-cols-2">
          {(accounts.data ?? []).map((account) => {
            const AccountIcon =
              account.type === "digital_wallet" ? WalletIcon : CreditCardIcon
            return (
              <Card key={account.id} className="min-w-0 gap-4">
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
                      <CardTitle className="wrap-break-word">
                        {account.name}
                      </CardTitle>
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
                      <Badge
                        key={ident.id}
                        variant="outline"
                        className="font-mono"
                      >
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
                            {formatSignedMinor(
                              account.net_flow_minor,
                              account.currency
                            )}
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
                            timezone={settings.data?.timezone}
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
                        settings.data?.timezone
                      )}
                    </p>
                  )}
                </CardContent>

                <CardFooter className="mt-auto flex flex-wrap items-center gap-2 border-t [&.border-t]:pt-4">
                  {account.status === "provisional" && (
                    <Button
                      type="button"
                      size="sm"
                      disabled={confirmMutation.isPending}
                      onClick={() => confirmMutation.mutate(account.id)}
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
                    onClick={(event) =>
                      openAccountForm(account, event.currentTarget)
                    }
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
                      render={
                        <Button type="button" variant="destructive" size="sm" />
                      }
                    >
                      Delete
                    </AlertDialogTrigger>
                    <AlertDialogContent>
                      <AlertDialogHeader>
                        <AlertDialogTitle>
                          Delete {account.name}?
                        </AlertDialogTitle>
                        <AlertDialogDescription>
                          This permanently deletes {account.name} and removes
                          its identifiers and reconciliation history. This
                          action cannot be undone.
                        </AlertDialogDescription>
                      </AlertDialogHeader>
                      <AlertDialogFooter>
                        <AlertDialogCancel>Cancel</AlertDialogCancel>
                        <AlertDialogAction
                          variant="destructive"
                          disabled={deleteMutation.isPending}
                          onClick={() => deleteMutation.mutate(account.id)}
                        >
                          {deleteMutation.isPending
                            ? "Deleting..."
                            : "Delete account permanently"}
                        </AlertDialogAction>
                      </AlertDialogFooter>
                    </AlertDialogContent>
                  </AlertDialog>
                </CardFooter>
              </Card>
            )
          })}
        </div>
      )}

      {hasOpenedForm && (
        <AccountFormDialog
          open={formOpen}
          onOpenChange={setFormOpen}
          account={editingAccount}
          currency={settings.data?.currency ?? "USD"}
          timezone={settings.data?.timezone}
          formTrigger={formTrigger}
        />
      )}

      {hasOpenedReconcile && (
        <ReconcileAccountDialog
          open={reconcilingAccount !== null}
          onOpenChange={(open) => {
            if (!open) setReconcilingAccount(null)
          }}
          account={reconcilingAccount}
          timezone={settings.data?.timezone}
        />
      )}

      {hasOpenedMerge && (
        <MergeAccountDialog
          open={mergingAccount !== null}
          onOpenChange={(open) => {
            if (!open) setMergingAccount(null)
          }}
          sourceAccount={mergingAccount}
          allAccounts={accounts.data ?? []}
        />
      )}
    </PageLayout>
  )
}
