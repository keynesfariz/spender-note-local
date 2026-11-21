'use client';

import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { Separator } from '@/components/ui/separator';
import type { MonthlySpendingSummary, SpendingGroupRow } from '@/lib/api';
import { formatMinorCurrency } from '@/lib/format';

function BreakdownCard({
  title,
  emptyText,
  rows,
  currency,
}: {
  title: string;
  emptyText: string;
  rows: SpendingGroupRow[];
  currency: string;
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardContent>
        {rows.length === 0 ? (
          <p className="text-sm text-muted-foreground">{emptyText}</p>
        ) : (
          <ul className="space-y-2">
            {rows.map((row, idx) => (
              <li key={row.key}>
                {idx > 0 && <Separator className="mb-2" />}
                <div className="flex items-center justify-between gap-3 text-sm">
                  <div className="min-w-0">
                    <p className="truncate font-medium">{row.label}</p>
                    <p className="text-xs text-muted-foreground">
                      {row.count} {row.count === 1 ? 'tx' : 'txs'}
                    </p>
                  </div>
                  <span className="shrink-0 font-medium tabular-nums">
                    {formatMinorCurrency(row.total_minor, currency)}
                  </span>
                </div>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

export function BreakdownPanels({
  summary,
}: {
  summary: MonthlySpendingSummary;
}) {
  const currency = summary.currency ?? 'USD';

  return (
    <section aria-label="Spending breakdowns" className="grid gap-4 md:grid-cols-3">
      <BreakdownCard
        title="By Category"
        emptyText="No local-currency spending by category yet."
        rows={summary.byCategory}
        currency={currency}
      />
      <BreakdownCard
        title="Top Merchants"
        emptyText="No local-currency merchants yet."
        rows={summary.byMerchant}
        currency={currency}
      />
      <BreakdownCard
        title="By Account"
        emptyText="No local-currency spending by account yet."
        rows={summary.byAccount}
        currency={currency}
      />
    </section>
  );
}
