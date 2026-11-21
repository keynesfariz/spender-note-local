'use client';

import { useQuery } from '@tanstack/react-query';
import Link from 'next/link';
import { useState } from 'react';

import { BreakdownPanels } from '@/components/breakdown-panels';
import { MonthlySummary } from '@/components/monthly-summary';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button, buttonVariants } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { monthlySummaryQuery } from '@/lib/api';
import { shiftYearMonth } from '@/lib/format';

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

export function DashboardView() {
  const [selectedMonth, setSelectedMonth] = useState<string | undefined>(
    undefined,
  );

  const summaryQuery = useQuery(monthlySummaryQuery(selectedMonth));

  const activeMonth = selectedMonth ?? summaryQuery.data?.month ?? '';
  const activeParts = parseYearMonth(activeMonth);
  const yearOptions = buildYearOptions(activeParts.year);
  const isConfigured = Boolean(summaryQuery.data?.configured);

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
            onClick={() => setSelectedMonth(shiftYearMonth(activeMonth, -1))}
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
            onClick={() => setSelectedMonth(shiftYearMonth(activeMonth, 1))}
          >
            Next
          </Button>

          <Link
            href="/transactions"
            className={buttonVariants({ variant: 'outline', size: 'sm' })}
          >
            View transactions
          </Link>
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
        <>
          <MonthlySummary summary={summaryQuery.data} />
          <BreakdownPanels summary={summaryQuery.data} />
        </>
      ) : (
        <Alert>
          <AlertTitle>Finish initial setup</AlertTitle>
          <AlertDescription className="mt-2 flex flex-wrap items-center justify-between gap-3">
            <span>
              Configure your country and currency in Settings to start tracking
              monthly spending.
            </span>
            <Link href="/settings" className={buttonVariants({ size: 'sm' })}>
              Go to Settings
            </Link>
          </AlertDescription>
        </Alert>
      )}
    </div>
  );
}
