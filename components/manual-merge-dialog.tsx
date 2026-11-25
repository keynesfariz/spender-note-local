"use client"

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useEffect, useId, useMemo, useState, type SubmitEvent } from "react"

import { DatePicker } from "@/components/date-picker"
import { DynamicIcon } from "@/components/dynamic-icon"
import {
  MagicEnrichButton,
  resolveMagicDraft,
  useMagicEnrich,
} from "@/components/magic-enrich"
import { TagMultiSelect } from "@/components/tag-multi-select"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
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
import { Textarea } from "@/components/ui/textarea"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { categoriesQuery } from "@/lib/api/categories"
import { tagsQuery } from "@/lib/api/tags"
import {
  combineTransactionsApi,
  duplicateMergeTransactionsApi,
  invalidateTransactionRelatedQueries,
} from "@/lib/api/transactions"
import {
  type AccountOption,
  type TransactionListItem,
} from "@/lib/api/types.generated"
import {
  DEFAULT_CATEGORY,
  buildCategoryIconMap,
  buildCategorySelectOptions,
} from "@/lib/categories"
import {
  dateTimeInputToUtcIso,
  formatMinorCurrency,
  formatUtcInTimezone,
  utcToDateInputValue,
  utcToTimeInputValue,
} from "@/lib/format"
import { cn } from "@/lib/utils"

const UNASSIGNED_ACCOUNT = "__unassigned__"

export type ManualMergeMode = "duplicate" | "combine"

