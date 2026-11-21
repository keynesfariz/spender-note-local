'use client';

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useEffect, useMemo, useState, type FormEvent } from 'react';

import { DatePicker } from '@/components/date-picker';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
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
import { Field, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  combineTransactionsApi,
  duplicateMergeTransactionsApi,
  type AccountSummary,
  type TransactionListItem,
} from '@/lib/api';
import { DEFAULT_CATEGORIES } from '@/lib/categories';
import {
  dateTimeInputToUtcIso,
  formatMinorCurrency,
  formatUtcInTimezone,
  utcToDateInputValue,
  utcToTimeInputValue,
} from '@/lib/format';
import { cn } from '@/lib/utils';

const UNASSIGNED_ACCOUNT = '__unassigned__';

export type ManualMergeMode = 'duplicate' | 'combine';

export function ManualMergeDialog({
  mode,
  transactions,
  accounts,
  timezone,
  onClose,
  onSuccess,
}: {
  mode: ManualMergeMode | null;
  transactions: TransactionListItem[];
  accounts: AccountSummary[];
  timezone: string | null | undefined;
  onClose: () => void;
  onSuccess: () => void;
}) {
  const queryClient = useQueryClient();

  const [keepId, setKeepId] = useState<string>('');
  const [merchant, setMerchant] = useState('');
  const [category, setCategory] = useState('Other');
  const [accountId, setAccountId] = useState(UNASSIGNED_ACCOUNT);
  const [date, setDate] = useState('');
  const [initialDate, setInitialDate] = useState('');
  const [initialTime, setInitialTime] = useState('00:00');
  const [note, setNote] = useState('');
  const [formError, setFormError] = useState<string | null>(null);

  const sortedByOccurred = useMemo(
    () =>
      [...transactions].sort((a, b) =>
        b.occurred_at_utc.localeCompare(a.occurred_at_utc),
      ),
    [transactions],
  );

  const summedAmount = useMemo(
    () => transactions.reduce((sum, tx) => sum + tx.amount_minor, 0),
    [transactions],
  );
  const sharedCurrency = transactions[0]?.currency ?? 'USD';

  useEffect(() => {
    if (!mode || transactions.length < 2) return;
    setFormError(null);
    setKeepId('');

    const primary = sortedByOccurred[0];
    if (primary) {
      const d = utcToDateInputValue(primary.occurred_at_utc, timezone);
      const t = utcToTimeInputValue(primary.occurred_at_utc, timezone) || '00:00';
      setMerchant(primary.merchant);
      setCategory(primary.category);
      setAccountId(primary.account_id ?? UNASSIGNED_ACCOUNT);
      setDate(d);
      setInitialDate(d);
      setInitialTime(t);
      const combinedNotes = sortedByOccurred
        .map((tx) => tx.note?.trim())
        .filter((n): n is string => Boolean(n))
        .join(' / ');
      setNote(combinedNotes);
    }
  }, [mode, transactions, sortedByOccurred, timezone]);

  const invalidateAll = () => {
    void queryClient.invalidateQueries({ queryKey: ['transactions'] });
    void queryClient.invalidateQueries({ queryKey: ['spending'] });
    void queryClient.invalidateQueries({ queryKey: ['accounts'] });
  };

  const duplicateMutation = useMutation({
    mutationFn: duplicateMergeTransactionsApi,
    onSuccess: () => {
      invalidateAll();
      onSuccess();
      onClose();
    },
    onError: (err) => {
      setFormError(
        err instanceof Error ? err.message : 'Failed to merge duplicate transactions.',
      );
    },
  });

  const combineMutation = useMutation({
    mutationFn: combineTransactionsApi,
    onSuccess: () => {
      invalidateAll();
      onSuccess();
      onClose();
    },
    onError: (err) => {
      setFormError(
        err instanceof Error ? err.message : 'Failed to combine transactions.',
      );
    },
  });

  const isPending = duplicateMutation.isPending || combineMutation.isPending;

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    setFormError(null);

    if (mode === 'duplicate') {
      if (transactions.length !== 2 || !keepId) {
        setFormError('Choose which transaction to keep.');
        return;
      }
      const dropTx = transactions.find((tx) => tx.id !== keepId);
      if (!dropTx) {
        setFormError('Could not determine the duplicate transaction to remove.');
        return;
      }
      duplicateMutation.mutate({
        keepTransactionId: keepId,
        dropTransactionId: dropTx.id,
      });
      return;
    }

    if (mode === 'combine') {
      if (!merchant.trim()) {
        setFormError('Merchant name is required.');
        return;
      }
      if (!date.trim()) {
        setFormError('Transaction date is required.');
        return;
      }
      let occurredAtUtc: string;
      try {
        const timeToUse = date === initialDate ? initialTime : '00:00';
        occurredAtUtc = dateTimeInputToUtcIso(date, timeToUse, timezone);
      } catch (err) {
        setFormError(err instanceof Error ? err.message : 'Invalid date.');
        return;
      }
      combineMutation.mutate({
        transactionIds: transactions.map((tx) => tx.id),
        merchant: merchant.trim(),
        category,
        account_id: accountId === UNASSIGNED_ACCOUNT ? null : accountId,
        occurred_at_utc: occurredAtUtc,
        note: note.trim() || null,
      });
    }
  };

  const canSubmit =
    mode === 'duplicate'
      ? Boolean(keepId) && transactions.length === 2 && !isPending
      : Boolean(merchant.trim()) && Boolean(date.trim()) && transactions.length >= 2 && !isPending;

  return (
    <Dialog
      open={mode !== null}
      onOpenChange={(open) => {
        if (!open && !isPending) onClose();
      }}
    >
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {mode === 'duplicate' ? 'Duplicate merge' : 'Combine transactions'}
          </DialogTitle>
          <DialogDescription>
            {mode === 'duplicate'
              ? 'Choose which transaction to keep. The other transaction will be deleted, and all linked emails from both will stay attached.'
              : `Combining ${transactions.length} transactions into one entry with their summed amount and all linked emails preserved.`}
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4">
          {mode === 'duplicate' && (
            <div
              role="radiogroup"
              aria-label="Choose transaction to keep"
              className="space-y-2"
            >
              {transactions.map((tx) => {
                const selected = keepId === tx.id;
                return (
                  <label
                    key={tx.id}
                    className={cn(
                      'flex cursor-pointer items-start gap-3 rounded-lg border p-3 text-sm transition-colors',
                      selected
                        ? 'border-primary bg-primary/5 ring-1 ring-primary'
                        : 'hover:bg-muted/50',
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
                        <span className="truncate font-medium">{tx.merchant}</span>
                        <span className="font-medium tabular-nums">
                          {formatMinorCurrency(tx.amount_minor, tx.currency)}
                        </span>
                      </div>
                      <div className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
                        <Badge variant="secondary">{tx.category}</Badge>
                        <Badge variant="outline">
                          {tx.sourceCount > 0
                            ? `${tx.sourceCount} ${tx.sourceCount === 1 ? 'email' : 'emails'}`
                            : 'Manual'}
                        </Badge>
                        <span>
                          {tx.account_name ?? 'Unassigned'} ·{' '}
                          {formatUtcInTimezone(tx.occurred_at_utc, timezone)}
                        </span>
                      </div>
                      {tx.note && (
                        <p className="text-xs text-muted-foreground">
                          Note: {tx.note}
                        </p>
                      )}
                    </div>
                  </label>
                );
              })}
            </div>
          )}

          {mode === 'combine' && (
            <div className="space-y-4">
              <div className="rounded-lg border bg-muted/40 p-3">
                <p className="text-xs text-muted-foreground">
                  Combined total ({transactions.length} transactions)
                </p>
                <p className="text-2xl font-semibold tabular-nums">
                  {formatMinorCurrency(summedAmount, sharedCurrency)}
                </p>
              </div>

              <div className="grid gap-4 sm:grid-cols-2">
                <Field className="sm:col-span-2">
                  <FieldLabel>Merchant</FieldLabel>
                  <Input
                    type="text"
                    required
                    value={merchant}
                    onChange={(e) => setMerchant(e.target.value)}
                  />
                </Field>

                <Field>
                  <FieldLabel>Category</FieldLabel>
                  <Select
                    value={category}
                    onValueChange={(v) => setCategory(v ?? 'Other')}
                  >
                    <SelectTrigger className="w-full" aria-label="Category">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {DEFAULT_CATEGORIES.map((cat) => (
                        <SelectItem key={cat} value={cat}>
                          {cat}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </Field>

                <Field>
                  <FieldLabel>Account</FieldLabel>
                  <Select
                    value={accountId}
                    onValueChange={(v) => setAccountId(v ?? UNASSIGNED_ACCOUNT)}
                  >
                    <SelectTrigger className="w-full" aria-label="Account">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value={UNASSIGNED_ACCOUNT}>
                        Unassigned
                      </SelectItem>
                      {accounts.map((acct) => (
                        <SelectItem key={acct.id} value={acct.id}>
                          {acct.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </Field>

                <Field className="sm:col-span-2">
                  <FieldLabel>Date</FieldLabel>
                  <DatePicker
                    value={date}
                    onChange={setDate}
                    ariaLabel="Combined transaction date"
                  />
                </Field>

                <Field className="sm:col-span-2">
                  <FieldLabel>Note</FieldLabel>
                  <Input
                    type="text"
                    placeholder="Optional note"
                    value={note}
                    onChange={(e) => setNote(e.target.value)}
                  />
                </Field>
              </div>
            </div>
          )}

          {formError && (
            <Alert variant="destructive">
              <AlertDescription>{formError}</AlertDescription>
            </Alert>
          )}

          <DialogFooter>
            <DialogClose
              render={<Button type="button" variant="outline" disabled={isPending} />}
            >
              Cancel
            </DialogClose>
            <Button type="submit" disabled={!canSubmit}>
              {isPending
                ? mode === 'duplicate'
                  ? 'Merging...'
                  : 'Combining...'
                : mode === 'duplicate'
                  ? 'Merge duplicate'
                  : 'Combine transactions'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
