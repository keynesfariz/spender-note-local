'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import Link from 'next/link';
import { useState } from 'react';

import { AccountFormDialog } from '@/components/account-form-dialog';
import { MergeAccountDialog } from '@/components/merge-account-dialog';
import { ReconcileAccountDialog } from '@/components/reconcile-account-dialog';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
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
} from '@/components/ui/alert-dialog';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { Separator } from '@/components/ui/separator';
import { Skeleton } from '@/components/ui/skeleton';
import {
  accountReconciliationsQuery,
  accountsQuery,
  confirmAccountApi,
  deleteAccountApi,
  settingsQuery,
  type AccountSummary,
  type AccountType,
} from '@/lib/api';
import {
  formatMinorCurrency,
  formatSignedMinor,
  formatUtcInTimezone,
} from '@/lib/format';

const ACCOUNT_TYPE_LABELS: Record<AccountType, string> = {
  credit_card: 'Credit Card',
  debit_card: 'Debit Card',
  digital_wallet: 'Digital Wallet',
};

function AccountReconciliationHistory({
  accountId,
  timezone,
}: {
  accountId: string;
  timezone?: string | null;
}) {
  const history = useQuery(accountReconciliationsQuery(accountId));

  return (
    <div className="space-y-2 pt-3">
      <Separator />
      <h3 className="pt-2 text-xs font-semibold tracking-wide text-muted-foreground uppercase">
        Reconciliation history
      </h3>
      {history.isPending ? (
        <p className="text-xs text-muted-foreground">
          Loading reconciliation history...
        </p>
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
                  Actual:{' '}
                  {formatMinorCurrency(rec.actual_balance_minor, rec.currency)}{' '}
                  <span className="text-muted-foreground">
                    (computed{' '}
                    {formatMinorCurrency(
                      rec.computed_balance_minor,
                      rec.currency,
                    )}
                    , adj{' '}
                    {formatSignedMinor(rec.adjustment_minor, rec.currency)})
                  </span>
                </p>
                {rec.note && <p className="text-muted-foreground">{rec.note}</p>}
              </div>
              <span className="text-muted-foreground">
                {formatUtcInTimezone(rec.recorded_at_utc, timezone)}
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

export function AccountsView() {
  const queryClient = useQueryClient();
  const accounts = useQuery(accountsQuery());
  const settings = useQuery(settingsQuery());

  const [formOpen, setFormOpen] = useState(false);
  const [editingAccount, setEditingAccount] = useState<AccountSummary | null>(null);
  const [reconcilingAccount, setReconcilingAccount] =
    useState<AccountSummary | null>(null);
  const [mergingAccount, setMergingAccount] = useState<AccountSummary | null>(null);
  const [expandedHistoryId, setExpandedHistoryId] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const hasCurrency = Boolean(settings.data?.currency);
  const provisionalCount = (accounts.data ?? []).filter(
    (a) => a.status === 'provisional',
  ).length;

  const confirmMutation = useMutation({
    mutationFn: confirmAccountApi,
    onSuccess: () => {
      setActionError(null);
      void queryClient.invalidateQueries({ queryKey: ['accounts'] });
    },
    onError: (err) => {
      setActionError(
        err instanceof Error ? err.message : 'Could not confirm account.',
      );
    },
  });

  const deleteMutation = useMutation({
    mutationFn: deleteAccountApi,
    onSuccess: () => {
      setActionError(null);
      void queryClient.invalidateQueries({ queryKey: ['accounts'] });
      void queryClient.invalidateQueries({ queryKey: ['senders'] });
      void queryClient.invalidateQueries({ queryKey: ['spending'] });
      void queryClient.invalidateQueries({ queryKey: ['transactions'] });
    },
    onError: (err) => {
      setActionError(
        err instanceof Error ? err.message : 'Could not delete account.',
      );
    },
  });

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Accounts</h1>
          <p className="text-sm text-muted-foreground">
            Manage credit cards, debit cards, and digital wallets and reconcile
            their balances.
          </p>
        </div>

        <Button
          type="button"
          disabled={!hasCurrency}
          onClick={() => {
            setEditingAccount(null);
            setFormOpen(true);
          }}
        >
          Add account
        </Button>
      </div>

      {!settings.isPending && !hasCurrency && (
        <Alert>
          <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
            <span>
              Configure your country and currency in Settings before adding
              accounts.
            </span>
            <Button size="sm" render={<Link href="/settings" />}>
              Go to Settings
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {provisionalCount > 0 && (
        <Alert>
          <AlertTitle>
            {provisionalCount} provisional{' '}
            {provisionalCount === 1 ? 'account was' : 'accounts were'}{' '}
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
        <div className="space-y-4">
          <Skeleton className="h-32 w-full" />
          <Skeleton className="h-32 w-full" />
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
        <div className="space-y-4">
          {(accounts.data ?? []).map((account) => (
            <Collapsible
              key={account.id}
              open={expandedHistoryId === account.id}
              onOpenChange={(open) =>
                setExpandedHistoryId(open ? account.id : null)
              }
            >
              <Card>
                <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-4">
                  <div className="space-y-2">
                    <div className="flex flex-wrap items-center gap-2">
                      <CardTitle>{account.name}</CardTitle>
                      <Badge variant="secondary">
                        {ACCOUNT_TYPE_LABELS[account.type]}
                      </Badge>
                      {account.status === 'provisional' && (
                        <Badge variant="outline">Provisional</Badge>
                      )}
                      {(account.identifiers ?? []).length > 0
                        ? account.identifiers.map((ident) => (
                            <Badge key={ident.id} variant="outline" className="font-mono">
                              {ident.kind === 'card_last4'
                                ? `•••• ${ident.value}`
                                : ident.value}
                            </Badge>
                          ))
                        : account.last4 && (
                            <Badge variant="outline" className="font-mono">
                              •••• {account.last4}
                            </Badge>
                          )}
                      {account.uncounted_foreign_count > 0 && (
                        <Badge variant="outline">
                          {account.uncounted_foreign_count} foreign{' '}
                          {account.uncounted_foreign_count === 1
                            ? 'transaction'
                            : 'transactions'}{' '}
                          excluded
                        </Badge>
                      )}
                    </div>

                    {account.latest_reconciliation && (
                      <p className="text-xs text-muted-foreground">
                        Last reconciled{' '}
                        {formatUtcInTimezone(
                          account.latest_reconciliation.recorded_at_utc,
                          settings.data?.timezone,
                        )}{' '}
                        — Actual{' '}
                        {formatMinorCurrency(
                          account.latest_reconciliation.actual_balance_minor,
                          account.latest_reconciliation.currency,
                        )}{' '}
                        (adj{' '}
                        {formatSignedMinor(
                          account.latest_reconciliation.adjustment_minor,
                          account.latest_reconciliation.currency,
                        )}
                        )
                        {account.latest_reconciliation.note &&
                          ` · ${account.latest_reconciliation.note}`}
                      </p>
                    )}
                  </div>

                  <div className="text-right">
                    {account.computed_balance_minor !== null ? (
                      <>
                        <p className="text-xs text-muted-foreground">
                          {account.type === 'credit_card'
                            ? 'Balance owed'
                            : 'Available balance'}
                        </p>
                        <p className="text-xl font-semibold tabular-nums">
                          {formatMinorCurrency(
                            account.computed_balance_minor,
                            account.currency,
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
                            account.currency,
                          )}
                        </p>
                        <Button
                          type="button"
                          variant="link"
                          size="sm"
                          onClick={() => setReconcilingAccount(account)}
                        >
                          Set balance
                        </Button>
                      </>
                    )}
                  </div>
                </CardHeader>

                <CardFooter className="flex flex-wrap items-center gap-2">
                  {account.status === 'provisional' && (
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
                    onClick={() => {
                      setEditingAccount(account);
                      setFormOpen(true);
                    }}
                  >
                    Edit
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => setReconcilingAccount(account)}
                  >
                    {account.computed_balance_minor === null
                      ? 'Set balance'
                      : 'Reconcile'}
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => setMergingAccount(account)}
                  >
                    Merge into...
                  </Button>
                  <CollapsibleTrigger
                    render={<Button type="button" variant="outline" size="sm" />}
                  >
                    {expandedHistoryId === account.id ? 'Hide history' : 'History'}
                  </CollapsibleTrigger>

                  <div className="ml-auto">
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
                              ? 'Deleting...'
                              : 'Delete account permanently'}
                          </AlertDialogAction>
                        </AlertDialogFooter>
                      </AlertDialogContent>
                    </AlertDialog>
                  </div>
                </CardFooter>

                <CollapsibleContent>
                  <CardContent>
                    {expandedHistoryId === account.id && (
                      <AccountReconciliationHistory
                        accountId={account.id}
                        timezone={settings.data?.timezone}
                      />
                    )}
                  </CardContent>
                </CollapsibleContent>
              </Card>
            </Collapsible>
          ))}
        </div>
      )}

      <AccountFormDialog
        open={formOpen}
        onOpenChange={(open) => {
          setFormOpen(open);
          if (!open) setEditingAccount(null);
        }}
        account={editingAccount}
        currency={settings.data?.currency ?? 'USD'}
        timezone={settings.data?.timezone}
      />

      <ReconcileAccountDialog
        open={reconcilingAccount !== null}
        onOpenChange={(open) => {
          if (!open) setReconcilingAccount(null);
        }}
        account={reconcilingAccount}
      />

      <MergeAccountDialog
        open={mergingAccount !== null}
        onOpenChange={(open) => {
          if (!open) setMergingAccount(null);
        }}
        sourceAccount={mergingAccount}
        allAccounts={accounts.data ?? []}
      />
    </div>
  );
}
