"use client"

import { CheckCircle2Icon } from "lucide-react"
import { useMemo } from "react"

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
import type { AccountSummary, TransactionListItem } from "@/lib/api"
import { DEFAULT_CATEGORIES } from "@/lib/categories"
import { formatMinorCurrency, formatUtcInTimezone } from "@/lib/format"

const CATEGORY_FILTER_ITEMS = [
  { value: "all", label: "All categories" },
  ...DEFAULT_CATEGORIES.map((cat) => ({ value: cat, label: cat })),
]

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

export function TransactionList({
  transactions,
  accounts,
  timezone,
  isLoading,
  errorMessage,
  search,
  onSearchChange,
  category,
  onCategoryChange,
  accountId,
  onAccountIdChange,
  unreviewedOnly,
  onUnreviewedOnlyChange,
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
  page,
  pageSize,
  totalCount,
  onPageChange,
}: {
  transactions: TransactionListItem[]
  accounts: AccountSummary[]
  timezone: string | null | undefined
  isLoading: boolean
  errorMessage: string | null
  search: string
  onSearchChange: (value: string) => void
  category: string
  onCategoryChange: (value: string) => void
  accountId: string
  onAccountIdChange: (value: string) => void
  unreviewedOnly: boolean
  onUnreviewedOnlyChange: (value: boolean) => void
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
  page: number
  pageSize: number
  totalCount: number
  onPageChange: (nextPage: number) => void
}) {
  const selectedSet = new Set(selectedIds)
  const selectedTransactions = transactions.filter((tx) =>
    selectedSet.has(tx.id)
  )
  const allOnPageSelected =
    transactions.length > 0 &&
    selectedTransactions.length === transactions.length
  const someOnPageSelected =
    selectedTransactions.length > 0 && !allOnPageSelected

  const eligibility = getMergeEligibility(selectedTransactions)
  const totalPages = Math.max(1, Math.ceil(totalCount / pageSize))

  const disabledHint =
    eligibility.duplicateReason && eligibility.combineReason
      ? eligibility.combineReason
      : eligibility.duplicateReason

  const accountFilterItems = useMemo(
    () => [
      { value: "all", label: "All accounts" },
      { value: "unassigned", label: "Unassigned" },
      ...accounts.map((acct) => ({ value: acct.id, label: acct.name })),
    ],
    [accounts]
  )

  return (
    <Card>
      <CardHeader className="space-y-3">
        <div className="grid gap-3 md:grid-cols-4">
          <Input
            type="search"
            aria-label="Search transactions"
            placeholder="Search merchant or note..."
            value={search}
            onChange={(e) => onSearchChange(e.target.value)}
          />

          <Select
            items={CATEGORY_FILTER_ITEMS}
            value={category}
            onValueChange={(value) => onCategoryChange(value ?? "all")}
          >
            <SelectTrigger className="w-full" aria-label="Filter by category">
              <SelectValue placeholder="All categories" />
            </SelectTrigger>
            <SelectContent>
              {CATEGORY_FILTER_ITEMS.map((item) => (
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
        </div>

        <div className="flex flex-wrap items-center justify-between gap-3 rounded-md border bg-muted/30 px-3 py-2">
          <div className="flex items-center gap-2.5">
            <Checkbox
              id="select-all-page"
              aria-label="Select all transactions on this page"
              disabled={transactions.length === 0 || isLoading}
              checked={allOnPageSelected}
              indeterminate={someOnPageSelected}
              onCheckedChange={(checked) =>
                onToggleSelectAllPage(Boolean(checked))
              }
            />
            <Label
              htmlFor="select-all-page"
              className="text-xs font-normal text-muted-foreground"
            >
              {selectedTransactions.length > 0
                ? `${selectedTransactions.length} selected`
                : "Select all on page"}
            </Label>
          </div>

          {selectedTransactions.length > 0 && (
            <div className="flex flex-wrap items-center gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={isMarkingReviewed || isDeleting}
                onClick={() =>
                  onMarkReviewed(selectedTransactions.map((tx) => tx.id))
                }
              >
                {isMarkingReviewed ? "Marking..." : "Mark reviewed"}
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={!eligibility.canDuplicate || isDeleting}
                aria-describedby={
                  !eligibility.canDuplicate
                    ? "merge-eligibility-reason"
                    : undefined
                }
                onClick={onOpenDuplicateMerge}
              >
                Duplicate merge
              </Button>
              <Button
                type="button"
                variant="default"
                size="sm"
                disabled={!eligibility.canCombine || isDeleting}
                aria-describedby={
                  !eligibility.canCombine
                    ? "merge-eligibility-reason"
                    : undefined
                }
                onClick={onOpenCombine}
              >
                Combine transactions
              </Button>
              <AlertDialog
                open={deleteDialogOpen}
                onOpenChange={onDeleteDialogOpenChange}
              >
                <AlertDialogTrigger
                  render={
                    <Button
                      type="button"
                      variant="destructive"
                      size="sm"
                      disabled={isDeleting || isMarkingReviewed}
                    />
                  }
                >
                  {isDeleting ? "Deleting..." : "Delete"}
                </AlertDialogTrigger>
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
                        onDeleteSelected(
                          selectedTransactions.map((tx) => tx.id)
                        )
                      }
                    >
                      {isDeleting ? "Deleting..." : "Delete forever"}
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
              {disabledHint && (
                <span
                  id="merge-eligibility-reason"
                  className="text-xs text-muted-foreground"
                >
                  {disabledHint}
                </span>
              )}
            </div>
          )}
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
          <ul className="space-y-1">
            {transactions.map((tx, idx) => {
              const formattedAmount = formatMinorCurrency(
                tx.amount_minor,
                tx.currency
              )
              const isSelected = selectedSet.has(tx.id)
              return (
                <li key={tx.id}>
                  {idx > 0 && <Separator className="my-1" />}
                  <div className="flex items-center gap-2 px-1 py-1">
                    <Checkbox
                      aria-label={`Select ${tx.merchant} (${formattedAmount})`}
                      checked={isSelected}
                      onCheckedChange={(checked) =>
                        onToggleRow(tx.id, Boolean(checked))
                      }
                    />
                    <Button
                      type="button"
                      variant="ghost"
                      aria-label={`Inspect ${tx.merchant} (${formattedAmount})`}
                      onClick={() => onSelectTransaction(tx.id)}
                      className="flex h-auto min-w-0 flex-1 items-center justify-between gap-4 px-2 py-2.5 text-left font-normal whitespace-normal"
                    >
                      <div className="min-w-0 flex-1 space-y-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="truncate font-medium">
                            {tx.merchant}
                          </span>
                          <Badge variant="secondary">{tx.category}</Badge>
                          <Badge variant="outline">
                            {sourceBadgeLabel(tx)}
                          </Badge>
                          {tx.reviewed === 1 ? (
                            <Badge variant="secondary" className="gap-1">
                              <CheckCircle2Icon className="size-3" />
                              Reviewed
                            </Badge>
                          ) : (
                            <Badge variant="outline">Needs review</Badge>
                          )}
                          {tx.manual_override === 1 && (
                            <Badge variant="secondary">Edited</Badge>
                          )}
                          {tx.isForeignCurrency && (
                            <Badge variant="outline">
                              Foreign ({tx.currency})
                            </Badge>
                          )}
                        </div>
                        <p className="truncate text-xs text-muted-foreground">
                          {tx.account_name ??
                            (tx.payment_method_raw
                              ? `Unassigned (${tx.payment_method_raw})`
                              : "Unassigned")}{" "}
                          · {formatUtcInTimezone(tx.occurred_at_utc, timezone)}
                        </p>
                      </div>
                      <span className="shrink-0 font-medium tabular-nums">
                        {formattedAmount}
                      </span>
                    </Button>
                    {tx.reviewed === 0 && (
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        aria-label={`Mark ${tx.merchant} as reviewed`}
                        disabled={isMarkingReviewed || isDeleting}
                        onClick={() => onMarkReviewed([tx.id])}
                        className="shrink-0"
                      >
                        Mark reviewed
                      </Button>
                    )}
                  </div>
                </li>
              )
            })}
          </ul>
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
