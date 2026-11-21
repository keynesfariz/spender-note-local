'use client';

import { useQuery } from '@tanstack/react-query';
import Link from 'next/link';
import { useState } from 'react';

import { AddTransactionDialog } from '@/components/add-transaction-dialog';
import { BreakdownPanels } from '@/components/breakdown-panels';
import { MonthlySummary } from '@/components/monthly-summary';
import { TransactionInspectorDialog } from '@/components/transaction-inspector-dialog';
import { TransactionList } from '@/components/transaction-list';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import {
  accountsQuery,
  monthlySummaryQuery,
  settingsQuery,
  transactionsQuery,
  type AccountSummary,
  type MonthlySpendingSummary,
} from '@/lib/api';

const MONTH_OPTIONS = [
  { value: '01', label: 'January' },
  { value: '02', label: 'February' },
  { value: '03', label: 'March' },
  { value: '04', label: 'April' },
  { value: '05', label: 'May' },
  { value: '06', label: 'June' },
  { value: '07', label: 'July' },
  { value: '08', label: 'August' },
  { value: '09', label: 'September' },
  { value: '10', label: 'October' },
  { value: '11', label: 'November' },
  { value: '12', label: 'December' },
];

function shiftMonth(yearMonth: string, delta: number): string {
  const match = /^(\d{4})-(\d{2})$/.exec(yearMonth.trim());
  if (!match) return yearMonth;
  const date = new Date(Date.UTC(Number(match[1]), Number(match[2]) - 1 + delta, 1));
  return `${date.getUTCFullYear()}-${String(date.getUTCMonth() + 1).padStart(2, '0')}`;
}

function parseYearMonth(yearMonth: string): { year: string; month: string } {
  const match = /^(\d{4})-(\d{2})$/.exec(yearMonth.trim());
  if (!match) return { year: '', month: '' };
  return { year: match[1], month: match[2] };
}

function buildYearOptions(activeYear: string): string[] {
  const center = /^\d{4}$/.test(activeYear)
    ? Number(activeYear)
    : new Date().getUTCFullYear();
  const years: string[] = [];
  for (let y = center - 3; y <= center + 1; y += 1) {
    years.push(String(y));
  }
  return years;
}

function ConfiguredDashboard({
  summary,
  accounts,
  month,
  timezone,
  onSelectTransaction,
}: {
  summary: MonthlySpendingSummary;
  accounts: AccountSummary[];
  month: string;
  timezone: string | null | undefined;
  onSelectTransaction: (id: string) => void;
}) {
  const [search, setSearch] = useState('');
  const [category, setCategory] = useState('all');
  const [accountId, setAccountId] = useState('all');
  const [unmergedOnly, setUnmergedOnly] = useState(false);

  const txListQuery = useQuery(
    transactionsQuery({
      month: month || undefined,
      category: category === 'all' ? undefined : category,
      accountId: accountId === 'all' ? undefined : accountId,
      search: search.trim() || undefined,
      unmergedOnly: unmergedOnly || undefined,
    }),
  );

  return (
    <>
      <MonthlySummary
        summary={summary}
        unmergedOnly={unmergedOnly}
        onQuickFilterUnmerged={() => setUnmergedOnly((prev) => !prev)}
      />
      <BreakdownPanels summary={summary} />
      <TransactionList
        transactions={txListQuery.data ?? []}
        accounts={accounts}
        timezone={timezone}
        search={search}
        onSearchChange={setSearch}
        category={category}
        onCategoryChange={setCategory}
        accountId={accountId}
        onAccountIdChange={setAccountId}
        unmergedOnly={unmergedOnly}
        onUnmergedOnlyChange={setUnmergedOnly}
        onSelectTransaction={onSelectTransaction}
      />
    </>
  );
}

