'use client';

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useEffect, useMemo, useState, type FormEvent } from 'react';

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { mergeAccountsApi, type AccountSummary, type AccountType } from '@/lib/api';
import { formatMinorCurrency } from '@/lib/format';

const ACCOUNT_TYPE_LABELS: Record<AccountType, string> = {
  credit_card: 'Credit Card',
  debit_card: 'Debit Card',
  digital_wallet: 'Digital Wallet',
};

export function MergeAccountDialog({
  open,
  onOpenChange,
  sourceAccount,
  allAccounts,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  sourceAccount: AccountSummary | null;
  allAccounts: AccountSummary[];
}) {
  const queryClient = useQueryClient();
  const [targetId, setTargetId] = useState('');
  const [formError, setFormError] = useState<string | null>(null);

  const eligibleTargets = useMemo(() => {
    if (!sourceAccount) return [];
    const sourceIsCredit = sourceAccount.type === 'credit_card';
    return allAccounts.filter(
      (c) =>
        c.id !== sourceAccount.id &&
        c.currency === sourceAccount.currency &&
        (c.type === 'credit_card') === sourceIsCredit,
    );
  }, [sourceAccount, allAccounts]);

  const hasExcludedAccounts =
    sourceAccount !== null && allAccounts.length - 1 > eligibleTargets.length;

  useEffect(() => {
    if (!open) return;
    setTargetId(eligibleTargets[0]?.id ?? '');
    setFormError(null);
  }, [open, eligibleTargets]);

  const selectedTarget =
    eligibleTargets.find((a) => a.id === targetId) ?? null;

  const mergeMutation = useMutation({
    mutationFn: mergeAccountsApi,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['accounts'] });
      void queryClient.invalidateQueries({ queryKey: ['senders'] });
      void queryClient.invalidateQueries({ queryKey: ['spending'] });
      void queryClient.invalidateQueries({ queryKey: ['transactions'] });
      onOpenChange(false);
    },
    onError: (err) => {
      setFormError(
        err instanceof Error ? err.message : 'Could not merge accounts.',
      );
    },
  });

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setFormError(null);
    if (!sourceAccount || !selectedTarget) {
      setFormError('Select a destination account.');
      return;
    }
    mergeMutation.mutate({
      sourceAccountId: sourceAccount.id,
      targetAccountId: selectedTarget.id,
    });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Merge {sourceAccount?.name ?? 'account'}</DialogTitle>
          <DialogDescription>
            Move all transactions and identifiers into another account and delete
            this one.
          </DialogDescription>
        </DialogHeader>

        {sourceAccount && (
          <form onSubmit={handleSubmit} className="space-y-4">
            {eligibleTargets.length === 0 ? (
              <Alert>
                <AlertDescription>
                  No eligible destination accounts. Accounts can only merge with
                  another account of the same currency ({sourceAccount.currency}
                  ), and credit cards can only merge with other credit cards.
                </AlertDescription>
              </Alert>
            ) : (
              <>
                <Field>
                  <FieldLabel>Destination account</FieldLabel>
                  <Select
                    value={targetId}
                    onValueChange={(v) => setTargetId(v ?? '')}
                  >
                    <SelectTrigger
                      className="w-full"
                      aria-label="Destination account"
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {eligibleTargets.map((target) => (
                        <SelectItem key={target.id} value={target.id}>
                          {target.name} ({ACCOUNT_TYPE_LABELS[target.type]}
                          {target.computed_balance_minor !== null
                            ? ` · ${formatMinorCurrency(target.computed_balance_minor, target.currency)}`
                            : ' · balance unknown'}
                          )
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {hasExcludedAccounts && (
                    <FieldDescription>
                      Only same-currency accounts are shown. Credit cards can
                      only merge with other credit cards.
                    </FieldDescription>
                  )}
                </Field>

                <Alert variant="destructive">
                  <AlertTitle>This action cannot be undone.</AlertTitle>
                  <AlertDescription>
                    All transactions, identifiers, and sender defaults on{' '}
                    {sourceAccount.name} will move to{' '}
                    {selectedTarget?.name ?? 'the destination account'}, and{' '}
                    {sourceAccount.name} will be permanently deleted. Duplicate
                    detection will re-run on the destination account.
                  </AlertDescription>
                </Alert>
              </>
            )}

            {formError && (
              <Alert variant="destructive">
                <AlertDescription>{formError}</AlertDescription>
              </Alert>
            )}

            <DialogFooter>
              <DialogClose render={<Button type="button" variant="outline" />}>
                Cancel
              </DialogClose>
              {eligibleTargets.length > 0 && (
                <Button
                  type="submit"
                  variant="destructive"
                  disabled={mergeMutation.isPending || !selectedTarget}
                >
                  {mergeMutation.isPending ? 'Merging...' : 'Confirm merge'}
                </Button>
              )}
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}
