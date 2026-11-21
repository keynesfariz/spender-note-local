'use client';

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useEffect, useMemo, useState, type FormEvent } from 'react';

import { Alert, AlertDescription } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Field, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { reconcileAccountApi, type AccountSummary } from '@/lib/api';
import {
  formatMinorCurrency,
  formatSignedMinor,
  minorToDecimalInput,
  parseDecimalToMinorClient,
} from '@/lib/format';

export function ReconcileAccountDialog({
  open,
  onOpenChange,
  account,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  account: AccountSummary | null;
}) {
  const queryClient = useQueryClient();
  const [actualBalance, setActualBalance] = useState('');
  const [note, setNote] = useState('');
  const [formError, setFormError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setActualBalance('');
    setNote('');
    setFormError(null);
  }, [open, account]);

  const adjustmentPreviewMinor = useMemo(() => {
    if (!account || !actualBalance.trim() || account.computed_balance_minor === null) {
      return null;
    }
    try {
      const actualMinor = parseDecimalToMinorClient(actualBalance, account.currency);
      return actualMinor - account.computed_balance_minor;
    } catch {
      return null;
    }
  }, [account, actualBalance]);

  const reconcileMutation = useMutation({
    mutationFn: reconcileAccountApi,
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: ['accounts'] });
      void queryClient.invalidateQueries({
        queryKey: ['accounts', 'reconciliations', variables.account_id],
      });
      onOpenChange(false);
    },
    onError: (err) => {
      setFormError(
        err instanceof Error ? err.message : 'Could not reconcile account.',
      );
    },
  });

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setFormError(null);
    if (!account) return;
    let actualMinor: number;
    try {
      actualMinor = parseDecimalToMinorClient(actualBalance, account.currency);
    } catch (err) {
      setFormError(
        err instanceof Error ? err.message : 'Enter a valid actual balance.',
      );
      return;
    }
    reconcileMutation.mutate({
      account_id: account.id,
      actual_balance_minor: actualMinor,
      note: note.trim() || null,
    });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Reconcile {account?.name ?? 'account'}</DialogTitle>
          <DialogDescription>
            Record the actual balance from your bank or statement to anchor
            future calculations.
          </DialogDescription>
        </DialogHeader>

        {account && (
          <form onSubmit={handleSubmit} className="space-y-4">
            <Card>
              <CardContent className="py-3">
                <p className="text-xs text-muted-foreground">
                  {account.computed_balance_minor === null
                    ? 'Starting balance'
                    : 'Server-computed balance'}
                </p>
                <p className="font-semibold">
                  {account.computed_balance_minor === null
                    ? 'Unknown — enter actual balance to set it'
                    : formatMinorCurrency(
                        account.computed_balance_minor,
                        account.currency,
                      )}
                </p>
              </CardContent>
            </Card>

            <Field>
              <FieldLabel>Actual balance ({account.currency})</FieldLabel>
              <Input
                type="text"
                inputMode="decimal"
                required
                value={actualBalance}
                onChange={(e) => setActualBalance(e.target.value)}
                placeholder={minorToDecimalInput(
                  account.computed_balance_minor ?? 0,
                  account.currency,
                )}
              />
            </Field>

            {adjustmentPreviewMinor !== null && (
              <p className="text-sm">
                <span className="text-muted-foreground">Adjustment: </span>
                <span className="font-medium">
                  {formatSignedMinor(adjustmentPreviewMinor, account.currency)}
                </span>
              </p>
            )}

            <Field>
              <FieldLabel>Note (optional)</FieldLabel>
              <Input
                type="text"
                value={note}
                onChange={(e) => setNote(e.target.value)}
                placeholder="e.g. Monthly statement check"
              />
            </Field>

            {formError && (
              <Alert variant="destructive">
                <AlertDescription>{formError}</AlertDescription>
              </Alert>
            )}

            <DialogFooter>
              <DialogClose render={<Button type="button" variant="outline" />}>
                Cancel
              </DialogClose>
              <Button type="submit" disabled={reconcileMutation.isPending}>
                {reconcileMutation.isPending
                  ? 'Reconciling...'
                  : 'Save reconciliation'}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}
