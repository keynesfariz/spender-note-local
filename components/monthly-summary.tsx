'use client';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import type { MonthlySpendingSummary } from '@/lib/api';
import { formatMinorCurrency } from '@/lib/format';

export function MonthlySummary({
  summary,
  unmergedOnly,
  onQuickFilterUnmerged,
}: {
  summary: MonthlySpendingSummary;
  unmergedOnly: boolean;
  onQuickFilterUnmerged: () => void;
}) {
  const currency = summary.currency ?? 'USD';

  return (
    <Card>
      <CardHeader>
        <CardDescription>Total spent ({summary.month})</CardDescription>
        <CardTitle className="text-3xl tabular-nums">
          {formatMinorCurrency(summary.totalSpentMinor, currency)}
        </CardTitle>
        <CardAction className="flex flex-wrap items-center gap-2">
          {summary.foreignCurrencyCount > 0 && (
            <Badge variant="secondary">
              {summary.foreignCurrencyCount} foreign-currency (uncounted)
            </Badge>
          )}
          {summary.pendingMergeSuggestionsCount > 0 && (
            <Button
              type="button"
              variant={unmergedOnly ? 'default' : 'outline'}
              size="sm"
              aria-pressed={unmergedOnly}
              onClick={onQuickFilterUnmerged}
            >
              {summary.pendingMergeSuggestionsCount} pending merge{' '}
              {summary.pendingMergeSuggestionsCount === 1
                ? 'suggestion'
                : 'suggestions'}
            </Button>
          )}
        </CardAction>
      </CardHeader>
      <CardContent>
        <p className="text-sm text-muted-foreground">
          {summary.transactionCount}{' '}
          {summary.transactionCount === 1 ? 'transaction' : 'transactions'}
        </p>
      </CardContent>
    </Card>
  );
}
