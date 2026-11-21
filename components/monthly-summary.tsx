'use client';

import { Badge } from '@/components/ui/badge';
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
}: {
  summary: MonthlySpendingSummary;
}) {
  const currency = summary.currency ?? 'USD';

  return (
    <Card>
      <CardHeader>
        <CardDescription>Total spent ({summary.month})</CardDescription>
        <CardTitle className="text-3xl tabular-nums">
          {formatMinorCurrency(summary.totalSpentMinor, currency)}
        </CardTitle>
        {summary.foreignCurrencyCount > 0 && (
          <CardAction className="flex flex-wrap items-center gap-2">
            <Badge variant="secondary">
              {summary.foreignCurrencyCount} foreign-currency (uncounted)
            </Badge>
          </CardAction>
        )}
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
