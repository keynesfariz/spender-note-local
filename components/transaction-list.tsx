"use client"

import { useQuery } from "@tanstack/react-query"
import {
  ArrowRightLeftIcon,
  CheckCircle2Icon,
  Clock3Icon,
  CopyIcon,
  MergeIcon,
  PencilIcon,
  RotateCcwIcon,
  Trash2Icon,
} from "lucide-react"
import { memo, useMemo } from "react"

import { DynamicIcon } from "@/components/dynamic-icon"
import { PaginationControls } from "@/components/pagination-controls"

import { CategoryBadge } from "@/components/category-badge"
import { DebouncedSearchInput } from "@/components/debounced-search-input"
import { DestructiveConfirmationContent } from "@/components/destructive-confirmation-content"
import { IconTooltip } from "@/components/icon-tooltip"
import { ReviewedIcon } from "@/components/reviewed"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { AlertDialog, AlertDialogTrigger } from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardFooter, CardHeader } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
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
import { categoriesQuery } from "@/lib/api/categories"
import { tagsQuery } from "@/lib/api/tags"
import {
  type AccountOption,
  type TransactionListItem,
} from "@/lib/api/types.generated"
import {
  buildCategoryIconMap,
  buildCategorySelectOptions,
} from "@/lib/categories"
import {
  formatTransactionAmount,
  formatUtcInTimezone,
  transactionKindLabel,
} from "@/lib/format"

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

  if (selected.some((tx) => tx.kind === "income" || tx.kind === "transfer")) {
    return {
      canDuplicate: false,
      duplicateReason: "Duplicate merge is available for expenses.",
      canCombine: false,
      combineReason: "Combine is available for expenses.",
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

export const TRANSACTION_SOURCE_OPTIONS = [
  { value: "all", label: "All sources" },
  { value: "gmail", label: "Gmail" },
  { value: "manual", label: "Manual" },
] as const

export type TransactionSourceFilter =
  (typeof TRANSACTION_SOURCE_OPTIONS)[number]["value"]

export function TransactionList({
  transactions,
  accounts,
  timezone,
  isLoading,
  errorMessage,
  search,
  searchResetKey,
  onSearchChange,
  category,
  onCategoryChange,
  tag,
  onTagChange,
  accountId,
  onAccountIdChange,
  source,
  onSourceChange,
  unreviewedOnly,
  onUnreviewedOnlyChange,
  onResetFilters,
  selectedTransactions,
  onToggleRow,
  onToggleSelectAllPage,
  onSelectTransaction,
  onOpenDuplicateMerge,
  onOpenCombine,
  onMarkAsTransfer,
  onMarkReviewed,
  isMarkingReviewed,
  reviewErrorMessage,
  onDeleteSelected,
  deleteDialogOpen,
  onDeleteDialogOpenChange,
  isDeleting,
  deleteErrorMessage,
  page,
  pageSize,
  totalCount,
  onPageChange,
}: {
  transactions: TransactionListItem[]
  accounts: AccountOption[]
  timezone: string | null | undefined
  isLoading: boolean
  errorMessage: string | null
  search: string
  searchResetKey: number
  onSearchChange: (value: string) => void
  category: string
  onCategoryChange: (value: string) => void
  tag: string
  onTagChange: (value: string) => void
  accountId: string
  onAccountIdChange: (value: string) => void
  source: TransactionSourceFilter
  onSourceChange: (value: TransactionSourceFilter) => void
  unreviewedOnly: boolean
  onUnreviewedOnlyChange: (value: boolean) => void
  onResetFilters: () => void
  selectedTransactions: TransactionListItem[]
  onToggleRow: (id: string, checked: boolean) => void
  onToggleSelectAllPage: (checked: boolean) => void
  onSelectTransaction: (id: string) => void
  onOpenDuplicateMerge: () => void
  onOpenCombine: () => void
  onMarkAsTransfer: () => void
  onMarkReviewed: (ids: string[]) => void
  isMarkingReviewed: boolean
  reviewErrorMessage: string | null
  onDeleteSelected: (ids: string[]) => void
  deleteDialogOpen: boolean
  onDeleteDialogOpenChange: (open: boolean) => void
  isDeleting: boolean
  deleteErrorMessage: string | null
  page: number
  pageSize: number
  totalCount: number
  onPageChange: (nextPage: number) => void
}) {
  const catListQuery = useQuery(categoriesQuery())
  const tagListQuery = useQuery(tagsQuery())

  const selectedSet = useMemo(
    () => new Set(selectedTransactions.map((tx) => tx.id)),
    [selectedTransactions]
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

  const selectedTransactionNoun =
    selectedTransactions.length === 1 ? "transaction" : "transactions"

  return (
    <Card>
      <CardHeader className="space-y-3">
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-[repeat(5,minmax(0,1fr))_auto]">
          <DebouncedSearchInput
            key={searchResetKey}
            label="Search transactions"
            placeholder="Search remarks, merchants, or notes..."
            value={search}
            onSearch={onSearchChange}
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

          <Select
            items={TRANSACTION_SOURCE_OPTIONS}
            value={source}
            onValueChange={(value) => onSourceChange(value ?? "all")}
          >
            <SelectTrigger className="w-full" aria-label="Filter by source">
              <SelectValue placeholder="All sources" />
            </SelectTrigger>
            <SelectContent>
              {TRANSACTION_SOURCE_OPTIONS.map((item) => (
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
                source === "all" &&
                !unreviewedOnly
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
            <IconTooltip label="Mark as transfer">
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="w-8 px-0 md:w-auto md:px-2.5"
                disabled={
                  isMarkingReviewed ||
                  isDeleting ||
                  selectedTransactions.length === 0 ||
                  selectedTransactions.some(
                    (tx) =>
                      tx.kind === "transfer" ||
                      tx.kind === "income" ||
                      tx.sourceCount === 0
                  )
                }
                onClick={onMarkAsTransfer}
              >
                <ArrowRightLeftIcon aria-hidden="true" />
                <span className="sr-only md:not-sr-only">Mark as transfer</span>
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
              <DestructiveConfirmationContent
                title={`Delete ${selectedTransactions.length} ${selectedTransactionNoun}?`}
                description={`This permanently deletes the selected ${selectedTransactionNoun} and marks any linked source emails as dismissed.`}
                confirmLabel={isDeleting ? "Deleting..." : "Delete forever"}
                confirmDisabled={isDeleting}
                cancelDisabled={isDeleting}
                onConfirm={() =>
                  onDeleteSelected(selectedTransactions.map((tx) => tx.id))
                }
              />
            </AlertDialog>
          </div>
          {selectedTransactions.length > 0 ? (
            <div className="flex items-baseline gap-3 text-xs leading-4 text-muted-foreground">
              <span
                role="status"
                className="shrink-0 font-medium text-foreground"
              >
                {`${selectedTransactions.length} selected`}
              </span>
              <span id="merge-eligibility-reason">{disabledHint}</span>
            </div>
          ) : null}
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
        ) : transactions.length === 0 ? (
          <p className="py-8 text-center text-sm text-muted-foreground">
            No transactions match the current filters.
          </p>
        ) : (
          <>
            <ul className="space-y-1">
              {transactions.map((tx, idx) => (
                <TransactionRow
                  key={tx.id}
                  tx={tx}
                  categoryIcon={categoryIconMap.get(tx.category)}
                  timezone={timezone}
                  isSelected={selectedSet.has(tx.id)}
                  separated={idx > 0}
                  actionsDisabled={isMarkingReviewed || isDeleting}
                  onToggleRow={onToggleRow}
                  onSelectTransaction={onSelectTransaction}
                  onMarkReviewed={onMarkReviewed}
                />
              ))}
            </ul>
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
            <PaginationControls
              page={page}
              totalPages={totalPages}
              isLoading={isLoading}
              onPageChange={onPageChange}
            />
          </div>
        </CardFooter>
      )}
    </Card>
  )
}

const TransactionRow = memo(function TransactionRow({
  tx,
  categoryIcon,
  timezone,
  isSelected,
  separated,
  actionsDisabled,
  onToggleRow,
  onSelectTransaction,
  onMarkReviewed,
}: {
  tx: TransactionListItem
  categoryIcon: string | undefined
  timezone: string | null | undefined
  isSelected: boolean
  separated: boolean
  actionsDisabled: boolean
  onToggleRow: (id: string, checked: boolean) => void
  onSelectTransaction: (id: string) => void
  onMarkReviewed: (ids: string[]) => void
}) {
  const title = tx.remark ?? tx.merchant
  const isTransfer = tx.kind === "transfer"
  const kindLabel = transactionKindLabel(tx.kind)
  const formattedAmount = formatTransactionAmount(tx)
  return (
    <li>
      {separated && <Separator className="my-1" />}
      <div
        data-selected={isSelected}
        className="flex items-center gap-2 rounded-lg p-3 transition-colors hover:bg-muted data-[selected=true]:bg-secondary/60 data-[selected=true]:hover:bg-secondary"
      >
        <Checkbox
          aria-label={`Select ${kindLabel.toLowerCase()} ${title} (${formattedAmount})`}
          checked={isSelected}
          onCheckedChange={(checked) => onToggleRow(tx.id, Boolean(checked))}
        />
        <Button
          type="button"
          variant="ghost"
          aria-label={`Inspect ${kindLabel.toLowerCase()} ${title} (${formattedAmount})${tx.manual_override === 1 ? ", edited" : ""}`}
          onClick={() => onSelectTransaction(tx.id)}
          className="grid h-auto min-w-0 flex-1 grid-cols-1 items-center gap-x-4 gap-y-2 px-2 text-left font-normal whitespace-normal hover:bg-transparent sm:grid-cols-[minmax(0,1fr)_auto] dark:hover:bg-transparent"
        >
          <span className="min-w-0 space-y-1">
            <span className="flex items-center gap-2">
              <span className="truncate font-semibold">{title}</span>
              {tx.manual_override === 1 && (
                <IconTooltip label="Edited">
                  <span className="text-muted-foreground">
                    <PencilIcon className="size-3" aria-hidden="true" />
                  </span>
                </IconTooltip>
              )}
            </span>
            <span className="block text-xs text-muted-foreground">
              {tx.account_name ??
                (tx.payment_method_raw
                  ? `Unassigned (${tx.payment_method_raw})`
                  : "Unassigned")}{" "}
              {isTransfer && (
                <> → {tx.destination_account_name ?? "Unknown account"}</>
              )}
              {" · "}
              {formatUtcInTimezone(tx.occurred_at_utc, timezone)}
            </span>
          </span>
          <span className="flex flex-wrap items-center gap-1.5 sm:col-start-1 sm:row-start-2">
            <Badge variant="outline">{kindLabel}</Badge>
            <CategoryBadge category={tx.category} icon={categoryIcon} />
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
              {tx.isForeignCurrency ? ` · Foreign (${tx.currency})` : ""}
            </span>
          </span>
        </Button>
        {tx.reviewed === 1 ? (
          <ReviewedIcon />
        ) : (
          <IconTooltip label="Needs review · Mark reviewed">
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={`Mark ${title} as reviewed`}
              disabled={actionsDisabled}
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
})
