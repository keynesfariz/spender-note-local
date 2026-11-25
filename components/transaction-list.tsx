"use client"

import { useQuery } from "@tanstack/react-query"
import {
  CheckCircle2Icon,
  Clock3Icon,
  CopyIcon,
  MergeIcon,
  PencilIcon,
  RotateCcwIcon,
  Trash2Icon,
} from "lucide-react"
import { useMemo } from "react"

import { DynamicIcon } from "@/components/dynamic-icon"

import { Alert, AlertDescription } from "@/components/ui/alert"
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
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardFooter, CardHeader } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import { Skeleton } from "@/components/ui/skeleton"
import { IconTooltip } from "@/components/ui/tooltip"
import {
  categoriesQuery,
  tagsQuery,
  type AccountSummary,
  type GhostRowItem,
  type TransactionListItem,
} from "@/lib/api"
import {
  buildCategoryIconMap,
  buildCategorySelectOptions,
} from "@/lib/categories"
import { formatMinorCurrency, formatUtcInTimezone } from "@/lib/format"

function sourceBadgeLabel(tx: TransactionListItem): string {
  if (tx.sourceCount > 1) return `${tx.sourceCount} emails`
  if (tx.sourceCount === 1) return "1 email"
  if (tx.source_type === "manual") return "Manual"
  return "0 emails"
}

function getMergeEligibility(selected: TransactionListItem[]): {
  canDuplicate: boolean
  duplicateReason: string | null
  canCombine: boolean
  combineReason: string | null
} {
  if (selected.length < 2) {
    return {
      canDuplicate: false,
      duplicateReason: "Select exactly 2 transactions to duplicate merge.",
      canCombine: false,
      combineReason: "Select at least 2 transactions to combine.",
    }
  }

  const baseCurrency = selected[0].currency
  const baseAccount = selected[0].account_id
  const sameCurrencyAndAccount = selected.every(
    (tx) => tx.currency === baseCurrency && tx.account_id === baseAccount
  )

  if (!sameCurrencyAndAccount) {
    return {
      canDuplicate: false,
      duplicateReason: "Currency and account must match to merge.",
      canCombine: false,
      combineReason: "Currency and account must match to combine.",
    }
  }

  if (selected.length !== 2) {
    return {
      canDuplicate: false,
      duplicateReason: "Select exactly 2 transactions to duplicate merge.",
      canCombine: true,
      combineReason: null,
    }
  }

  const sameAmount = selected[0].amount_minor === selected[1].amount_minor
  if (!sameAmount) {
    return {
      canDuplicate: false,
      duplicateReason: "Amounts must match for duplicate merge.",
      canCombine: true,
      combineReason: null,
    }
  }

  return {
    canDuplicate: true,
    duplicateReason: null,
    canCombine: true,
    combineReason: null,
  }
}

type TimelineEntry =
  | {
      kind: "transaction"
      tx: TransactionListItem
      sortMs: number
      index: number
    }
  | {
      kind: "ghost"
      ghost: GhostRowItem
      sortMs: number
      index: number
    }

function parseSortEpochMs(isoUtc: string): number {
  const ms = Date.parse(isoUtc)
  return Number.isNaN(ms) ? 0 : ms
}

