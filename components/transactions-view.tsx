'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { CheckCircle2Icon, ChevronLeftIcon, ChevronRightIcon } from 'lucide-react';
import Link from 'next/link';
import { useEffect, useMemo, useState } from 'react';

import { AddTransactionDialog } from '@/components/add-transaction-dialog';
import { DateRangePicker } from '@/components/date-picker';
import {
  ManualMergeDialog,
  type ManualMergeMode,
} from '@/components/manual-merge-dialog';
import { TransactionInspectorDialog } from '@/components/transaction-inspector-dialog';
import { TransactionList } from '@/components/transaction-list';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button, buttonVariants } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import {
  accountsQuery,
  markTransactionsReviewedApi,
  settingsQuery,
  transactionsQuery,
} from '@/lib/api';
import { getTodayDateString, shiftDateRange } from '@/lib/format';

const DEFAULT_PAGE_SIZE = 25;

export function TransactionsView() {
  const queryClient = useQueryClient();
  const settingsDataQuery = useQuery(settingsQuery());
  const accountsListQuery = useQuery(accountsQuery());

  const timezone = settingsDataQuery.data?.timezone;
  const currency = settingsDataQuery.data?.currency ?? 'USD';
  const isConfigured = Boolean(
    settingsDataQuery.data?.timezone && settingsDataQuery.data?.currency,
  );

  const [dateRange, setDateRange] = useState<{ from: string; to: string } | null>(
    null,
  );
  const [search, setSearch] = useState('');
  const [category, setCategory] = useState('all');
  const [accountId, setAccountId] = useState('all');
  const [unreviewedOnly, setUnreviewedOnly] = useState(false);
  const [page, setPage] = useState(1);
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [selectedTransactionId, setSelectedTransactionId] = useState<string | null>(
    null,
  );
  const [mergeMode, setMergeMode] = useState<ManualMergeMode | null>(null);
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [reviewError, setReviewError] = useState<string | null>(null);

  const todayStr = useMemo(() => getTodayDateString(timezone), [timezone]);
  const activeRange = dateRange ?? { from: todayStr, to: todayStr };

  const txQuery = useQuery({
    ...transactionsQuery({
      startDate: activeRange.from,
      endDate: activeRange.to,
      category: category === 'all' ? undefined : category,
      accountId: accountId === 'all' ? undefined : accountId,
      search: search.trim() || undefined,
      unreviewedOnly: unreviewedOnly || undefined,
      page,
      pageSize: DEFAULT_PAGE_SIZE,
    }),
    enabled: isConfigured,
  });

  const items = useMemo(() => txQuery.data?.items ?? [], [txQuery.data?.items]);
  const totalCount = txQuery.data?.totalCount ?? 0;
  const totalInRangeCount = txQuery.data?.totalInRangeCount ?? 0;
  const unreviewedInRangeCount = txQuery.data?.unreviewedInRangeCount ?? 0;
  const totalPages = Math.max(1, Math.ceil(totalCount / DEFAULT_PAGE_SIZE));

  // Clamp page if items shrink after filtering, merging, or reviewing.
  useEffect(() => {
    if (txQuery.isSuccess && page > totalPages) {
      setPage(totalPages);
      setSelectedIds([]);
    }
  }, [txQuery.isSuccess, page, totalPages]);

  // Prune stale selected IDs if background sync shifts page contents.
  useEffect(() => {
    const visibleIds = new Set(items.map((tx) => tx.id));
    setSelectedIds((prev) => {
      const next = prev.filter((id) => visibleIds.has(id));
      return next.length === prev.length ? prev : next;
    });
  }, [items]);

  const markReviewedMutation = useMutation({
    mutationFn: markTransactionsReviewedApi,
    onMutate: () => setReviewError(null),
    onSuccess: () => {
      setSelectedIds([]);
      void queryClient.invalidateQueries({ queryKey: ['transactions'] });
      void queryClient.invalidateQueries({ queryKey: ['spending'] });
    },
    onError: (err) => {
      setReviewError(
        err instanceof Error ? err.message : 'Failed to mark transactions reviewed.',
      );
    },
  });

  const updateRange = (next: { from: string; to: string }) => {
    setDateRange(next);
    setPage(1);
    setSelectedIds([]);
  };

  const handleShiftRange = (direction: -1 | 1) => {
    updateRange(shiftDateRange(activeRange.from, activeRange.to, direction));
  };

  const handleSearchChange = (value: string) => {
    setSearch(value);
    setPage(1);
    setSelectedIds([]);
  };

  const handleCategoryChange = (value: string) => {
    setCategory(value);
    setPage(1);
    setSelectedIds([]);
  };

  const handleAccountIdChange = (value: string) => {
    setAccountId(value);
    setPage(1);
    setSelectedIds([]);
  };

  const handleUnreviewedOnlyChange = (value: boolean) => {
    setUnreviewedOnly(value);
    setPage(1);
    setSelectedIds([]);
  };

  const handlePageChange = (nextPage: number) => {
    setPage(nextPage);
    setSelectedIds([]);
  };

  const handleToggleRow = (id: string, checked: boolean) => {
    setSelectedIds((prev) => {
      if (checked) {
        return prev.includes(id) ? prev : [...prev, id];
      }
      return prev.filter((existing) => existing !== id);
    });
  };

  const handleToggleSelectAllPage = (checked: boolean) => {
    if (checked) {
      setSelectedIds(items.map((tx) => tx.id));
    } else {
      setSelectedIds([]);
    }
  };

  const selectedTransactions = useMemo(() => {
    const selectedSet = new Set(selectedIds);
    return items.filter((tx) => selectedSet.has(tx.id));
  }, [items, selectedIds]);

  const allInRangeReviewed =
    !txQuery.isLoading && totalInRangeCount > 0 && unreviewedInRangeCount === 0;
  const allOnPageReviewedButMoreUnreviewed =
    !txQuery.isLoading &&
    items.length > 0 &&
    items.every((tx) => tx.reviewed === 1) &&
    unreviewedInRangeCount > 0;

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Transactions</h1>
          <p className="text-sm text-muted-foreground">
            Review daily spending, merge duplicate emails, or combine related
            charges.
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!isConfigured}
            onClick={() => handleShiftRange(-1)}
          >
            <ChevronLeftIcon className="mr-1 size-4" />
            Previous
          </Button>

          <DateRangePicker
            from={activeRange.from}
            to={activeRange.to}
            disabled={!isConfigured}
            onChange={updateRange}
          />

          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!isConfigured}
            onClick={() => handleShiftRange(1)}
          >
            Next
            <ChevronRightIcon className="ml-1 size-4" />
          </Button>

          <Button
            type="button"
            size="sm"
            disabled={!isConfigured}
            onClick={() => setAddDialogOpen(true)}
          >
            Add transaction
          </Button>
        </div>
      </div>

      {settingsDataQuery.isPending ? (
        <div className="space-y-4">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-64 w-full" />
        </div>
      ) : !isConfigured ? (
        <Alert>
          <AlertTitle>Finish initial setup</AlertTitle>
          <AlertDescription className="mt-2 flex flex-wrap items-center justify-between gap-3">
            <span>
              Configure your country and currency in Settings to review and
              manage transactions.
            </span>
            <Link href="/settings" className={buttonVariants({ size: 'sm' })}>
              Go to Settings
            </Link>
          </AlertDescription>
        </Alert>
      ) : (
        <>
          {allInRangeReviewed && (
            <Alert>
              <CheckCircle2Icon className="size-4" />
              <AlertTitle>All transactions reviewed</AlertTitle>
              <AlertDescription className="mt-1 flex flex-wrap items-center justify-between gap-3">
                <span>
                  Every transaction in this date range has been reviewed. You
                  can skip to another day.
                </span>
                <div className="flex items-center gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => handleShiftRange(-1)}
                  >
                    {activeRange.from === activeRange.to
                      ? 'Previous day'
                      : 'Previous range'}
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => handleShiftRange(1)}
                  >
                    {activeRange.from === activeRange.to
                      ? 'Next day'
                      : 'Next range'}
                  </Button>
                </div>
              </AlertDescription>
            </Alert>
          )}

          {allOnPageReviewedButMoreUnreviewed && (
            <Alert>
              <CheckCircle2Icon className="size-4" />
              <AlertTitle>This page is all reviewed</AlertTitle>
              <AlertDescription className="mt-1 flex flex-wrap items-center justify-between gap-3">
                <span>
                  All transactions on this page are reviewed ({unreviewedInRangeCount}{' '}
                  unreviewed remaining in this date range).
                </span>
                {page < totalPages && (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => handlePageChange(page + 1)}
                  >
                    Next page
                  </Button>
                )}
              </AlertDescription>
            </Alert>
          )}

          <TransactionList
            transactions={items}
            accounts={accountsListQuery.data ?? []}
            timezone={timezone}
            isLoading={txQuery.isLoading}
            errorMessage={
              txQuery.isError
                ? txQuery.error instanceof Error
                  ? txQuery.error.message
                  : 'Failed to load transactions.'
                : null
            }
            search={search}
            onSearchChange={handleSearchChange}
            category={category}
            onCategoryChange={handleCategoryChange}
            accountId={accountId}
            onAccountIdChange={handleAccountIdChange}
            unreviewedOnly={unreviewedOnly}
            onUnreviewedOnlyChange={handleUnreviewedOnlyChange}
            selectedIds={selectedIds}
            onToggleRow={handleToggleRow}
            onToggleSelectAllPage={handleToggleSelectAllPage}
            onSelectTransaction={setSelectedTransactionId}
            onOpenDuplicateMerge={() => setMergeMode('duplicate')}
            onOpenCombine={() => setMergeMode('combine')}
            onMarkReviewed={(ids) => markReviewedMutation.mutate(ids)}
            isMarkingReviewed={markReviewedMutation.isPending}
            reviewErrorMessage={reviewError}
            page={page}
            pageSize={DEFAULT_PAGE_SIZE}
            totalCount={totalCount}
            onPageChange={handlePageChange}
          />
        </>
      )}

      <ManualMergeDialog
        mode={mergeMode}
        transactions={selectedTransactions}
        accounts={accountsListQuery.data ?? []}
        timezone={timezone}
        onClose={() => setMergeMode(null)}
        onSuccess={() => setSelectedIds([])}
      />

      <TransactionInspectorDialog
        transactionId={selectedTransactionId}
        accounts={accountsListQuery.data ?? []}
        localCurrency={currency}
        timezone={timezone}
        onClose={() => setSelectedTransactionId(null)}
      />

      <AddTransactionDialog
        open={addDialogOpen}
        onOpenChange={setAddDialogOpen}
        accounts={accountsListQuery.data ?? []}
        currency={currency}
        timezone={timezone}
      />
    </div>
  );
}
