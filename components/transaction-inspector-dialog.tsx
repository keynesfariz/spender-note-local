'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useState, type FormEvent } from 'react';

import { Alert, AlertDescription } from '@/components/ui/alert';
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
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Separator } from '@/components/ui/separator';
import { Skeleton } from '@/components/ui/skeleton';
import {
  acceptMergeSuggestionApi,
  deleteTransactionApi,
  rejectMergeSuggestionApi,
  splitTransactionSourceApi,
  transactionDetailQuery,
  updateTransactionApi,
  type AccountSummary,
} from '@/lib/api';
import { DEFAULT_CATEGORIES } from '@/lib/categories';
import {
  dateTimeInputToUtcIso,
  formatMinorCurrency,
  formatUtcInTimezone,
  minorToDecimalInput,
  parseDecimalToMinorClient,
  utcToDateInputValue,
  utcToTimeInputValue,
} from '@/lib/format';

const UNASSIGNED_ACCOUNT = '__unassigned__';

function InspectorBody({
  transactionId,
  accounts,
  localCurrency,
  timezone,
  onClose,
  onSelectTransaction,
}: {
  transactionId: string;
  accounts: AccountSummary[];
  localCurrency: string;
  timezone: string | null | undefined;
  onClose: () => void;
  onSelectTransaction?: (id: string) => void;
}) {
  const queryClient = useQueryClient();
  const detailQuery = useQuery(transactionDetailQuery(transactionId));

  const [merchant, setMerchant] = useState('');
  const [amount, setAmount] = useState('');
  const [category, setCategory] = useState('Other');
  const [accountId, setAccountId] = useState(UNASSIGNED_ACCOUNT);
  const [date, setDate] = useState('');
  const [time, setTime] = useState('');
  const [note, setNote] = useState('');
  const [convertToLocalCurrency, setConvertToLocalCurrency] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [statusMessage, setStatusMessage] = useState<string | null>(null);

  useEffect(() => {
    const tx = detailQuery.data?.transaction;
    if (!tx) return;
    setMerchant(tx.merchant);
    setAmount(minorToDecimalInput(tx.amount_minor, tx.currency));
    setCategory(tx.category);
    setAccountId(tx.account_id ?? UNASSIGNED_ACCOUNT);
    setDate(utcToDateInputValue(tx.occurred_at_utc, timezone));
    setTime(utcToTimeInputValue(tx.occurred_at_utc, timezone));
    setNote(tx.note ?? '');
    setConvertToLocalCurrency(false);
  }, [detailQuery.data?.transaction, timezone]);

  const invalidateAll = () => {
    void queryClient.invalidateQueries({ queryKey: ['spending'] });
    void queryClient.invalidateQueries({ queryKey: ['transactions'] });
    void queryClient.invalidateQueries({ queryKey: ['accounts'] });
  };

  const updateMutation = useMutation({
    mutationFn: updateTransactionApi,
    onSuccess: () => {
      setFormError(null);
      setStatusMessage('Transaction updated.');
      invalidateAll();
    },
    onError: (err) => {
      setFormError(
        err instanceof Error ? err.message : 'Failed to update transaction.',
      );
    },
  });

  const deleteMutation = useMutation({
    mutationFn: deleteTransactionApi,
    onSuccess: () => {
      invalidateAll();
      onClose();
    },
    onError: (err) => {
      setFormError(
        err instanceof Error ? err.message : 'Failed to delete transaction.',
      );
    },
  });

  const splitMutation = useMutation({
    mutationFn: splitTransactionSourceApi,
    onSuccess: () => {
      setFormError(null);
      setStatusMessage('Email split into its own transaction.');
      invalidateAll();
    },
    onError: (err) => {
      setFormError(
        err instanceof Error ? err.message : 'Failed to split email source.',
      );
    },
  });

  const acceptMergeMutation = useMutation({
    mutationFn: acceptMergeSuggestionApi,
    onSuccess: (survivorId) => {
      setFormError(null);
      setStatusMessage('Transactions merged.');
      if (survivorId && survivorId !== transactionId && onSelectTransaction) {
        onSelectTransaction(survivorId);
      }
      invalidateAll();
    },
    onError: (err) => {
      setFormError(
        err instanceof Error ? err.message : 'Failed to accept merge.',
      );
    },
  });

  const rejectMergeMutation = useMutation({
    mutationFn: rejectMergeSuggestionApi,
    onSuccess: () => {
      setFormError(null);
      setStatusMessage('Merge suggestion dismissed.');
      invalidateAll();
    },
    onError: (err) => {
      setFormError(
        err instanceof Error ? err.message : 'Failed to dismiss suggestion.',
      );
    },
  });

  if (detailQuery.isPending) {
    return (
      <div className="space-y-3 py-4">
        <Skeleton className="h-9 w-full" />
        <Skeleton className="h-9 w-full" />
        <Skeleton className="h-24 w-full" />
      </div>
    );
  }

  const detail = detailQuery.data;
  if (!detail) {
    return (
      <Alert variant="destructive">
        <AlertDescription>
          {detailQuery.error instanceof Error
            ? detailQuery.error.message
            : 'Transaction not found.'}
        </AlertDescription>
      </Alert>
    );
  }

  const tx = detail.transaction;
  const pendingSuggestions = detail.mergeSuggestions.filter(
    (s) => s.status === 'pending',
  );

  function handleSave(event: FormEvent) {
    event.preventDefault();
    setFormError(null);
    setStatusMessage(null);
    try {
      const targetCurrency =
        tx.isForeignCurrency && !convertToLocalCurrency
          ? tx.currency
          : localCurrency;
      const initialAmountStr = minorToDecimalInput(tx.amount_minor, tx.currency);
      const amountChanged = amount.trim() !== initialAmountStr;
      const parsedAmountMinor =
        !tx.isForeignCurrency || convertToLocalCurrency || amountChanged
          ? parseDecimalToMinorClient(amount, targetCurrency)
          : undefined;

      const initialDateStr = utcToDateInputValue(tx.occurred_at_utc, timezone);
      const initialTimeStr = utcToTimeInputValue(tx.occurred_at_utc, timezone);
      const occurredAtUtc =
        date.trim() !== initialDateStr || time.trim() !== initialTimeStr
          ? dateTimeInputToUtcIso(date, time || '00:00', timezone)
          : undefined;

      updateMutation.mutate({
        id: tx.id,
        merchant: merchant.trim(),
        amount_minor: parsedAmountMinor,
        category,
        account_id: accountId !== UNASSIGNED_ACCOUNT ? accountId : null,
        occurred_at_utc: occurredAtUtc,
        note: note.trim() || null,
        convertToLocalCurrency: tx.isForeignCurrency
          ? convertToLocalCurrency
          : undefined,
      });
    } catch (err) {
      setFormError(err instanceof Error ? err.message : 'Invalid input.');
    }
  }

  return (
    <div className="space-y-6">
      {formError && (
        <Alert variant="destructive">
          <AlertDescription>{formError}</AlertDescription>
        </Alert>
      )}
      {statusMessage && (
        <Alert>
          <AlertDescription>{statusMessage}</AlertDescription>
        </Alert>
      )}

      <form onSubmit={handleSave} className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <Field>
            <FieldLabel>Merchant</FieldLabel>
            <Input
              type="text"
              required
              value={merchant}
              onChange={(e) => setMerchant(e.target.value)}
            />
          </Field>

          <Field>
            <FieldLabel>
              Amount (
              {tx.isForeignCurrency && !convertToLocalCurrency
                ? tx.currency
                : localCurrency}
              )
            </FieldLabel>
            <Input
              type="text"
              inputMode="decimal"
              required
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
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
                <SelectItem value={UNASSIGNED_ACCOUNT}>Unassigned</SelectItem>
                {accounts.map((acct) => (
                  <SelectItem key={acct.id} value={acct.id}>
                    {acct.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {tx.payment_method_raw && (
              <FieldDescription>
                Extracted payment method: {tx.payment_method_raw}
              </FieldDescription>
            )}
          </Field>

          <Field>
            <FieldLabel>Occurred date</FieldLabel>
            <Input
              type="date"
              required
              value={date}
              onChange={(e) => setDate(e.target.value)}
            />
          </Field>

          <Field>
            <FieldLabel>Occurred time</FieldLabel>
            <Input
              type="time"
              required
              value={time}
              onChange={(e) => setTime(e.target.value)}
            />
          </Field>

          <Field className="sm:col-span-2">
            <FieldLabel>Note</FieldLabel>
            <Input
              type="text"
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />
          </Field>
        </div>

        {tx.isForeignCurrency && (
          <div className="flex items-center gap-2">
            <Checkbox
              id="convert-local"
              checked={convertToLocalCurrency}
              onCheckedChange={(checked) =>
                setConvertToLocalCurrency(Boolean(checked))
              }
            />
            <Label htmlFor="convert-local">
              Convert to local currency ({localCurrency})
            </Label>
          </div>
        )}

        <div className="flex flex-wrap items-center justify-between gap-3 pt-2">
          <AlertDialog>
            <AlertDialogTrigger
              render={<Button type="button" variant="destructive" size="sm" />}
            >
              Delete transaction
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>Delete this transaction?</AlertDialogTitle>
                <AlertDialogDescription>
                  This permanently deletes the transaction and marks any linked
                  source emails as dismissed.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction
                  variant="destructive"
                  disabled={deleteMutation.isPending}
                  onClick={() => deleteMutation.mutate(tx.id)}
                >
                  Delete forever
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>

          <Button type="submit" disabled={updateMutation.isPending}>
            {updateMutation.isPending ? 'Saving...' : 'Save changes'}
          </Button>
        </div>
      </form>

      {pendingSuggestions.length > 0 && (
        <>
          <Separator />
          <div className="space-y-3">
            <h3 className="text-sm font-semibold">Pending merge suggestions</h3>
            <ul className="space-y-2">
              {pendingSuggestions.map((sug) => (
                <li key={sug.id}>
                  <Card>
                    <CardContent className="flex flex-wrap items-center justify-between gap-3 py-3 text-sm">
                      <div>
                        <p className="font-medium">
                          {sug.other_merchant} ·{' '}
                          {formatMinorCurrency(
                            sug.other_amount_minor,
                            sug.other_currency,
                          )}
                        </p>
                        <p className="text-xs text-muted-foreground">
                          {formatUtcInTimezone(sug.other_occurred_at_utc, timezone)}{' '}
                          · Score {Math.round(sug.score * 100)}%
                        </p>
                      </div>
                      <div className="flex items-center gap-2">
                        <Button
                          type="button"
                          size="sm"
                          disabled={acceptMergeMutation.isPending}
                          onClick={() => acceptMergeMutation.mutate(sug.id)}
                        >
                          Accept merge
                        </Button>
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          disabled={rejectMergeMutation.isPending}
                          onClick={() => rejectMergeMutation.mutate(sug.id)}
                        >
                          Dismiss
                        </Button>
                      </div>
                    </CardContent>
                  </Card>
                </li>
              ))}
            </ul>
          </div>
        </>
      )}

      <Separator />
      <div className="space-y-3">
        <h3 className="text-sm font-semibold">
          Attached cleaned emails ({detail.sources.length})
        </h3>
        {detail.sources.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No source emails attached (manual transaction).
          </p>
        ) : (
          <ul className="space-y-3">
            {detail.sources.map((source) => (
              <li key={source.id}>
                <Card>
                  <CardContent className="space-y-2 py-3 text-sm">
                    <div className="flex flex-wrap items-start justify-between gap-2">
                      <div className="min-w-0">
                        <p className="font-medium">{source.subject}</p>
                        <p className="text-xs text-muted-foreground">
                          From {source.from_address} ·{' '}
                          {formatUtcInTimezone(source.received_at_utc, timezone)} ·
                          role: {source.role} · state: {source.match_state}
                        </p>
                      </div>
                      {detail.sources.length > 1 && (
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          disabled={splitMutation.isPending}
                          onClick={() =>
                            splitMutation.mutate({
                              transactionId: tx.id,
                              emailId: source.email_id,
                            })
                          }
                        >
                          Split email
                        </Button>
                      )}
                    </div>
                    <pre className="max-h-48 overflow-y-auto rounded-md border bg-muted p-2.5 font-mono text-xs whitespace-pre-wrap">
                      {source.cleaned_text}
                    </pre>
                  </CardContent>
                </Card>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

export function TransactionInspectorDialog({
  transactionId,
  accounts,
  localCurrency,
  timezone,
  onClose,
  onSelectTransaction,
}: {
  transactionId: string | null;
  accounts: AccountSummary[];
  localCurrency: string;
  timezone: string | null | undefined;
  onClose: () => void;
  onSelectTransaction?: (id: string) => void;
}) {
  return (
    <Dialog
      open={transactionId !== null}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Transaction details</DialogTitle>
          <DialogDescription>
            Inspect extracted source emails, resolve merge suggestions, or edit
            transaction fields.
          </DialogDescription>
        </DialogHeader>
        {transactionId && (
          <InspectorBody
            transactionId={transactionId}
            accounts={accounts}
            localCurrency={localCurrency}
            timezone={timezone}
            onClose={onClose}
            onSelectTransaction={onSelectTransaction}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