export function TransactionList({
  transactions,
  ghostRows,
  ghostRowsTruncated,
  accounts,
  timezone,
  isLoading,
  errorMessage,
  search,
  onSearchChange,
  category,
  onCategoryChange,
  tag,
  onTagChange,
  accountId,
  onAccountIdChange,
  unreviewedOnly,
  onUnreviewedOnlyChange,
  showSkipped,
  onShowSkippedChange,
  onResetFilters,
  selectedIds,
  onToggleRow,
  onToggleSelectAllPage,
  onSelectTransaction,
  onOpenDuplicateMerge,
  onOpenCombine,
  onMarkReviewed,
  isMarkingReviewed,
  reviewErrorMessage,
  onDeleteSelected,
  deleteDialogOpen,
  onDeleteDialogOpenChange,
  isDeleting,
  deleteErrorMessage,
  onTriageVerdict,
  pendingTriageVerdict,
  triageErrorMessage,
  page,
  pageSize,
  totalCount,
  onPageChange,
}: {
  transactions: TransactionListItem[]
  ghostRows: GhostRowItem[]
  ghostRowsTruncated: boolean
  accounts: AccountSummary[]
  timezone: string | null | undefined
  isLoading: boolean
  errorMessage: string | null
  search: string
  onSearchChange: (value: string) => void
  category: string
  onCategoryChange: (value: string) => void
  tag: string
  onTagChange: (value: string) => void
  accountId: string
  onAccountIdChange: (value: string) => void
  unreviewedOnly: boolean
  onUnreviewedOnlyChange: (value: boolean) => void
  showSkipped: boolean
  onShowSkippedChange: (value: boolean) => void
  onResetFilters: () => void
  selectedIds: string[]
  onToggleRow: (id: string, checked: boolean) => void
  onToggleSelectAllPage: (checked: boolean) => void
  onSelectTransaction: (id: string) => void
  onOpenDuplicateMerge: () => void
  onOpenCombine: () => void
  onMarkReviewed: (ids: string[]) => void
  isMarkingReviewed: boolean
  reviewErrorMessage: string | null
  onDeleteSelected: (ids: string[]) => void
  deleteDialogOpen: boolean
  onDeleteDialogOpenChange: (open: boolean) => void
  isDeleting: boolean
  deleteErrorMessage: string | null
  onTriageVerdict: (emailId: string, action: "promote" | "ignore") => void
  pendingTriageVerdict: { emailId: string; action: "promote" | "ignore" } | null
  triageErrorMessage: string | null
  page: number
  pageSize: number
  totalCount: number
  onPageChange: (nextPage: number) => void
}) {
  const catListQuery = useQuery(categoriesQuery())
  const tagListQuery = useQuery(tagsQuery())

  const timelineEntries = useMemo<TimelineEntry[]>(() => {
    const merged: TimelineEntry[] = [
      ...transactions.map((tx, index) => ({
        kind: "transaction" as const,
        tx,
        sortMs: parseSortEpochMs(tx.occurred_at_utc),
        index,
      })),
      ...ghostRows.map((ghost, index) => ({
        kind: "ghost" as const,
        ghost,
        sortMs: parseSortEpochMs(ghost.occurred_at_utc),
        index,
      })),
    ]
    merged.sort((a, b) => {
      if (a.sortMs !== b.sortMs) {
        return b.sortMs - a.sortMs
      }
      if (a.kind !== b.kind) {
        return a.kind === "transaction" ? -1 : 1
      }
      return a.index - b.index
    })
    return merged
  }, [transactions, ghostRows])

  const selectedSet = useMemo(() => new Set(selectedIds), [selectedIds])
  const selectedTransactions = useMemo(
    () => transactions.filter((tx) => selectedSet.has(tx.id)),
    [transactions, selectedSet]
  )

  const allOnPageSelected =
    transactions.length > 0 &&
    selectedTransactions.length === transactions.length
  const someOnPageSelected =
    selectedTransactions.length > 0 && !allOnPageSelected

  const eligibility = useMemo(
    () => getMergeEligibility(selectedTransactions),
    [selectedTransactions]
  )
  const totalPages = Math.max(1, Math.ceil(totalCount / pageSize))

  const disabledHint =
    eligibility.duplicateReason && eligibility.combineReason
      ? eligibility.combineReason
      : eligibility.duplicateReason

  const categoryIconMap = useMemo(
    () => buildCategoryIconMap(catListQuery.data),
    [catListQuery.data]
  )

  const categoryFilterItems = useMemo(
    () => [
      { value: "", label: "All categories" },
      ...buildCategorySelectOptions(catListQuery.data, category),
    ],
    [catListQuery.data, category]
  )

  const tagFilterItems = useMemo(() => {
    const items = [
      { value: "", label: "All tags" },
      ...(tagListQuery.data ?? []).map((t) => ({
        value: t.name,
        label: t.name,
      })),
    ]
    if (tag && !items.some((item) => item.value === tag)) {
      items.push({ value: tag, label: tag })
    }
    return items
  }, [tagListQuery.data, tag])

  const accountFilterItems = useMemo(() => {
    const items = [
      { value: "all", label: "All accounts" },
      { value: "unassigned", label: "Unassigned" },
      ...accounts.map((acct) => ({ value: acct.id, label: acct.name })),
    ]
    if (
      accountId !== "all" &&
      accountId !== "unassigned" &&
      !items.some((item) => item.value === accountId)
    ) {
      const matchedName = transactions.find(
        (tx) => tx.account_id === accountId && tx.account_name
      )?.account_name
      items.push({ value: accountId, label: matchedName ?? "Account" })
    }
    return items
  }, [accounts, accountId, transactions])

  return (
    <Card>
      <CardHeader className="space-y-3">
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-[repeat(4,minmax(0,1fr))_auto]">
          <Input
            type="search"
            aria-label="Search transactions"
            placeholder="Search remarks, merchants, or notes..."
            value={search}
            onChange={(e) => onSearchChange(e.target.value)}
          />

          <Select
            items={categoryFilterItems}
            value={category}
            onValueChange={(value) => onCategoryChange(value ?? "")}
          >
            <SelectTrigger className="w-full" aria-label="Filter by category">
              <SelectValue placeholder="All categories" />
            </SelectTrigger>
            <SelectContent>
              {categoryFilterItems.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.value === "" ? (
                    item.label
                  ) : (
                    <span className="flex items-center gap-2">
                      <DynamicIcon
                        name={categoryIconMap.get(item.value)}
                        className="size-3.5 text-muted-foreground"
                      />
                      <span>{item.label}</span>
                    </span>
                  )}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Select
            items={tagFilterItems}
            value={tag}
            onValueChange={(value) => onTagChange(value ?? "")}
          >
            <SelectTrigger className="w-full" aria-label="Filter by tag">
              <SelectValue placeholder="All tags" />
            </SelectTrigger>
            <SelectContent>
              {tagFilterItems.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Select
            items={accountFilterItems}
            value={accountId}
            onValueChange={(value) => onAccountIdChange(value ?? "all")}
          >
            <SelectTrigger className="w-full" aria-label="Filter by account">
              <SelectValue placeholder="All accounts" />
            </SelectTrigger>
            <SelectContent>
              {accountFilterItems.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <div className="flex flex-wrap items-center gap-x-4 gap-y-2 sm:col-span-2 lg:col-span-1">
            <div className="flex items-center gap-2">
              <Checkbox
                id="unreviewed-filter"
                checked={unreviewedOnly}
                onCheckedChange={(checked) =>
                  onUnreviewedOnlyChange(Boolean(checked))
                }
              />
              <Label htmlFor="unreviewed-filter">Needs review</Label>
            </div>
            <div className="flex items-center gap-2">
              <Checkbox
                id="show-skipped-filter"
                checked={showSkipped}
                onCheckedChange={(checked) =>
                  onShowSkippedChange(Boolean(checked))
                }
              />
              <Label htmlFor="show-skipped-filter">Show skipped</Label>
            </div>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={onResetFilters}
              disabled={
                !search &&
                !category &&
                !tag &&
                accountId === "all" &&
                !unreviewedOnly &&
                showSkipped
              }
            >
              <RotateCcwIcon aria-hidden="true" />
              Reset filters
            </Button>
          </div>
        </div>

        <div className="space-y-2 rounded-lg border bg-muted/40 px-3 py-3">
          <div
            className="flex flex-wrap items-center gap-2"
            role="group"
            aria-label="Bulk actions"
          >
            <IconTooltip label="Select all on page">
              <Checkbox
                aria-label="Select all on page"
                className="mr-2"
                disabled={transactions.length === 0 || isLoading}
                checked={allOnPageSelected}
                indeterminate={someOnPageSelected}
                onCheckedChange={(checked) =>
                  onToggleSelectAllPage(Boolean(checked))
                }
              />
            </IconTooltip>
            <IconTooltip label="Mark reviewed">
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="w-8 px-0 md:w-auto md:px-2.5"
                disabled={
                  !selectedTransactions.some((tx) => tx.reviewed === 0) ||
                  isMarkingReviewed ||
                  isDeleting
                }
                onClick={() =>
                  onMarkReviewed(selectedTransactions.map((tx) => tx.id))
                }
              >
                <CheckCircle2Icon aria-hidden="true" />
                <span className="sr-only md:not-sr-only">
                  {isMarkingReviewed ? "Marking..." : "Mark reviewed"}
                </span>
              </Button>
            </IconTooltip>
            <IconTooltip label="Duplicate merge">
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="w-8 px-0 md:w-auto md:px-2.5"
                disabled={!eligibility.canDuplicate || isDeleting}
                aria-describedby={
                  selectedTransactions.length > 0 &&
                  disabledHint &&
                  !eligibility.canDuplicate
                    ? "merge-eligibility-reason"
                    : undefined
                }
                onClick={onOpenDuplicateMerge}
              >
                <CopyIcon aria-hidden="true" />
                <span className="sr-only md:not-sr-only">Duplicate merge</span>
              </Button>
            </IconTooltip>
            <IconTooltip label="Combine transactions">
              <Button
                type="button"
                variant="default"
                size="sm"
                className="w-8 px-0 md:w-auto md:px-2.5"
                disabled={!eligibility.canCombine || isDeleting}
                aria-describedby={
                  selectedTransactions.length > 0 &&
                  disabledHint &&
                  !eligibility.canCombine
                    ? "merge-eligibility-reason"
                    : undefined
                }
                onClick={onOpenCombine}
              >
                <MergeIcon aria-hidden="true" />
                <span className="sr-only md:not-sr-only">
                  Combine transactions
                </span>
              </Button>
            </IconTooltip>
            <AlertDialog
              open={deleteDialogOpen}
              onOpenChange={onDeleteDialogOpenChange}
            >
              <IconTooltip label="Delete selected transactions">
                <AlertDialogTrigger
                  render={
                    <Button
                      type="button"
                      variant="destructive"
                      size="sm"
                      className="w-8 px-0 md:w-auto md:px-2.5"
                      disabled={
                        selectedTransactions.length === 0 ||
                        isDeleting ||
                        isMarkingReviewed
                      }
                    />
                  }
                >
                  <Trash2Icon aria-hidden="true" />
                  <span className="sr-only md:not-sr-only">
                    {isDeleting ? "Deleting..." : "Delete"}
                  </span>
                </AlertDialogTrigger>
              </IconTooltip>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>
                    Delete {selectedTransactions.length}{" "}
                    {selectedTransactions.length === 1
                      ? "transaction"
                      : "transactions"}
                    ?
                  </AlertDialogTitle>
                  <AlertDialogDescription>
                    This permanently deletes the selected{" "}
                    {selectedTransactions.length === 1
                      ? "transaction"
                      : "transactions"}{" "}
                    and marks any linked source emails as dismissed.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel disabled={isDeleting}>
                    Cancel
                  </AlertDialogCancel>
                  <AlertDialogAction
                    variant="destructive"
                    disabled={isDeleting}
                    onClick={() =>
                      onDeleteSelected(selectedTransactions.map((tx) => tx.id))
                    }
                  >
                    {isDeleting ? "Deleting..." : "Delete forever"}
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          </div>
          <div className="flex min-h-8 items-baseline gap-3 text-xs leading-4 text-muted-foreground sm:min-h-4">
            <span
              role="status"
              className="shrink-0 font-medium text-foreground"
            >
              {selectedTransactions.length > 0
                ? `${selectedTransactions.length} selected`
                : null}
            </span>
            <span id="merge-eligibility-reason">
              {selectedTransactions.length > 0 ? disabledHint : null}
            </span>
          </div>
        </div>

        {reviewErrorMessage && (
          <Alert variant="destructive">
            <AlertDescription>{reviewErrorMessage}</AlertDescription>
          </Alert>
        )}
        {deleteErrorMessage && (
          <Alert variant="destructive">
            <AlertDescription>{deleteErrorMessage}</AlertDescription>
          </Alert>
        )}
        {triageErrorMessage && (
          <Alert variant="destructive">
            <AlertDescription>{triageErrorMessage}</AlertDescription>
          </Alert>
        )}
      </CardHeader>

      <CardContent>
        {isLoading ? (
          <div className="space-y-3 py-2">
            <Skeleton className="h-14 w-full" />
            <Skeleton className="h-14 w-full" />
            <Skeleton className="h-14 w-full" />
          </div>
        ) : errorMessage ? (
          <Alert variant="destructive">
            <AlertDescription>{errorMessage}</AlertDescription>
          </Alert>
        ) : transactions.length === 0 && ghostRows.length === 0 ? (
          <p className="py-8 text-center text-sm text-muted-foreground">
            No transactions match the current filters.
          </p>
        ) : (
          <>
            <ul className="space-y-1">
              {timelineEntries.map((entry, idx) => {
                if (entry.kind === "ghost") {
                  const ghost = entry.ghost
                  const ghostTitle =
                    ghost.merchant_raw?.trim() ||
                    ghost.subject.trim() ||
                    ghost.sender_email
                  const bodyCue = ghost.triage_body_cue.trim()
                  const subjectSubtitle = ghost.subject.trim()
                  const framingLine =
                    bodyCue ||
                    (subjectSubtitle && subjectSubtitle !== ghostTitle
                      ? subjectSubtitle
                      : "")
                  const formattedAmount = formatMinorCurrency(
                    ghost.amount_minor,
                    ghost.currency
                  )
                  const isTriageBusy = pendingTriageVerdict !== null
                  const isPromoting =
                    pendingTriageVerdict?.emailId === ghost.email_id &&
                    pendingTriageVerdict.action === "promote"
                  const isIgnoring =
                    pendingTriageVerdict?.emailId === ghost.email_id &&
                    pendingTriageVerdict.action === "ignore"

                  return (
                    <li key={`ghost-${ghost.email_id}`}>
                      {idx > 0 && <Separator className="my-1" />}
                      <div className="flex flex-col gap-3 rounded-lg border border-dashed border-input bg-muted/35 p-3 sm:flex-row sm:items-start sm:justify-between">
                        <div className="min-w-0 flex-1 space-y-1.5">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="truncate font-semibold text-foreground">
                              {ghostTitle}
                            </span>
                            <Badge variant="outline">Skipped</Badge>
                            {ghost.category && (
                              <Badge variant="secondary" className="gap-1">
                                <DynamicIcon
                                  name={categoryIconMap.get(ghost.category)}
                                  className="size-3"
                                />
                                {ghost.category}
                              </Badge>
                            )}
                          </div>
                          <p className="text-xs text-muted-foreground">
                            {ghost.account_name ??
                              (ghost.payment_method_raw
                                ? `Unassigned (${ghost.payment_method_raw})`
                                : "Unassigned")}{" "}
                            ·{" "}
                            {formatUtcInTimezone(
                              ghost.occurred_at_utc,
                              timezone
                            )}{" "}
                            · {ghost.sender_email}
                          </p>
                          {framingLine && (
                            <p className="text-xs text-muted-foreground">
                              {framingLine}
                            </p>
                          )}
                        </div>
                        <div className="flex shrink-0 flex-col gap-2 sm:items-end sm:text-right">
                          <div>
                            <span className="block font-semibold break-all tabular-nums">
                              {formattedAmount}
                            </span>
                            <span className="block text-xs text-muted-foreground">
                              {ghost.isForeignCurrency
                                ? `Foreign (${ghost.currency}) · Not imported`
                                : "Not imported"}
                            </span>
                          </div>
                          <div className="flex flex-wrap items-center gap-1.5 sm:justify-end">
                            <Button
                              type="button"
                              size="xs"
                              variant="default"
                              disabled={isTriageBusy}
                              onClick={() =>
                                onTriageVerdict(ghost.email_id, "promote")
                              }
                            >
                              {isPromoting ? "Promoting..." : "Promote"}
                            </Button>
                            <Button
                              type="button"
                              size="xs"
                              variant="outline"
                              disabled={isTriageBusy}
                              onClick={() =>
                                onTriageVerdict(ghost.email_id, "ignore")
                              }
                            >
                              {isIgnoring ? "Ignoring..." : "Always ignore"}
                            </Button>
                          </div>
                        </div>
                      </div>
                    </li>
                  )
                }

                const tx = entry.tx
                const title = tx.remark ?? tx.merchant
                const formattedAmount = formatMinorCurrency(
                  tx.amount_minor,
                  tx.currency
                )
                const isSelected = selectedSet.has(tx.id)
                return (
                  <li key={tx.id}>
                    {idx > 0 && <Separator className="my-1" />}
                    <div
                      data-selected={isSelected}
                      className="flex items-center gap-2 rounded-lg p-3 transition-colors hover:bg-muted data-[selected=true]:bg-secondary/60 data-[selected=true]:hover:bg-secondary"
                    >
                      <Checkbox
                        aria-label={`Select ${title} (${formattedAmount})`}
                        checked={isSelected}
                        onCheckedChange={(checked) =>
                          onToggleRow(tx.id, Boolean(checked))
                        }
                        className="mt-1 self-start"
                      />
                      <Button
                        type="button"
                        variant="ghost"
                        aria-label={`Inspect ${title} (${formattedAmount})${tx.manual_override === 1 ? ", edited" : ""}`}
                        onClick={() => onSelectTransaction(tx.id)}
                        className="grid h-auto min-w-0 flex-1 grid-cols-1 items-start gap-x-4 gap-y-2 px-2 text-left font-normal whitespace-normal hover:bg-transparent sm:grid-cols-[minmax(0,1fr)_auto] dark:hover:bg-transparent"
                      >
                        <span className="min-w-0 space-y-1">
                          <span className="flex items-center gap-2">
                            <span className="truncate font-semibold">
                              {title}
                            </span>
                            {tx.manual_override === 1 && (
                              <IconTooltip label="Edited">
                                <span className="text-muted-foreground">
                                  <PencilIcon
                                    className="size-3"
                                    aria-hidden="true"
                                  />
                                </span>
                              </IconTooltip>
                            )}
                          </span>
                          <span className="block text-xs text-muted-foreground">
                            {tx.account_name ??
                              (tx.payment_method_raw
                                ? `Unassigned (${tx.payment_method_raw})`
                                : "Unassigned")}{" "}
                            ·{" "}
                            {formatUtcInTimezone(tx.occurred_at_utc, timezone)}
                          </span>
                        </span>
                        <span className="flex flex-wrap items-center gap-1.5 sm:col-start-1 sm:row-start-2">
                          <Badge variant="secondary" className="gap-1">
                            <DynamicIcon
                              name={categoryIconMap.get(tx.category)}
                              className="size-3"
                            />
                            {tx.category}
                          </Badge>
                          {(tx.tags ?? []).map((tag) => (
                            <Badge
                              key={tag.id}
                              variant="outline"
                              className="text-muted-foreground"
                            >
                              #{tag.name}
                            </Badge>
                          ))}
                        </span>
                        <span className="min-w-0 sm:col-start-2 sm:row-span-2 sm:row-start-1 sm:text-right">
                          <span className="block font-semibold break-all tabular-nums">
                            {formattedAmount}
                          </span>
                          <span className="block text-xs text-muted-foreground">
                            {sourceBadgeLabel(tx)}
                            {tx.isForeignCurrency
                              ? ` · Foreign (${tx.currency})`
                              : ""}
                          </span>
                        </span>
                      </Button>
                      {tx.reviewed === 1 ? (
                        <IconTooltip label="Reviewed">
                          <span className="grid size-9 shrink-0 place-items-center rounded-full bg-emerald-100 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300">
                            <CheckCircle2Icon
                              className="size-4"
                              aria-hidden="true"
                            />
                            <span className="sr-only">Reviewed</span>
                          </span>
                        </IconTooltip>
                      ) : (
                        <IconTooltip label="Needs review · Mark reviewed">
                          <Button
                            type="button"
                            variant="ghost"
                            size="icon"
                            aria-label={`Mark ${title} as reviewed`}
                            disabled={isMarkingReviewed || isDeleting}
                            onClick={() => onMarkReviewed([tx.id])}
                            className="rounded-full bg-amber-100 text-amber-800 hover:bg-amber-200 hover:text-amber-950 dark:bg-amber-950 dark:text-amber-300 dark:hover:bg-amber-900"
                          >
                            <Clock3Icon className="size-4" aria-hidden="true" />
                          </Button>
                        </IconTooltip>
                      )}
                    </div>
                  </li>
                )
              })}
            </ul>
            {ghostRowsTruncated && (
              <p className="mt-3 text-center text-xs text-muted-foreground">
                Showing the first 100 skipped emails matching these filters.
                Narrow your date range or search to inspect earlier entries.
              </p>
            )}
          </>
        )}
      </CardContent>

      {totalCount > 0 && (
        <CardFooter className="flex flex-wrap items-center justify-between gap-3 border-t pt-4 text-xs text-muted-foreground">
          <span>
            Showing {(page - 1) * pageSize + 1}–
            {Math.min(page * pageSize, totalCount)} of {totalCount}{" "}
            {totalCount === 1 ? "transaction" : "transactions"}
          </span>
          <div className="flex items-center gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={page <= 1 || isLoading}
              onClick={() => onPageChange(page - 1)}
            >
              Previous page
            </Button>
            <span className="tabular-nums">
              Page {page} of {totalPages}
            </span>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={page >= totalPages || isLoading}
              onClick={() => onPageChange(page + 1)}
            >
              Next page
            </Button>
          </div>
        </CardFooter>
      )}
    </Card>
  )
}