export function DashboardView() {
  const [selectedMonth, setSelectedMonth] = useState<string | undefined>(undefined);
  const [selectedTransactionId, setSelectedTransactionId] = useState<string | null>(
    null,
  );
  const [addDialogOpen, setAddDialogOpen] = useState(false);

  const summaryQuery = useQuery(monthlySummaryQuery(selectedMonth));
  const accountsListQuery = useQuery(accountsQuery());
  const settingsDataQuery = useQuery(settingsQuery());

  const activeMonth = selectedMonth ?? summaryQuery.data?.month ?? '';
  const activeParts = parseYearMonth(activeMonth);
  const yearOptions = buildYearOptions(activeParts.year);
  const isConfigured = Boolean(summaryQuery.data?.configured);
  const localCurrency =
    summaryQuery.data?.currency ?? settingsDataQuery.data?.currency ?? 'USD';
  const timezone =
    summaryQuery.data?.timezone ?? settingsDataQuery.data?.timezone;

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Monthly Spending
          </h1>
          <p className="text-sm text-muted-foreground">
            Track spending across synced emails and manual entries.
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!isConfigured || !activeMonth}
            onClick={() => setSelectedMonth(shiftMonth(activeMonth, -1))}
          >
            Prev
          </Button>

          <Select
            value={activeParts.month || ''}
            disabled={!isConfigured}
            onValueChange={(nextMonth) => {
              if (nextMonth && activeParts.year) {
                setSelectedMonth(`${activeParts.year}-${nextMonth}`);
              }
            }}
          >
            <SelectTrigger className="w-36" aria-label="Select month">
              <SelectValue placeholder="Month" />
            </SelectTrigger>
            <SelectContent>
              {MONTH_OPTIONS.map((opt) => (
                <SelectItem key={opt.value} value={opt.value}>
                  {opt.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Select
            value={activeParts.year || ''}
            disabled={!isConfigured}
            onValueChange={(nextYear) => {
              if (nextYear && activeParts.month) {
                setSelectedMonth(`${nextYear}-${activeParts.month}`);
              }
            }}
          >
            <SelectTrigger className="w-24" aria-label="Select year">
              <SelectValue placeholder="Year" />
            </SelectTrigger>
            <SelectContent>
              {yearOptions.map((yr) => (
                <SelectItem key={yr} value={yr}>
                  {yr}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!isConfigured || !activeMonth}
            onClick={() => setSelectedMonth(shiftMonth(activeMonth, 1))}
          >
            Next
          </Button>

          <Button
            type="button"
            disabled={!isConfigured}
            onClick={() => setAddDialogOpen(true)}
          >
            Add transaction
          </Button>
        </div>
      </div>

      {summaryQuery.isPending ? (
        <div className="space-y-4">
          <Skeleton className="h-32 w-full" />
          <div className="grid gap-4 md:grid-cols-3">
            <Skeleton className="h-40 w-full" />
            <Skeleton className="h-40 w-full" />
            <Skeleton className="h-40 w-full" />
          </div>
        </div>
      ) : summaryQuery.data?.configured ? (
        <ConfiguredDashboard
          summary={summaryQuery.data}
          accounts={accountsListQuery.data ?? []}
          month={selectedMonth ?? summaryQuery.data.month ?? ''}
          timezone={timezone}
          onSelectTransaction={setSelectedTransactionId}
        />
      ) : (
        <Alert>
          <AlertTitle>Finish initial setup</AlertTitle>
          <AlertDescription className="mt-2 flex flex-wrap items-center justify-between gap-3">
            <span>
              Configure your country and currency in Settings to start tracking
              monthly spending.
            </span>
            <Button size="sm" render={<Link href="/settings" />}>
              Go to Settings
            </Button>
          </AlertDescription>
        </Alert>
      )}

      <TransactionInspectorDialog
        transactionId={selectedTransactionId}
        accounts={accountsListQuery.data ?? []}
        localCurrency={localCurrency}
        timezone={timezone}
        onClose={() => setSelectedTransactionId(null)}
        onSelectTransaction={setSelectedTransactionId}
      />

      <AddTransactionDialog
        open={addDialogOpen}
        onOpenChange={setAddDialogOpen}
        accounts={accountsListQuery.data ?? []}
        currency={localCurrency}
        timezone={timezone}
      />
    </div>
  );
}
