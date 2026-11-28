"use client"

import { AccountCard } from "./account-card"
import { LocaleSetupAlert } from "@/components/locale/locale-setup-alert"
import { TransferFormDialog } from "@/components/transfer/transfer-form-dialog"
import { useHasOpened } from "@/hooks/use-has-opened"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import dynamic from "next/dynamic"
import { useRef, useState } from "react"

import { PageLayout } from "@/components/PageLayout"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { DrawerTrigger } from "@/components/ui/drawer"
import { Skeleton } from "@/components/ui/skeleton"
import {
  accountsQuery,
  confirmAccountApi,
  deleteAccountApi,
} from "@/lib/api/accounts"
import { localeQuery } from "@/lib/api/settings"
import { type AccountSummary } from "@/lib/api/types.generated"

const AccountFormDialog = dynamic(() =>
  import("@/components/accounts/account-form-dialog").then(
    (mod) => mod.AccountFormDialog
  )
)
const ReconcileAccountDialog = dynamic(() =>
  import("@/components/accounts/reconcile-account-dialog").then(
    (mod) => mod.ReconcileAccountDialog
  )
)
const MergeAccountDialog = dynamic(() =>
  import("@/components/accounts/merge-account-dialog").then(
    (mod) => mod.MergeAccountDialog
  )
)

function preloadAccountFormDialog() {
  void import("@/components/accounts/account-form-dialog").catch(() => {})
}

function preloadReconcileAccountDialog() {
  void import("@/components/accounts/reconcile-account-dialog").catch(() => {})
}

function preloadMergeAccountDialog() {
  void import("@/components/accounts/merge-account-dialog").catch(() => {})
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
          {(accounts.data ?? []).map((account) => (
            <AccountCard
              key={account.id}
              account={account}
              timezone={settings.data?.timezone}
              confirming={confirmMutation.isPending}
              onConfirm={confirmMutation.mutate}
              preloadAccountFormDialog={preloadAccountFormDialog}
              preloadReconcileAccountDialog={preloadReconcileAccountDialog}
              preloadMergeAccountDialog={preloadMergeAccountDialog}
              openAccountForm={openAccountForm}
              setReconcilingAccount={setReconcilingAccount}
              setMergingAccount={setMergingAccount}
              deleting={deleteMutation.isPending}
              onDelete={deleteMutation.mutate}
            />
          ))}
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
