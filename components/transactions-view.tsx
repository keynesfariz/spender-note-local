"use client"

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  CheckCircle2Icon,
  ChevronLeftIcon,
  ChevronRightIcon,
} from "lucide-react"
import Link from "next/link"
import {
  parseAsBoolean,
  parseAsInteger,
  parseAsString,
  parseAsStringLiteral,
  useQueryStates,
} from "nuqs"
import { useEffect, useMemo, useState } from "react"

import { AddTransactionDialog } from "@/components/add-transaction-dialog"
import { DateRangePicker } from "@/components/date-picker"
import {
  ManualMergeDialog,
  type ManualMergeMode,
} from "@/components/manual-merge-dialog"
import { TransactionInspectorDialog } from "@/components/transaction-inspector-dialog"
import { TransactionList } from "@/components/transaction-list"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button, buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import {
  accountsQuery,
  deleteTransactionsApi,
  markTransactionsReviewedApi,
  settingsQuery,
  transactionsQuery,
} from "@/lib/api"
import { getTodayDateString, shiftDateRange } from "@/lib/format"

const DEFAULT_PAGE_SIZE = 25

export function TransactionsView() {
  const queryClient = useQueryClient()
  const settingsDataQuery = useQuery(settingsQuery())
  const accountsListQuery = useQuery(accountsQuery())

  const timezone = settingsDataQuery.data?.timezone
  const currency = settingsDataQuery.data?.currency ?? "USD"
  const isConfigured = Boolean(
    settingsDataQuery.data?.timezone && settingsDataQuery.data?.currency
  )

  const [filters, setFilters] = useQueryStates({
    from: parseAsString,
    to: parseAsString,
    range: parseAsStringLiteral(["all"] as const),
    search: parseAsString.withDefault(""),
    category: parseAsString.withDefault("all"),
    tag: parseAsString.withDefault("all"),
    accountId: parseAsString.withDefault("all"),
    unreviewed: parseAsBoolean.withDefault(false),
    page: parseAsInteger.withDefault(1),
  })
  const [selectedIds, setSelectedIds] = useState<string[]>([])
  const [selectedTransactionId, setSelectedTransactionId] = useState<
    string | null
  >(null)
  const [mergeMode, setMergeMode] = useState<ManualMergeMode | null>(null)
  const [addDialogOpen, setAddDialogOpen] = useState(false)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [reviewError, setReviewError] = useState<string | null>(null)

  const todayStr = useMemo(() => getTodayDateString(timezone), [timezone])
  const isAllDates = filters.range === "all"
  const activeRange = {
    from: filters.from ?? todayStr,
    to: filters.to ?? filters.from ?? todayStr,
  }
  const activePage = Math.max(1, filters.page)

  const txQuery = useQuery({
    ...transactionsQuery({
      startDate: isAllDates ? undefined : activeRange.from,
      endDate: isAllDates ? undefined : activeRange.to,
      category: filters.category === "all" ? undefined : filters.category,
      tag: filters.tag === "all" ? undefined : filters.tag,
      accountId: filters.accountId === "all" ? undefined : filters.accountId,
      search: filters.search.trim() || undefined,
      unreviewedOnly: filters.unreviewed || undefined,
      page: activePage,
      pageSize: DEFAULT_PAGE_SIZE,
    }),
    enabled: isConfigured,
  })

  const items = useMemo(() => txQuery.data?.items ?? [], [txQuery.data?.items])
  const totalCount = txQuery.data?.totalCount ?? 0
  const totalInRangeCount = txQuery.data?.totalInRangeCount ?? 0
  const unreviewedInRangeCount = txQuery.data?.unreviewedInRangeCount ?? 0
  const totalPages = Math.max(1, Math.ceil(totalCount / DEFAULT_PAGE_SIZE))

  // Clamp page if items shrink after filtering, merging, or reviewing.
  useEffect(() => {
    if (txQuery.isSuccess && activePage > totalPages) {
      void setFilters({ page: totalPages })
      setSelectedIds([])
    }
  }, [txQuery.isSuccess, activePage, totalPages, setFilters])

  // Prune stale selected IDs if background sync shifts page contents.
  useEffect(() => {
    const visibleIds = new Set(items.map((tx) => tx.id))
    setSelectedIds((prev) => {
      const next = prev.filter((id) => visibleIds.has(id))
      return next.length === prev.length ? prev : next
    })
  }, [items])

  const markReviewedMutation = useMutation({
    mutationFn: markTransactionsReviewedApi,
    onMutate: () => setReviewError(null),
    onSuccess: () => {
      setSelectedIds([])
      void queryClient.invalidateQueries({ queryKey: ["transactions"] })
      void queryClient.invalidateQueries({ queryKey: ["spending"] })
    },
    onError: (err) => {
      setReviewError(
        err instanceof Error
          ? err.message
          : "Failed to mark transactions reviewed."
      )
    },
  })

  const deleteTransactionsMutation = useMutation({
    mutationFn: deleteTransactionsApi,
    onSuccess: () => {
      setSelectedIds([])
      void queryClient.invalidateQueries({ queryKey: ["transactions"] })
      void queryClient.invalidateQueries({ queryKey: ["spending"] })
      void queryClient.invalidateQueries({ queryKey: ["accounts"] })
      void queryClient.invalidateQueries({ queryKey: ["categories"] })
      void queryClient.invalidateQueries({ queryKey: ["tags"] })
    },
    onSettled: () => {
      setDeleteDialogOpen(false)
    },
  })

  const updateRange = (next: { from: string; to: string }) => {
    void setFilters({ range: null, from: next.from, to: next.to, page: 1 })
    setSelectedIds([])
    deleteTransactionsMutation.reset()
  }

  const handleToggleAllDates = () => {
    if (isAllDates) {
      void setFilters({ range: null, from: null, to: null, page: 1 })
    } else {
      void setFilters({ range: "all", from: null, to: null, page: 1 })
    }
    setSelectedIds([])
    deleteTransactionsMutation.reset()
  }

  const handleShiftRange = (direction: -1 | 1) => {
    updateRange(shiftDateRange(activeRange.from, activeRange.to, direction))
  }

  const handleSearchChange = (value: string) => {
    void setFilters({ search: value, page: 1 })
    setSelectedIds([])
    deleteTransactionsMutation.reset()
  }

  const handleCategoryChange = (value: string) => {
    void setFilters({ category: value, page: 1 })
    setSelectedIds([])
    deleteTransactionsMutation.reset()
  }

  const handleTagChange = (value: string) => {
    void setFilters({ tag: value, page: 1 })
    setSelectedIds([])
    deleteTransactionsMutation.reset()
  }

  const handleAccountIdChange = (value: string) => {
    void setFilters({ accountId: value, page: 1 })
    setSelectedIds([])
    deleteTransactionsMutation.reset()
  }

  const handleUnreviewedOnlyChange = (value: boolean) => {
    void setFilters({ unreviewed: value, page: 1 })
    setSelectedIds([])
    deleteTransactionsMutation.reset()
  }

  const handlePageChange = (nextPage: number) => {
    void setFilters({ page: Math.max(1, nextPage) })
    setSelectedIds([])
    deleteTransactionsMutation.reset()
  }

  const handleToggleRow = (id: string, checked: boolean) => {
    deleteTransactionsMutation.reset()
    setSelectedIds((prev) => {
      if (checked) {
        return prev.includes(id) ? prev : [...prev, id]
      }
      return prev.filter((existing) => existing !== id)
    })
  }

  const handleToggleSelectAllPage = (checked: boolean) => {
    deleteTransactionsMutation.reset()
    if (checked) {
      setSelectedIds(items.map((tx) => tx.id))
    } else {
      setSelectedIds([])
    }
  }

  const selectedTransactions = useMemo(() => {
    const selectedSet = new Set(selectedIds)
    return items.filter((tx) => selectedSet.has(tx.id))
  }, [items, selectedIds])

  const allInRangeReviewed =
    !txQuery.isLoading && totalInRangeCount > 0 && unreviewedInRangeCount === 0
  const allOnPageReviewedButMoreUnreviewed =
    !txQuery.isLoading &&
    items.length > 0 &&
    items.every((tx) => tx.reviewed === 1) &&
    unreviewedInRangeCount > 0

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Transactions
          </h1>
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
            disabled={!isConfigured || isAllDates}
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
            disabled={!isConfigured || isAllDates}
            onClick={() => handleShiftRange(1)}
          >
            Next
            <ChevronRightIcon className="ml-1 size-4" />
          </Button>

          <Button
            type="button"
            variant={isAllDates ? "default" : "outline"}
            size="sm"
            disabled={!isConfigured}
            onClick={handleToggleAllDates}
          >
            All dates
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
            <Link href="/settings" className={buttonVariants({ size: "sm" })}>
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
                      ? "Previous day"
                      : "Previous range"}
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => handleShiftRange(1)}
                  >
                    {activeRange.from === activeRange.to
                      ? "Next day"
                      : "Next range"}
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
                  All transactions on this page are reviewed (
                  {unreviewedInRangeCount} unreviewed remaining in this date
                  range).
                </span>
                {activePage < totalPages && (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => handlePageChange(activePage + 1)}
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
                  : "Failed to load transactions."
                : null
            }
            search={filters.search}
            onSearchChange={handleSearchChange}
            category={filters.category}
            onCategoryChange={handleCategoryChange}
            tag={filters.tag}
            onTagChange={handleTagChange}
            accountId={filters.accountId}
            onAccountIdChange={handleAccountIdChange}
            unreviewedOnly={filters.unreviewed}
            onUnreviewedOnlyChange={handleUnreviewedOnlyChange}
            selectedIds={selectedIds}
            onToggleRow={handleToggleRow}
            onToggleSelectAllPage={handleToggleSelectAllPage}
            onSelectTransaction={setSelectedTransactionId}
            onOpenDuplicateMerge={() => setMergeMode("duplicate")}
            onOpenCombine={() => setMergeMode("combine")}
            onMarkReviewed={(ids) => markReviewedMutation.mutate(ids)}
            isMarkingReviewed={markReviewedMutation.isPending}
            reviewErrorMessage={reviewError}
            onDeleteSelected={(ids) => deleteTransactionsMutation.mutate(ids)}
            deleteDialogOpen={deleteDialogOpen}
            onDeleteDialogOpenChange={(open) => {
              if (!open && deleteTransactionsMutation.isPending) return
              setDeleteDialogOpen(open)
            }}
            isDeleting={deleteTransactionsMutation.isPending}
            deleteErrorMessage={
              deleteTransactionsMutation.isError
                ? deleteTransactionsMutation.error instanceof Error
                  ? deleteTransactionsMutation.error.message
                  : "Failed to delete transactions."
                : null
            }
            page={activePage}
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
  )
}
