"use client"

import { useMemo } from "react"
import { useQuery } from "@tanstack/react-query"
import { CategoryBadge } from "@/components/categories/category-badge"
import { PaginationControls } from "@/components/pagination-controls"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardFooter } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { categoriesQuery } from "@/lib/api/categories"
import {
  type ApplyTriageVerdictInput,
  type GhostRowItem,
} from "@/lib/api/types.generated"
import { buildCategoryIconMap } from "@/lib/categories"
import { formatMinorCurrency, formatUtcInTimezone } from "@/lib/format"

export function SkippedTransactionsList({
  rows,
  withoutDraft,
  onWithoutDraftChange,
  onCardPayment,
  timezone,
  isLoading,
  errorMessage,
  triageError,
  pendingTriageVerdict,
  isUndoing,
  onTriageVerdict,
  page,
  totalPages,
  onPageChange,
}: {
  rows: GhostRowItem[]
  withoutDraft: boolean
  onWithoutDraftChange: (value: boolean) => void
  onCardPayment: (row: GhostRowItem) => void
  timezone?: string | null
  isLoading: boolean
  errorMessage: string | null
  triageError: string | null
  pendingTriageVerdict: Pick<
    ApplyTriageVerdictInput,
    "emailId" | "action"
  > | null
  isUndoing: boolean
  onTriageVerdict: (
    emailId: string,
    action: ApplyTriageVerdictInput["action"]
  ) => void
  page: number
  totalPages: number
  onPageChange: (page: number) => void
}) {
  const categories = useQuery(categoriesQuery())
  const categoryIconMap = useMemo(
    () => buildCategoryIconMap(categories.data),
    [categories.data]
  )

  return (
    <Card>
      <CardContent className="space-y-3">
        <div
          role="group"
          aria-label="Skipped email views"
          className="flex flex-wrap gap-2"
        >
          <Button
            type="button"
            size="sm"
            variant={withoutDraft ? "outline" : "default"}
            aria-pressed={!withoutDraft}
            onClick={() => onWithoutDraftChange(false)}
          >
            With a draft
          </Button>
          <Button
            type="button"
            size="sm"
            variant={withoutDraft ? "default" : "outline"}
            aria-pressed={withoutDraft}
            onClick={() => onWithoutDraftChange(true)}
          >
            Without a draft
          </Button>
        </div>
        {triageError && (
          <Alert variant="destructive">
            <AlertDescription>{triageError}</AlertDescription>
          </Alert>
        )}
        {isLoading ? (
          <div className="space-y-3" role="status">
            <span className="sr-only">Loading skipped emails</span>
            <Skeleton className="h-16 w-full" />
            <Skeleton className="h-16 w-full" />
          </div>
        ) : errorMessage ? (
          <Alert variant="destructive">
            <AlertDescription>{errorMessage}</AlertDescription>
          </Alert>
        ) : rows.length === 0 ? (
          <p
            className="py-8 text-center text-sm text-muted-foreground"
            role="status"
          >
            No skipped emails to review.
          </p>
        ) : (
          <ul className="space-y-2">
            {rows.map((ghost) => {
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
              const formattedAmount = withoutDraft
                ? null
                : formatMinorCurrency(ghost.amount_minor, ghost.currency)
              const isTriageBusy = pendingTriageVerdict !== null || isUndoing
              const isPromoting =
                pendingTriageVerdict?.emailId === ghost.email_id &&
                pendingTriageVerdict.action === "promote"
              const isIgnoring =
                pendingTriageVerdict?.emailId === ghost.email_id &&
                pendingTriageVerdict.action === "ignore"

              return (
                <li key={ghost.email_id}>
                  <div className="flex flex-col gap-3 rounded-lg border border-dashed border-input bg-muted/35 p-3 sm:flex-row sm:items-start sm:justify-between">
                    <div className="min-w-0 flex-1 space-y-1.5">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="truncate font-semibold text-foreground">
                          {ghostTitle}
                        </span>
                        {ghost.category && (
                          <CategoryBadge
                            category={ghost.category}
                            icon={categoryIconMap.get(ghost.category)}
                          />
                        )}
                      </div>
                      <p className="text-xs text-muted-foreground">
                        {ghost.account_name ??
                          (ghost.payment_method_raw
                            ? `Unassigned (${ghost.payment_method_raw})`
                            : "Unassigned")}{" "}
                        {ghost.occurred_at_utc && (
                          <>
                            ·{" "}
                            {formatUtcInTimezone(
                              ghost.occurred_at_utc,
                              timezone
                            )}{" "}
                          </>
                        )}
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
                          variant="outline"
                          disabled={isTriageBusy}
                          onClick={() => onCardPayment(ghost)}
                        >
                          Mark as card payment
                        </Button>
                        {!withoutDraft && (
                          <>
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
                          </>
                        )}
                      </div>
                    </div>
                  </div>
                </li>
              )
            })}
          </ul>
        )}
      </CardContent>

      {totalPages > 1 && (
        <CardFooter className="flex flex-wrap items-center justify-end gap-2 border-t pt-4 text-xs text-muted-foreground">
          <PaginationControls
            page={page}
            totalPages={totalPages}
            isLoading={isLoading}
            onPageChange={onPageChange}
          />
        </CardFooter>
      )}
    </Card>
  )
}
