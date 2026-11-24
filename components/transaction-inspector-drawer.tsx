"use client"

import { ChevronDownIcon, MailIcon } from "lucide-react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type SubmitEvent,
} from "react"

import { DatePicker } from "@/components/date-picker"
import { DynamicIcon } from "@/components/dynamic-icon"
import {
  MagicEnrichButton,
  patchTagsQueryCache,
  resolveEnrichedDateTimeInputs,
  syncAndMergeEnrichedTags,
  useMagicEnrich,
} from "@/components/magic-enrich"
import { TagMultiSelect } from "@/components/tag-multi-select"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
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
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Drawer,
  DrawerBody,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@/components/ui/drawer"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsList, TabsPanel, TabsTab } from "@/components/ui/tabs"
import {
  categoriesQuery,
  deleteTransactionApi,
  invalidateTransactionRelatedQueries,
  markTransactionsReviewedApi,
  splitTransactionSourceApi,
  tagsQuery,
  transactionDetailQuery,
  updateTransactionApi,
  type AccountSummary,
  type TransactionDetailView,
  type TransactionListItem,
} from "@/lib/api"
import {
  DEFAULT_CATEGORY,
  buildCategoryIconMap,
  buildCategorySelectOptions,
} from "@/lib/categories"
import {
  dateTimeInputToUtcIso,
  formatMinorCurrency,
  formatUtcInTimezone,
  getCurrencyExponentClient,
  minorToDecimalInput,
  normalizeDecimalAmountString,
  parseDecimalToMinorClient,
  utcToDateInputValue,
  utcToTimeInputValue,
} from "@/lib/format"

const UNASSIGNED_ACCOUNT = "__unassigned__"

function groupAmountInput(value: string): string {
  const match = /^(-?\d+)(\.\d*)?$/.exec(value.trim())
  if (!match) return value
  return match[1].replace(/\B(?=(\d{3})+(?!\d))/g, ",") + (match[2] ?? "")
}

