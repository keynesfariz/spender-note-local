"use client"

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  CheckCircle2Icon,
  Clock3Icon,
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
import { TransactionInspectorDrawer } from "@/components/transaction-inspector-drawer"
import { TransactionList } from "@/components/transaction-list"
import { TriageUndoToast } from "@/components/triage-undo-toast"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button, buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import {
  accountsQuery,
  applyTriageVerdictApi,
  deleteTransactionsApi,
  invalidateTransactionRelatedQueries,
  invalidateTriageRelatedQueries,
  markTransactionsReviewedApi,
  settingsQuery,
  transactionsQuery,
  undoTriageVerdictApi,
  type TriageVerdictResult,
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
    category: parseAsString.withDefault(""),
    tag: parseAsString.withDefault(""),
    accountId: parseAsString.withDefault("all"),
    unreviewed: parseAsBoolean.withDefault(false),
    skipped: parseAsBoolean.withDefault(true),
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
  const [triageError, setTriageError] = useState<string | null>(null)
  const [undoError, setUndoError] = useState<string | null>(null)
  const [pendingVerdict, setPendingVerdict] = useState<{
    emailId: string
    action: "promote" | "ignore"
  } | null>(null)
  const [activeUndo, setActiveUndo] = useState<{
    key: string
    verdict: TriageVerdictResult
    action: "promote" | "ignore"
  } | null>(null)

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
      category: filters.category || undefined,
      tag: filters.tag || undefined,
      accountId: filters.accountId === "all" ? undefined : filters.accountId,
      search: filters.search.trim() || undefined,
      unreviewedOnly: filters.unreviewed || undefined,
      showSkipped: filters.skipped,
      page: activePage,
      pageSize: DEFAULT_PAGE_SIZE,
    }),
    enabled: isConfigured,
  })

  const items = useMemo(() => txQuery.data?.items ?? [], [txQuery.data?.items])
  const ghostRows = useMemo(
    () => txQuery.data?.ghostRows ?? [],
    [txQuery.data?.ghostRows]
  )
  const ghostInRangeCount = txQuery.data?.ghostInRangeCount ?? 0
  const totalGhostCount = txQuery.data?.totalGhostCount ?? 0
  const ghostRowsTruncated = txQuery.data?.ghostRowsTruncated ?? false
  const totalCount = txQuery.data?.totalCount ?? 0
  const totalInRangeCount = txQuery.data?.totalInRangeCount ?? 0
  const unreviewedInRangeCount = txQuery.data?.unreviewedInRangeCount ?? 0
  const totalPages = Math.max(1, Math.ceil(totalCount / DEFAULT_PAGE_SIZE))

  const clearSelection = () => {
    setSelectedIds((prev) => (prev.length === 0 ? prev : []))
  }

  const clearActiveUndo = () => {
    setActiveUndo(null)
    setUndoError(null)
  }

  const pageOutOfRange = txQuery.isSuccess && activePage > totalPages
  useEffect(() => {
    if (pageOutOfRange) {
      void setFilters({ page: totalPages })
    }
  }, [pageOutOfRange, activePage, totalPages, setFilters])

  const [previousItems, setPreviousItems] = useState(items)
  if (previousItems !== items) {
    setPreviousItems(items)
    const visibleIds = new Set(items.map((tx) => tx.id))
    const visibleSelection = selectedIds.filter((id) => visibleIds.has(id))
    if (visibleSelection.length !== selectedIds.length) {
      setSelectedIds(visibleSelection)
    }
  }
  if (pageOutOfRange && selectedIds.length > 0) {
    setSelectedIds([])
  }

  const markReviewedMutation = useMutation({
    mutationFn: markTransactionsReviewedApi,
    onMutate: () => setReviewError(null),
    onSuccess: () => {
      clearSelection()
      clearActiveUndo()
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
      clearSelection()
      clearActiveUndo()
      invalidateTransactionRelatedQueries(queryClient)
    },
    onSettled: () => {
      setDeleteDialogOpen(false)
    },
  })

  const triageVerdictMutation = useMutation({
    mutationFn: (vars: { emailId: string; action: "promote" | "ignore" }) =>
      applyTriageVerdictApi({
        emailId: vars.emailId,
        action: vars.action,
        startDate: isAllDates ? undefined : activeRange.from,
        endDate: isAllDates ? undefined : activeRange.to,
      }),
    onMutate: (vars) => {
      setPendingVerdict(vars)
      setTriageError(null)
      setUndoError(null)
    },
    onSuccess: (verdict, vars) => {
      setActiveUndo({
        key: `${vars.action}:${verdict.policy.signature_key}:${verdict.affectedEmailIds.join(",")}:${Date.now()}`,
        verdict,
        action: vars.action,
      })
      invalidateTriageRelatedQueries(queryClient)
    },
    onError: (err) => {
      setTriageError(
        err instanceof Error ? err.message : "Failed to update skipped email."
      )
    },
    onSettled: () => {
      setPendingVerdict(null)
    },
  })

  const undoVerdictMutation = useMutation({
    mutationFn: undoTriageVerdictApi,
    onMutate: () => setUndoError(null),
    onSuccess: () => {
      clearActiveUndo()
      invalidateTriageRelatedQueries(queryClient)
    },
    onError: (err) => {
      setUndoError(
        err instanceof Error ? err.message : "Failed to undo triage action."
      )
      invalidateTriageRelatedQueries(queryClient)
    },
  })

  const updateRange = (next: { from: string; to: string }) => {
    void setFilters({ range: null, from: next.from, to: next.to, page: 1 })
    clearSelection()
    deleteTransactionsMutation.reset()
  }

  const handleToggleAllDates = () => {
    if (isAllDates) {
      void setFilters({ range: null, from: null, to: null, page: 1 })
    } else {
      void setFilters({ range: "all", from: null, to: null, page: 1 })
    }
    clearSelection()
    deleteTransactionsMutation.reset()
  }

  const handleShiftRange = (direction: -1 | 1) => {
    updateRange(shiftDateRange(activeRange.from, activeRange.to, direction))
  }

  const handleSearchChange = (value: string) => {
    void setFilters({ search: value, page: 1 })
    clearSelection()
    deleteTransactionsMutation.reset()
  }

  const handleCategoryChange = (value: string) => {
    void setFilters({ category: value, page: 1 })
    clearSelection()
    deleteTransactionsMutation.reset()
  }

  const handleTagChange = (value: string) => {
    void setFilters({ tag: value, page: 1 })
    clearSelection()
    deleteTransactionsMutation.reset()
  }

  const handleAccountIdChange = (value: string) => {
    void setFilters({ accountId: value, page: 1 })
    clearSelection()
    deleteTransactionsMutation.reset()
  }

  const handleUnreviewedOnlyChange = (value: boolean) => {
    void setFilters({ unreviewed: value, page: 1 })
    clearSelection()
    deleteTransactionsMutation.reset()
  }

  const handleShowSkippedChange = (value: boolean) => {
    void setFilters({ skipped: value, page: 1 })
    clearSelection()
    deleteTransactionsMutation.reset()
  }

  const handleResetFilters = () => {
    void setFilters({
      search: null,
      category: null,
      tag: null,
      accountId: null,
      unreviewed: null,
      skipped: null,
      page: 1,
    })
    clearSelection()
    deleteTransactionsMutation.reset()
  }

  const handlePageChange = (nextPage: number) => {
    void setFilters({ page: Math.max(1, nextPage) })
    clearSelection()
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
          <div className="flex min-h-12 flex-wrap items-center justify-between gap-3 rounded-lg border bg-card px-3 py-2">
            <p
              role="status"
              className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm font-medium"
            >
              {allInRangeReviewed ? (
                <>
                  <CheckCircle2Icon className="size-4 shrink-0 text-emerald-700 dark:text-emerald-300" />
                  <span className="text-emerald-800 dark:text-emerald-300">
                    All transactions reviewed
                  </span>
                </>
              ) : txQuery.isLoading || txQuery.isError ? (
                <span className="text-muted-foreground">Review status</span>
              ) : (
                <>
                  <Clock3Icon className="size-4 shrink-0 text-amber-700 dark:text-amber-300" />
                  <span>
                    {allOnPageReviewedButMoreUnreviewed
                      ? `Page reviewed · ${unreviewedInRangeCount} left`
                      : unreviewedInRangeCount > 0
                        ? `${unreviewedInRangeCount} need review`
                        : "No transactions to review"}
                  </span>
                </>
              )}
              {!txQuery.isLoading &&
                !txQuery.isError &&
                ghostInRangeCount > 0 && (
                  <span className="font-normal text-muted-foreground">
                    · {ghostInRangeCount} skipped
                  </span>
                )}
            </p>
            <div className="flex flex-wrap items-center gap-2">
              {allOnPageReviewedButMoreUnreviewed && activePage < totalPages && (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => handlePageChange(activePage + 1)}
                >
                  Next page
                </Button>
              )}
              {!txQuery.isLoading &&
                !txQuery.isError &&
                !isAllDates &&
                filters.skipped &&
                ghostInRangeCount === 0 &&
                totalGhostCount > 0 && (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => {
                      void setFilters({
                        range: "all",
                        from: null,
                        to: null,
                        page: 1,
                      })
                      clearSelection()
                      deleteTransactionsMutation.reset()
                    }}
                  >
                    View all dates
                  </Button>
                )}
              {!txQuery.isLoading &&
                !txQuery.isError &&
                !filters.skipped &&
                totalGhostCount > 0 && (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => handleShowSkippedChange(true)}
                  >
                    Show skipped
                  </Button>
                )}
            </div>
          </div>

          <TransactionList
            transactions={items}
            ghostRows={ghostRows}
            ghostRowsTruncated={ghostRowsTruncated}
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
            showSkipped={filters.skipped}
            onShowSkippedChange={handleShowSkippedChange}
            onResetFilters={handleResetFilters}
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
            onTriageVerdict={(emailId, action) =>
              triageVerdictMutation.mutate({ emailId, action })
            }
            pendingTriageVerdict={pendingVerdict}
            triageErrorMessage={triageError}
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
        onSuccess={() => {
          setSelectedIds([])
          clearActiveUndo()
        }}
      />

      <TransactionInspectorDrawer
        transactionId={selectedTransactionId}
        accounts={accountsListQuery.data ?? []}
        localCurrency={currency}
        timezone={timezone}
        onClose={() => setSelectedTransactionId(null)}
        onTransactionMutated={clearActiveUndo}
      />

      <AddTransactionDialog
        open={addDialogOpen}
        onOpenChange={setAddDialogOpen}
        accounts={accountsListQuery.data ?? []}
        currency={currency}
        timezone={timezone}
      />

      {activeUndo && (
        <TriageUndoToast
          key={activeUndo.key}
          verdict={activeUndo.verdict}
          action={activeUndo.action}
          isUndoing={undoVerdictMutation.isPending}
          undoError={undoError}
          onUndo={() =>
            undoVerdictMutation.mutate({
              undoPayload: activeUndo.verdict.undoPayload,
            })
          }
          onDismiss={clearActiveUndo}
        />
      )}
    </div>
  )
}