export function ManualMergeDialog({
  mode,
  transactions,
  accounts,
  timezone,
  onClose,
  onSuccess,
}: {
  mode: ManualMergeMode | null
  transactions: TransactionListItem[]
  accounts: AccountOption[]
  timezone: string | null | undefined
  onClose: () => void
  onSuccess: () => void
}) {
  const queryClient = useQueryClient()
  const catListQuery = useQuery({
    ...categoriesQuery(),
    enabled: mode === "combine",
  })
  const tagListQuery = useQuery({
    ...tagsQuery(),
    enabled: mode === "combine",
  })

  const [keepId, setKeepId] = useState<string>("")
  const [remark, setRemark] = useState("")
  const [merchant, setMerchant] = useState("")
  const [remarkUserEdited, setRemarkUserEdited] = useState(false)
  const remarkId = useId()
  const noteId = useId()
  const [category, setCategory] = useState<string>(DEFAULT_CATEGORY)
  const [selectedTagIds, setSelectedTagIds] = useState<string[]>([])
  const [draftTagNames, setDraftTagNames] = useState<string[]>([])
  const [tagCreatePending, setTagCreatePending] = useState(false)
  const [accountId, setAccountId] = useState(UNASSIGNED_ACCOUNT)
  const [date, setDate] = useState("")
  const [time, setTime] = useState("00:00")
  const [note, setNote] = useState("")
  const [formError, setFormError] = useState<string | null>(null)
  const [statusMessage, setStatusMessage] = useState<string | null>(null)

  const { isEnriching, abortEnrichment, runEnrichment } = useMagicEnrich()

  const sortedByOccurred = useMemo(
    () =>
      mode === "combine"
        ? transactions.toSorted((a, b) =>
            b.occurred_at_utc.localeCompare(a.occurred_at_utc)
          )
        : transactions,
    [mode, transactions]
  )

  const hasSourceEmails = useMemo(
    () => mode === "combine" && transactions.some((tx) => tx.sourceCount > 0),
    [mode, transactions]
  )

  const summedAmount = useMemo(
    () => transactions.reduce((sum, tx) => sum + tx.amount_minor, 0),
    [transactions]
  )
  const sharedCurrency = transactions[0]?.currency ?? "USD"

  const categoryIconMap = useMemo(
    () => buildCategoryIconMap(catListQuery.data),
    [catListQuery.data]
  )

  const categoryOptions = useMemo(
    () => buildCategorySelectOptions(catListQuery.data, category),
    [catListQuery.data, category]
  )

  const primaryAccountId = sortedByOccurred[0]?.account_id ?? null
  const primaryAccountName = sortedByOccurred[0]?.account_name ?? null
  const accountItems = useMemo(() => {
    const base = [
      { value: UNASSIGNED_ACCOUNT, label: "Unassigned" },
      ...accounts.map((acct) => ({ value: acct.id, label: acct.name })),
    ]
    if (
      accountId !== UNASSIGNED_ACCOUNT &&
      !accounts.some((acct) => acct.id === accountId)
    ) {
      const fallbackLabel =
        primaryAccountId === accountId && primaryAccountName
          ? primaryAccountName
          : "Unknown account"
      base.push({ value: accountId, label: fallbackLabel })
    }
    return base
  }, [accounts, accountId, primaryAccountId, primaryAccountName])

  const [previousSelectionKey, setPreviousSelectionKey] = useState<
    string | null
  >(null)
  const selectionKey =
    mode && transactions.length >= 2
      ? `${mode}:${transactions
          .map((tx) => tx.id)
          .sort()
          .join(",")}`
      : null

  if (previousSelectionKey !== selectionKey) {
    setPreviousSelectionKey(selectionKey)
    setTagCreatePending(false)
    setFormError(null)
    setStatusMessage(null)
    setKeepId("")
    setDraftTagNames((prev) => (prev.length === 0 ? prev : []))

    if (mode === "combine" && selectionKey) {
      const seenTagIds = new Set<string>()
      const unionTagIds: string[] = []
      for (const tx of sortedByOccurred) {
        for (const tag of tx.tags ?? []) {
          if (!seenTagIds.has(tag.id)) {
            seenTagIds.add(tag.id)
            unionTagIds.push(tag.id)
          }
        }
      }
      setSelectedTagIds(unionTagIds)

      const primary = sortedByOccurred[0]
      if (primary) {
        setRemark(primary.remark ?? primary.merchant)
        setMerchant(primary.merchant)
        setRemarkUserEdited(primary.remark_user_edited)
        setCategory(primary.category)
        setAccountId(primary.account_id ?? UNASSIGNED_ACCOUNT)
        setDate(utcToDateInputValue(primary.occurred_at_utc, timezone))
        setTime(
          utcToTimeInputValue(primary.occurred_at_utc, timezone) || "00:00"
        )
        const combinedNotes = sortedByOccurred
          .map((tx) => tx.note?.trim())
          .filter((n): n is string => Boolean(n))
          .join(" / ")
        setNote(combinedNotes)
      }
    } else {
      setSelectedTagIds([])
    }
  }

  useEffect(() => abortEnrichment, [selectionKey, abortEnrichment])

  const invalidateAll = () => {
    invalidateTransactionRelatedQueries(queryClient)
  }

  const duplicateMutation = useMutation({
    mutationFn: duplicateMergeTransactionsApi,
    onSuccess: () => {
      invalidateAll()
      onSuccess()
      onClose()
    },
    onError: (err) => {
      setFormError(
        err instanceof Error
          ? err.message
          : "Failed to merge duplicate transactions."
      )
    },
  })

  const combineMutation = useMutation({
    mutationFn: combineTransactionsApi,
    onSuccess: () => {
      invalidateAll()
      onSuccess()
      onClose()
    },
    onError: (err) => {
      setFormError(
        err instanceof Error ? err.message : "Failed to combine transactions."
      )
    },
  })

  const isPending = duplicateMutation.isPending || combineMutation.isPending

  const handleMagicEnrich = () => {
    if (mode !== "combine" || !hasSourceEmails || tagCreatePending) return
    runEnrichment({
      transactionIds: transactions.map((tx) => tx.id),
      onStart: () => {
        setFormError(null)
        setStatusMessage(null)
      },
      onSuccess: (preview) => {
        const { next, changed, message } = resolveMagicDraft(
          {
            remark,
            merchant,
            remarkUserEdited,
            category,
            date,
            time,
            note,
            selectedTagIds,
            draftTagNames,
          },
          preview,
          queryClient,
          tagListQuery.data ?? [],
          timezone
        )

        setRemarkUserEdited(next.remarkUserEdited)
        if (changed.has("Remark")) setRemark(next.remark)
        setMerchant(next.merchant)
        setCategory(next.category)
        if (changed.has("Note")) setNote(next.note)
        setDate(next.date)
        setTime(next.time)
        if (changed.has("Tags")) {
          setSelectedTagIds(next.selectedTagIds)
          setDraftTagNames(next.draftTagNames)
        }
        setStatusMessage(message)
      },
      onError: (msg) => {
        setStatusMessage(null)
        setFormError(msg)
      },
    })
  }

  const handleSubmit = (e: SubmitEvent<HTMLFormElement>) => {
    e.preventDefault()
    if (isEnriching || isPending || tagCreatePending) return
    setFormError(null)
    setStatusMessage(null)

    if (mode === "duplicate") {
      if (transactions.length !== 2 || !keepId) {
        setFormError("Choose which transaction to keep.")
        return
      }
      const dropTx = transactions.find((tx) => tx.id !== keepId)
      if (!dropTx) {
        setFormError("Could not determine the duplicate transaction to remove.")
        return
      }
      duplicateMutation.mutate({
        keepTransactionId: keepId,
        dropTransactionId: dropTx.id,
      })
      return
    }

    if (mode === "combine") {
      if (!date.trim()) {
        setFormError("Transaction date is required.")
        return
      }
      if (!time.trim()) {
        setFormError("Transaction time is required.")
        return
      }
      let occurredAtUtc: string
      try {
        occurredAtUtc = dateTimeInputToUtcIso(date, time, timezone)
      } catch (err) {
        setFormError(err instanceof Error ? err.message : "Invalid date.")
        return
      }
      combineMutation.mutate({
        transactionIds: transactions.map((tx) => tx.id),
        remark: remark.trim(),
        merchant,
        remark_user_edited: remarkUserEdited,
        category,
        tag_ids: selectedTagIds,
        tag_names: draftTagNames,
        account_id: accountId === UNASSIGNED_ACCOUNT ? null : accountId,
        occurred_at_utc: occurredAtUtc,
        note: note.trim() || null,
      })
    }
  }

  const canSubmit =
    mode === "duplicate"
      ? Boolean(keepId) && transactions.length === 2 && !isPending
      : Boolean(date.trim()) &&
        Boolean(time.trim()) &&
        transactions.length >= 2 &&
        !isPending &&
        !tagCreatePending &&
        !isEnriching

  return (
    <Dialog
      open={mode !== null}
      onOpenChange={(open) => {
        if (!open && !isPending) onClose()
      }}
    >
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {mode === "duplicate" ? "Duplicate merge" : "Combine transactions"}
          </DialogTitle>
          <DialogDescription>
            {mode === "duplicate"
              ? "Choose which transaction to keep. The other transaction will be deleted, and all linked emails from both will stay attached."
              : `Combining ${transactions.length} transactions into one entry with their summed amount and all linked emails preserved.`}
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4">
          {mode === "duplicate" && (
            <div
              role="radiogroup"
              aria-label="Choose transaction to keep"
              className="space-y-2"
            >
              {transactions.map((tx) => {
                const selected = keepId === tx.id
                return (
                  <label
                    key={tx.id}
                    className={cn(
                      "flex cursor-pointer items-start gap-3 rounded-lg border p-3 text-sm transition-colors",
                      selected
                        ? "border-primary bg-primary/5 ring-1 ring-primary"
                        : "hover:bg-muted/50"
                    )}
                  >
                    <input
                      type="radio"
                      name="keep-transaction"
                      value={tx.id}
                      checked={selected}
                      onChange={() => setKeepId(tx.id)}
                      className="mt-1 size-4 accent-primary"
                    />
                    <div className="min-w-0 flex-1 space-y-1">
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <span className="truncate font-medium">
                          {tx.remark ?? tx.merchant}
                        </span>
                        <span className="font-medium tabular-nums">
                          {formatMinorCurrency(tx.amount_minor, tx.currency)}
                        </span>
                      </div>
                      <div className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
                        <Badge variant="secondary">{tx.category}</Badge>
                        <Badge variant="outline">
                          {tx.sourceCount > 0
                            ? `${tx.sourceCount} ${tx.sourceCount === 1 ? "email" : "emails"}`
                            : "Manual"}
                        </Badge>
                        <span>
                          {tx.account_name ?? "Unassigned"} ·{" "}
                          {formatUtcInTimezone(tx.occurred_at_utc, timezone)}
                        </span>
                      </div>
                      {tx.note && (
                        <p className="text-xs text-muted-foreground">
                          Note: {tx.note}
                        </p>
                      )}
                    </div>
                  </label>
                )
              })}
            </div>
          )}

          {mode === "combine" && (
            <div className="space-y-4">
              <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border bg-muted/40 p-3">
                <div>
                  <p className="text-xs text-muted-foreground">
                    Combined total ({transactions.length} transactions)
                  </p>
                  <p className="text-2xl font-semibold tabular-nums">
                    {formatMinorCurrency(summedAmount, sharedCurrency)}
                  </p>
                </div>
                {hasSourceEmails && (
                  <MagicEnrichButton
                    isEnriching={isEnriching}
                    disabled={isPending || tagCreatePending}
                    onClick={handleMagicEnrich}
                  />
                )}
              </div>

              <div className="grid gap-4 sm:grid-cols-2" inert={isEnriching}>
                <Field className="sm:col-span-2">
                  <FieldLabel htmlFor={remarkId}>Remark</FieldLabel>
                  <Input
                    id={remarkId}
                    value={remark}
                    onChange={(e) => {
                      const value = e.target.value
                      const primary = sortedByOccurred[0]
                      setRemarkUserEdited(
                        Boolean(primary?.remark_user_edited) ||
                          value.trim() !==
                            (primary?.remark ?? primary?.merchant ?? "").trim()
                      )
                      setRemark(value)
                    }}
                  />
                </Field>

                <Field>
                  <FieldLabel>Category</FieldLabel>
                  <Select
                    items={categoryOptions}
                    value={category}
                    onValueChange={(v) => setCategory(v ?? DEFAULT_CATEGORY)}
                  >
                    <SelectTrigger className="w-full" aria-label="Category">
                      <DynamicIcon
                        name={categoryIconMap.get(category)}
                        className="size-4 text-muted-foreground"
                      />
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {categoryOptions.map((opt) => (
                        <SelectItem key={opt.value} value={opt.value}>
                          <span className="flex items-center gap-2">
                            <DynamicIcon
                              name={categoryIconMap.get(opt.value)}
                              className="size-4 text-muted-foreground"
                            />
                            <span>{opt.label}</span>
                          </span>
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </Field>

                <Field>
                  <FieldLabel>Account</FieldLabel>
                  <Select
                    items={accountItems}
                    value={accountId}
                    onValueChange={(v) => setAccountId(v ?? UNASSIGNED_ACCOUNT)}
                  >
                    <SelectTrigger className="w-full" aria-label="Account">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {accountItems.map((item) => (
                        <SelectItem key={item.value} value={item.value}>
                          {item.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </Field>

                <Field className="sm:col-span-2">
                  <FieldLabel>Tags</FieldLabel>
                  <TagMultiSelect
                    key={selectionKey}
                    tags={tagListQuery.data ?? []}
                    selectedTagIds={selectedTagIds}
                    onChange={setSelectedTagIds}
                    draftTagNames={draftTagNames}
                    onDraftTagNamesChange={setDraftTagNames}
                    onCreatePendingChange={setTagCreatePending}
                    disabled={isEnriching || isPending}
                  />
                </Field>

                <Field>
                  <FieldLabel>Date</FieldLabel>
                  <DatePicker
                    value={date}
                    onChange={setDate}
                    ariaLabel="Combined transaction date"
                  />
                </Field>

                <Field>
                  <FieldLabel>Time</FieldLabel>
                  <Input
                    type="time"
                    required
                    aria-label="Combined transaction time"
                    value={time}
                    onChange={(e) => setTime(e.target.value)}
                  />
                </Field>

                <Field className="sm:col-span-2">
                  <FieldLabel htmlFor={noteId}>Note</FieldLabel>
                  <Textarea
                    id={noteId}
                    rows={4}
                    placeholder="Optional note"
                    value={note}
                    onChange={(e) => setNote(e.target.value)}
                  />
                </Field>
              </div>
            </div>
          )}

          <div role="status" className="empty:absolute">
            {statusMessage && (
              <Alert role="presentation">
                <AlertDescription>{statusMessage}</AlertDescription>
              </Alert>
            )}
          </div>

          {formError && (
            <Alert variant="destructive">
              <AlertDescription>{formError}</AlertDescription>
            </Alert>
          )}

          <DialogFooter>
            <DialogClose
              render={
                <Button type="button" variant="outline" disabled={isPending} />
              }
            >
              Cancel
            </DialogClose>
            <Button type="submit" disabled={!canSubmit}>
              {isPending
                ? mode === "duplicate"
                  ? "Merging..."
                  : "Combining..."
                : mode === "duplicate"
                  ? "Merge duplicate"
                  : "Combine transactions"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
