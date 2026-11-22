"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useEffect, useMemo, useState, type FormEvent } from "react"

import { DatePicker } from "@/components/date-picker"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { reconcileAccountApi, type AccountSummary } from "@/lib/api"
import {
  dateTimeInputToUtcIso,
  formatDateString,
  formatMinorCurrency,
  formatSignedMinor,
  formatUtcInTimezone,
  getTodayDateString,
  minorToDecimalInput,
  parseDecimalToMinorClient,
} from "@/lib/format"

export function ReconcileAccountDialog({
  open,
  onOpenChange,
  account,
  timezone,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  account: AccountSummary | null
  timezone?: string | null
}) {
  const queryClient = useQueryClient()
  const [actualBalance, setActualBalance] = useState("")
  const [asOfDate, setAsOfDate] = useState("")
  const [note, setNote] = useState("")
  const [formError, setFormError] = useState<string | null>(null)

  const todayStr = getTodayDateString(timezone)
  const lastStatementDate =
    account?.type === "credit_card"
      ? (account.billing_cycle?.last_statement_date ?? null)
      : null

  useEffect(() => {
    if (!open) return
    setActualBalance("")
    setAsOfDate(getTodayDateString(timezone))
    setNote("")
    setFormError(null)
  }, [open, account, timezone])

  const effectiveAsOfDate = asOfDate || todayStr
  const isToday = effectiveAsOfDate === todayStr
  const isFutureDate = effectiveAsOfDate > todayStr

  const backdatedBeforeLatest = useMemo(() => {
    if (isToday || isFutureDate || !account?.latest_reconciliation) {
      return false
    }
    try {
      const targetUtc = dateTimeInputToUtcIso(
        effectiveAsOfDate,
        "23:59:59",
        timezone
      )
      return targetUtc < account.latest_reconciliation.recorded_at_utc
    } catch {
      return false
    }
  }, [isToday, isFutureDate, account, effectiveAsOfDate, timezone])

  const adjustmentPreviewMinor = useMemo(() => {
    if (
      !isToday ||
      !account ||
      !actualBalance.trim() ||
      account.computed_balance_minor === null
    ) {
      return null
    }
    try {
      const actualMinor = parseDecimalToMinorClient(
        actualBalance,
        account.currency
      )
      return actualMinor - account.computed_balance_minor
    } catch {
      return null
    }
  }, [isToday, account, actualBalance])

  const reconcileMutation = useMutation({
    mutationFn: reconcileAccountApi,
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: ["accounts"] })
      void queryClient.invalidateQueries({
        queryKey: ["accounts", "reconciliations", variables.account_id],
      })
      onOpenChange(false)
    },
    onError: (err) => {
      setFormError(
        err instanceof Error ? err.message : "Could not reconcile account."
      )
    },
  })

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setFormError(null)
    if (!account) return
    let actualMinor: number
    try {
      actualMinor = parseDecimalToMinorClient(actualBalance, account.currency)
    } catch (err) {
      setFormError(
        err instanceof Error ? err.message : "Enter a valid actual balance."
      )
      return
    }
    if (isFutureDate) {
      setFormError("Reconciliation date cannot be in the future.")
      return
    }
    let recordedAtUtc: string | undefined
    if (!isToday) {
      try {
        recordedAtUtc = dateTimeInputToUtcIso(
          effectiveAsOfDate,
          "23:59:59",
          timezone
        )
      } catch (err) {
        setFormError(
          err instanceof Error ? err.message : "Enter a valid as-of date."
        )
        return
      }
    }
    reconcileMutation.mutate({
      account_id: account.id,
      actual_balance_minor: actualMinor,
      note: note.trim() || null,
      ...(recordedAtUtc ? { recorded_at_utc: recordedAtUtc } : {}),
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Reconcile {account?.name ?? "account"}</DialogTitle>
          <DialogDescription>
            Record the actual balance from your bank or statement to anchor
            future calculations.
          </DialogDescription>
        </DialogHeader>

        {account && (
          <form onSubmit={handleSubmit} className="space-y-4">
            <Card>
              <CardContent className="py-3">
                <p className="text-xs text-muted-foreground">
                  {account.computed_balance_minor === null
                    ? "Starting balance"
                    : "Server-computed balance"}
                </p>
                <p className="font-semibold">
                  {account.computed_balance_minor === null
                    ? "Unknown — enter actual balance to set it"
                    : formatMinorCurrency(
                        account.computed_balance_minor,
                        account.currency
                      )}
                </p>
              </CardContent>
            </Card>

            <Field>
              <FieldLabel>As of date</FieldLabel>
              <DatePicker
                value={effectiveAsOfDate}
                onChange={setAsOfDate}
                ariaLabel="As of date"
              />
              {lastStatementDate && (
                <div className="flex flex-wrap gap-2 pt-1">
                  <Button
                    type="button"
                    size="sm"
                    variant={isToday ? "secondary" : "outline"}
                    onClick={() => setAsOfDate(todayStr)}
                  >
                    Today
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant={
                      effectiveAsOfDate === lastStatementDate
                        ? "secondary"
                        : "outline"
                    }
                    onClick={() => setAsOfDate(lastStatementDate)}
                  >
                    Last statement ({formatDateString(lastStatementDate)})
                  </Button>
                </div>
              )}
            </Field>

            {backdatedBeforeLatest && account.latest_reconciliation && (
              <Alert>
                <AlertDescription>
                  This date is earlier than your latest reconciliation (
                  {formatUtcInTimezone(
                    account.latest_reconciliation.recorded_at_utc,
                    timezone
                  )}
                  ). Saving records this historical entry, but the newer
                  reconciliation will remain your active balance anchor.
                </AlertDescription>
              </Alert>
            )}

            <Field>
              <FieldLabel>Actual balance ({account.currency})</FieldLabel>
              <Input
                type="text"
                inputMode="decimal"
                required
                value={actualBalance}
                onChange={(e) => setActualBalance(e.target.value)}
                placeholder={minorToDecimalInput(
                  account.computed_balance_minor ?? 0,
                  account.currency
                )}
              />
            </Field>

            {adjustmentPreviewMinor !== null && (
              <p className="text-sm">
                <span className="text-muted-foreground">Adjustment: </span>
                <span className="font-medium">
                  {formatSignedMinor(adjustmentPreviewMinor, account.currency)}
                </span>
              </p>
            )}

            {!isToday && !isFutureDate && (
              <p className="text-xs text-muted-foreground">
                Anchors at end of day ({formatDateString(effectiveAsOfDate)}).
                Subsequent transactions add to this balance, and the adjustment
                is calculated as of that date.
              </p>
            )}

            <Field>
              <FieldLabel>Note (optional)</FieldLabel>
              <Input
                type="text"
                value={note}
                onChange={(e) => setNote(e.target.value)}
                placeholder="e.g. Monthly statement check"
              />
            </Field>

            {formError && (
              <Alert variant="destructive">
                <AlertDescription>{formError}</AlertDescription>
              </Alert>
            )}

            <DialogFooter>
              <DialogClose render={<Button type="button" variant="outline" />}>
                Cancel
              </DialogClose>
              <Button type="submit" disabled={reconcileMutation.isPending}>
                {reconcileMutation.isPending
                  ? "Reconciling..."
                  : "Save reconciliation"}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}
