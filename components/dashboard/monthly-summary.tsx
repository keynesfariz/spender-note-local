"use client"

import { Badge } from "@/components/ui/badge"
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import type { MonthlySpendingSummary } from "@/lib/api/types.generated"
import { formatMinorCurrency } from "@/lib/format"

export function MonthlySummary({
  summary,
}: {
  summary: MonthlySpendingSummary
}) {
  const currency = summary.currency ?? "USD"

  return (
    <Card className="bg-summary ring-transparent">
      <CardHeader>
        <CardDescription>Net spending ({summary.month})</CardDescription>
        <CardTitle className="text-3xl font-semibold text-primary tabular-nums">
          {formatMinorCurrency(summary.netSpentMinor, currency)}
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
        <dl className="mb-4 flex flex-wrap gap-x-8 gap-y-3">
          <div>
            <dt className="text-sm text-muted-foreground">Expenses</dt>
            <dd className="font-medium tabular-nums">
              {formatMinorCurrency(summary.totalSpentMinor, currency)}
            </dd>
          </div>
          <div>
            <dt className="text-sm text-muted-foreground">Income</dt>
            <dd className="font-medium tabular-nums">
              {formatMinorCurrency(summary.incomeMinor, currency)}
            </dd>
          </div>
        </dl>
        <p className="text-sm text-muted-foreground">
          {summary.transactionCount}{" "}
          {summary.transactionCount === 1 ? "transaction" : "transactions"}
        </p>
        <dl className="mt-4 border-t pt-4">
          <div>
            <dt className="text-sm text-muted-foreground">
              Card payments for the full month
            </dt>
            <dd className="font-medium tabular-nums">
              {formatMinorCurrency(summary.cardPaymentsMinor, currency)}
            </dd>
          </div>
          {summary.foreignCardPaymentCount > 0 && (
            <div className="mt-2">
              <dt className="sr-only">Foreign-currency card payments</dt>
              <dd>
                <Badge variant="secondary">
                  {summary.foreignCardPaymentCount} foreign-currency (uncounted)
                </Badge>
              </dd>
            </div>
          )}
        </dl>
        <p className="mt-2 text-sm text-muted-foreground">
          Card payments may cover purchases from earlier periods.
        </p>
      </CardContent>
    </Card>
  )
}