function InspectorBody({
  transactionId,
  accounts,
  localCurrency,
  timezone,
  onClose,
}: {
  transactionId: string
  accounts: AccountSummary[]
  localCurrency: string
  timezone: string | null | undefined
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const detailQuery = useQuery(transactionDetailQuery(transactionId))
  const catListQuery = useQuery(categoriesQuery())
  const tagListQuery = useQuery(tagsQuery())

  const [merchant, setMerchant] = useState("")
  const [amount, setAmount] = useState("")
  const [category, setCategory] = useState<string>(DEFAULT_CATEGORY)
  const [selectedTagIds, setSelectedTagIds] = useState<string[]>([])
  const [draftTagNames, setDraftTagNames] = useState<string[]>([])
  const [hasUnsavedMagicEdits, setHasUnsavedMagicEdits] = useState(false)
  const [accountId, setAccountId] = useState(UNASSIGNED_ACCOUNT)
  const [date, setDate] = useState("")
  const [time, setTime] = useState("")
  const [note, setNote] = useState("")
  const [convertToLocalCurrency, setConvertToLocalCurrency] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const [statusMessage, setStatusMessage] = useState<string | null>(null)

  const { isEnriching, abortEnrichment, runEnrichment } = useMagicEnrich()
  const lastLoadedTxRef = useRef<{ id: string; key: string } | null>(null)

  const syncFormFromTransaction = useCallback(
    (tx: TransactionListItem) => {
      const nextKey = `${tx.id}:${tx.updated_at_utc}|${timezone ?? ""}`
      lastLoadedTxRef.current = { id: tx.id, key: nextKey }
      setMerchant(tx.merchant)
      setAmount(
        groupAmountInput(minorToDecimalInput(tx.amount_minor, tx.currency))
      )
      setCategory(tx.category)
      setSelectedTagIds((tx.tags ?? []).map((t) => t.id))
      setDraftTagNames([])
      setHasUnsavedMagicEdits(false)
      setAccountId(tx.account_id ?? UNASSIGNED_ACCOUNT)
      setDate(utcToDateInputValue(tx.occurred_at_utc, timezone))
      setTime(utcToTimeInputValue(tx.occurred_at_utc, timezone))
      setNote(tx.note ?? "")
      setConvertToLocalCurrency(false)
    },
    [timezone]
  )

  useEffect(() => {
    const tx = detailQuery.data?.transaction
    if (!tx) return
    const nextKey = `${tx.id}:${tx.updated_at_utc}|${timezone ?? ""}`
    const isSameTx = lastLoadedTxRef.current?.id === tx.id
    if (
      isSameTx &&
      (hasUnsavedMagicEdits ||
        isEnriching ||
        lastLoadedTxRef.current?.key === nextKey)
    ) {
      return
    }
    if (!isSameTx) {
      abortEnrichment()
    }
    syncFormFromTransaction(tx)
  }, [
    detailQuery.data?.transaction,
    timezone,
    hasUnsavedMagicEdits,
    isEnriching,
    abortEnrichment,
    syncFormFromTransaction,
  ])

  const invalidateAll = () => {
    invalidateTransactionRelatedQueries(queryClient)
  }

  const updateMutation = useMutation({
    mutationFn: updateTransactionApi,
    onSuccess: (updatedTx) => {
      syncFormFromTransaction(updatedTx)
      queryClient.setQueryData<TransactionDetailView>(
        transactionDetailQuery(updatedTx.id).queryKey,
        (prev) => (prev ? { ...prev, transaction: updatedTx } : prev)
      )
      patchTagsQueryCache(
        queryClient,
        updatedTx.tags ?? [],
        updatedTx.updated_at_utc
      )
      setFormError(null)
      setStatusMessage("Transaction updated.")
      invalidateAll()
    },
    onError: (err) => {
      setFormError(
        err instanceof Error ? err.message : "Failed to update transaction."
      )
    },
  })

  const deleteMutation = useMutation({
    mutationFn: deleteTransactionApi,
    onSuccess: () => {
      invalidateAll()
      onClose()
    },
    onError: (err) => {
      setFormError(
        err instanceof Error ? err.message : "Failed to delete transaction."
      )
    },
  })

  const splitMutation = useMutation({
    mutationFn: splitTransactionSourceApi,
    onSuccess: () => {
      setFormError(null)
      setStatusMessage("Email split into its own transaction.")
      invalidateAll()
    },
    onError: (err) => {
      setFormError(
        err instanceof Error ? err.message : "Failed to split email source."
      )
    },
  })

  const markReviewedMutation = useMutation({
    mutationFn: () => markTransactionsReviewedApi([transactionId]),
    onSuccess: () => {
      setFormError(null)
      setStatusMessage("Marked as reviewed.")
      invalidateAll()
    },
    onError: (err) => {
      setFormError(
        err instanceof Error
          ? err.message
          : "Failed to mark transaction reviewed."
      )
    },
  })

  const detailTx = detailQuery.data?.transaction

  const categoryIconMap = useMemo(
    () => buildCategoryIconMap(catListQuery.data),
    [catListQuery.data]
  )

  const categoryOptions = useMemo(
    () => buildCategorySelectOptions(catListQuery.data, category),
    [catListQuery.data, category]
  )

  const detailAccountId = detailTx?.account_id
  const detailAccountName = detailTx?.account_name
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
        detailAccountId === accountId && detailAccountName
          ? detailAccountName
          : "Unknown account"
      base.push({ value: accountId, label: fallbackLabel })
    }
    return base
  }, [accounts, accountId, detailAccountId, detailAccountName])

  if (detailQuery.isPending) {
    return (
      <div className="space-y-3 py-4">
        <Skeleton className="h-9 w-full" />
        <Skeleton className="h-9 w-full" />
        <Skeleton className="h-24 w-full" />
      </div>
    )
  }

  const detail = detailQuery.data
  if (!detail) {
    return (
      <Alert variant="destructive">
        <AlertDescription>
          {detailQuery.error instanceof Error
            ? detailQuery.error.message
            : "Transaction not found."}
        </AlertDescription>
      </Alert>
    )
  }

  const tx = detail.transaction

  function handleSave(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    if (isEnriching) return
    setFormError(null)
    setStatusMessage(null)
    try {
      const targetCurrency =
        tx.isForeignCurrency && !convertToLocalCurrency
          ? tx.currency
          : localCurrency
      const initialAmountStr = minorToDecimalInput(tx.amount_minor, tx.currency)
      const amountChanged =
        normalizeDecimalAmountString(
          amount,
          getCurrencyExponentClient(tx.currency)
        ) !== initialAmountStr
      const parsedAmountMinor =
        !tx.isForeignCurrency || convertToLocalCurrency || amountChanged
          ? parseDecimalToMinorClient(amount, targetCurrency)
          : undefined

      const initialDateStr = utcToDateInputValue(tx.occurred_at_utc, timezone)
      const initialTimeStr = utcToTimeInputValue(tx.occurred_at_utc, timezone)
      const occurredAtUtc =
        date.trim() !== initialDateStr || time.trim() !== initialTimeStr
          ? dateTimeInputToUtcIso(date, time || "00:00", timezone)
          : undefined

      updateMutation.mutate({
        id: tx.id,
        merchant: merchant.trim(),
        amount_minor: parsedAmountMinor,
        category,
        tag_ids: selectedTagIds,
        tag_names: draftTagNames,
        account_id: accountId !== UNASSIGNED_ACCOUNT ? accountId : null,
        occurred_at_utc: occurredAtUtc,
        note: note.trim() || null,
        convertToLocalCurrency: tx.isForeignCurrency
          ? convertToLocalCurrency
          : undefined,
      })
    } catch (err) {
      setFormError(err instanceof Error ? err.message : "Invalid input.")
    }
  }

  function handleMagicEnrich() {
    runEnrichment({
      transactionIds: [tx.id],
      onStart: () => {
        setFormError(null)
        setStatusMessage(null)
      },
      onSuccess: (preview) => {
        setMerchant(preview.merchant)
        setCategory(preview.category)
        setNote(preview.note ?? "")
        const nextDateTime = resolveEnrichedDateTimeInputs(
          preview.occurred_at_utc,
          timezone
        )
        if (nextDateTime) {
          setDate(nextDateTime.date)
          setTime(nextDateTime.time)
        }
        const { nextSelectedTagIds, nextDraftTagNames } =
          syncAndMergeEnrichedTags({
            queryClient,
            preview,
            currentSelectedTagIds: selectedTagIds,
            fallbackKnownTags: tagListQuery.data ?? [],
          })
        setSelectedTagIds(nextSelectedTagIds)
        setDraftTagNames(nextDraftTagNames)
        setHasUnsavedMagicEdits(true)
        setStatusMessage(
          "Suggested fields populated from attached emails. Review and save changes."
        )
      },
      onError: (msg) => {
        setFormError(msg)
      },
    })
  }

  return (
    <div className="space-y-5">
      <div className="rounded-xl border bg-secondary/60 p-4">
        <p className="text-sm font-medium break-words text-secondary-foreground">
          {tx.merchant}
        </p>
        <p className="mt-1 text-2xl font-semibold tracking-tight text-foreground tabular-nums">
          {formatMinorCurrency(tx.amount_minor, tx.currency)}
        </p>
        <p className="mt-1 text-xs text-muted-foreground">
          {formatUtcInTimezone(tx.occurred_at_utc, timezone)}
        </p>
      </div>
      {formError && (
        <Alert variant="destructive">
          <AlertDescription>{formError}</AlertDescription>
        </Alert>
      )}
      {statusMessage && (
        <Alert>
          <AlertDescription>{statusMessage}</AlertDescription>
        </Alert>
      )}

      <Tabs defaultValue="details">
        <TabsList aria-label="Transaction detail sections">
          <TabsTab value="details">Details</TabsTab>
          <TabsTab value="sources">
            <MailIcon className="size-3.5" /> Source emails (
            {detail.sources.length})
          </TabsTab>
        </TabsList>
        <TabsPanel value="details" keepMounted>
          <form onSubmit={handleSave} className="space-y-4">
            {detail.sources.length >= 1 && (
              <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border bg-muted/40 px-3 py-2">
                <p className="text-xs text-muted-foreground">
                  Fill merchant, category, tags, date, time, and note from{" "}
                  {detail.sources.length}{" "}
                  {detail.sources.length === 1
                    ? "attached email"
                    : "attached emails"}
                  .
                </p>
                <MagicEnrichButton
                  isEnriching={isEnriching}
                  disabled={
                    updateMutation.isPending || deleteMutation.isPending
                  }
                  onClick={handleMagicEnrich}
                />
              </div>
            )}

            <div className="grid gap-4 sm:grid-cols-2">
              <Field>
                <FieldLabel>Merchant</FieldLabel>
                <Input
                  type="text"
                  required
                  value={merchant}
                  onChange={(e) => setMerchant(e.target.value)}
                />
              </Field>

              <Field>
                <FieldLabel>
                  Amount (
                  {tx.isForeignCurrency && !convertToLocalCurrency
                    ? tx.currency
                    : localCurrency}
                  )
                </FieldLabel>
                <Input
                  type="text"
                  inputMode="decimal"
                  required
                  value={amount}
                  onBlur={() => setAmount(groupAmountInput(amount))}
                  className="tabular-nums"
                  onChange={(e) => setAmount(e.target.value)}
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

              <Field className="sm:col-span-2">
                <FieldLabel>Tags</FieldLabel>
                <TagMultiSelect
                  tags={tagListQuery.data ?? []}
                  selectedTagIds={selectedTagIds}
                  onChange={setSelectedTagIds}
                  draftTagNames={draftTagNames}
                  onDraftTagNamesChange={setDraftTagNames}
                  disabled={isEnriching || updateMutation.isPending}
                />
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
                {tx.payment_method_raw && (
                  <FieldDescription>
                    Extracted payment method: {tx.payment_method_raw}
                  </FieldDescription>
                )}
              </Field>

              <Field>
                <FieldLabel>Occurred date</FieldLabel>
                <DatePicker
                  value={date}
                  onChange={setDate}
                  ariaLabel="Occurred date"
                />
              </Field>

              <Field>
                <FieldLabel>Occurred time</FieldLabel>
                <Input
                  type="time"
                  required
                  value={time}
                  onChange={(e) => setTime(e.target.value)}
                />
              </Field>

              <Field className="sm:col-span-2">
                <FieldLabel>Note</FieldLabel>
                <Input
                  type="text"
                  value={note}
                  onChange={(e) => setNote(e.target.value)}
                />
              </Field>
            </div>

            {tx.isForeignCurrency && (
              <div className="flex items-center gap-2">
                <Checkbox
                  id="convert-local"
                  checked={convertToLocalCurrency}
                  onCheckedChange={(checked) =>
                    setConvertToLocalCurrency(Boolean(checked))
                  }
                />
                <Label htmlFor="convert-local">
                  Convert to local currency ({localCurrency})
                </Label>
              </div>
            )}

            <div className="flex flex-wrap items-center justify-between gap-3 pt-2">
              <AlertDialog>
                <AlertDialogTrigger
                  render={
                    <Button type="button" variant="destructive" size="sm" />
                  }
                >
                  Delete transaction
                </AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>
                      Delete this transaction?
                    </AlertDialogTitle>
                    <AlertDialogDescription>
                      This permanently deletes the transaction and marks any
                      linked source emails as dismissed.
                    </AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>Cancel</AlertDialogCancel>
                    <AlertDialogAction
                      variant="destructive"
                      disabled={deleteMutation.isPending}
                      onClick={() => deleteMutation.mutate(tx.id)}
                    >
                      Delete forever
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>

              <div className="flex flex-wrap items-center gap-2">
                {tx.reviewed === 1 ? (
                  <Badge variant="secondary">Reviewed</Badge>
                ) : !hasUnsavedMagicEdits ? (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={markReviewedMutation.isPending || isEnriching}
                    onClick={() => markReviewedMutation.mutate()}
                  >
                    {markReviewedMutation.isPending
                      ? "Marking..."
                      : "Mark reviewed"}
                  </Button>
                ) : null}
                <Button
                  type="submit"
                  disabled={updateMutation.isPending || isEnriching}
                >
                  {updateMutation.isPending ? "Saving..." : "Save changes"}
                </Button>
              </div>
            </div>
          </form>
        </TabsPanel>
        <TabsPanel value="sources" className="space-y-3">
          {tx.manual_override === 1 && detail.sources.length > 1 && (
            <Alert>
              <AlertDescription>
                This transaction has a locked amount (edited or combined).
                Splitting an email creates a separate transaction without
                reducing this transaction&apos;s amount — adjust the amount
                manually if needed.
              </AlertDescription>
            </Alert>
          )}
          {detail.sources.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No source emails attached (manual transaction).
            </p>
          ) : (
            <ul className="space-y-3">
              {detail.sources.map((source) => (
                <li key={source.id}>
                  <Card>
                    <CardContent className="space-y-2 py-3 text-sm">
                      <details className="group">
                        <summary className="flex cursor-pointer list-none items-start gap-3 rounded outline-none focus-visible:ring-2 focus-visible:ring-ring [&::-webkit-details-marker]:hidden">
                          <div className="min-w-0 flex-1 break-words">
                            <p className="font-medium">
                              {source.subject || "Untitled email"}
                            </p>
                            <p className="mt-1 text-xs text-muted-foreground">
                              {source.from_address} ·{" "}
                              {formatUtcInTimezone(
                                source.received_at_utc,
                                timezone
                              )}
                            </p>
                          </div>
                          <ChevronDownIcon className="mt-0.5 size-4 shrink-0 text-muted-foreground transition-transform group-open:rotate-180" />
                        </summary>
                        <div className="mt-3 space-y-3">
                          <p className="text-xs text-muted-foreground">
                            Role: {source.role} · State: {source.match_state}
                          </p>
                          <pre className="max-h-64 overflow-auto rounded-md border bg-muted p-3 font-mono text-xs break-words whitespace-pre-wrap">
                            {source.cleaned_text}
                          </pre>
                          {detail.sources.length > 1 && (
                            <Button
                              type="button"
                              variant="outline"
                              size="sm"
                              disabled={splitMutation.isPending || isEnriching}
                              onClick={() =>
                                splitMutation.mutate({
                                  transactionId: tx.id,
                                  emailId: source.email_id,
                                })
                              }
                            >
                              Split email
                            </Button>
                          )}
                        </div>
                      </details>
                    </CardContent>
                  </Card>
                </li>
              ))}
            </ul>
          )}
        </TabsPanel>
      </Tabs>
    </div>
  )
}

export function TransactionInspectorDrawer({
  transactionId,
  accounts,
  localCurrency,
  timezone,
  onClose,
}: {
  transactionId: string | null
  accounts: AccountSummary[]
  localCurrency: string
  timezone: string | null | undefined
  onClose: () => void
}) {
  return (
    <Drawer
      open={transactionId !== null}
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <DrawerContent>
        <DrawerHeader>
          <DrawerTitle>Transaction details</DrawerTitle>
          <DrawerDescription>
            Inspect attached source emails, mark reviewed, or edit transaction
            fields.
          </DrawerDescription>
        </DrawerHeader>
        <DrawerBody>
          {transactionId && (
            <InspectorBody
              key={transactionId}
              transactionId={transactionId}
              accounts={accounts}
              localCurrency={localCurrency}
              timezone={timezone}
              onClose={onClose}
            />
          )}
        </DrawerBody>
      </DrawerContent>
    </Drawer>
  )
}
