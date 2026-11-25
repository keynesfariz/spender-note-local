"use client"

import { ChevronDownIcon, MailIcon } from "lucide-react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  useCallback,
  useEffect,
  useMemo,
  useId,
  useRef,
  useState,
  type SubmitEvent,
} from "react"

import { DatePicker } from "@/components/date-picker"
import { DynamicIcon } from "@/components/dynamic-icon"
import {
  MagicEnrichButton,
  patchTagsQueryCache,
  resolveMagicDraft,
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
import { Textarea } from "@/components/ui/textarea"
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
import { categoriesQuery } from "@/lib/api/categories"
import { tagsQuery } from "@/lib/api/tags"
import {
  deleteTransactionApi,
  invalidateTransactionRelatedQueries,
  markTransactionsReviewedApi,
  splitTransactionSourceApi,
  updateTransactionApi,
} from "@/lib/api/transactions"
import {
  type AccountOption,
  type CategorySummary,
  type TagSummary,
  type TransactionInspectorSource,
  type TransactionInspectorView,
  type TransactionListItem,
} from "@/lib/api/types.generated"
import {
  transactionInspectorQuery,
  transactionSourceBodyQuery,
} from "@/lib/api/transaction-inspector"
import {
  DEFAULT_CATEGORY,
  buildCategoryIconMap,
  buildCategorySelectOptions,
} from "@/lib/categories"
import {
  dateTimeInputToUtcIso,
  formatMinorCurrency,
  formatUtcInTimezone,
  minorToDecimalInput,
  parseDecimalToMinorClient,
  utcToDateInputValue,
  utcToTimeInputValue,
} from "@/lib/format"

const UNASSIGNED_ACCOUNT = "__unassigned__"

type DraftField =
  | "remark"
  | "merchant"
  | "amount"
  | "category"
  | "tags"
  | "account"
  | "date"
  | "note"
  | "conversion"

function groupAmountInput(value: string): string {
  const match = /^(-?\d+)(\.\d*)?$/.exec(value.trim())
  if (!match) return value
  return match[1].replace(/\B(?=(\d{3})+(?!\d))/g, ",") + (match[2] ?? "")
}

function SourceEmailBody({
  transactionId,
  emailId,
}: {
  transactionId: string
  emailId: string
}) {
  const body = useQuery(transactionSourceBodyQuery(transactionId, emailId))

  if (body.isPending) {
    return (
      <p role="status" className="text-xs text-muted-foreground">
        Loading email…
      </p>
    )
  }

  if (body.isError) {
    return (
      <div className="space-y-2">
        <p role="alert" className="text-xs text-destructive">
          {body.error.message}
        </p>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => void body.refetch()}
        >
          Retry
        </Button>
      </div>
    )
  }

  return (
    <pre className="max-h-64 overflow-auto rounded-md border bg-muted p-3 font-mono text-xs wrap-break-word whitespace-pre-wrap">
      {body.data.cleaned_text}
    </pre>
  )
}

function SourceEmail({
  source,
  transactionId,
  timezone,
  onSplit,
  splitDisabled,
}: {
  source: TransactionInspectorSource
  transactionId: string
  timezone: string | null | undefined
  onSplit?: () => void
  splitDisabled: boolean
}) {
  const [open, setOpen] = useState(false)

  return (
    <Card>
      <CardContent className="space-y-2 py-3 text-sm">
        <details
          className="group"
          onToggle={(event) => setOpen(event.currentTarget.open)}
        >
          <summary className="flex cursor-pointer list-none items-start gap-3 rounded outline-none focus-visible:ring-2 focus-visible:ring-ring [&::-webkit-details-marker]:hidden">
            <div className="min-w-0 flex-1 wrap-break-word">
              <p className="font-medium">
                {source.subject || "Untitled email"}
              </p>
              <p className="mt-1 text-xs text-muted-foreground">
                {source.from_address} ·{" "}
                {formatUtcInTimezone(source.received_at_utc, timezone)}
              </p>
            </div>
            <span className="mt-0.5 size-4 shrink-0 text-muted-foreground transition-transform group-open:rotate-180">
              <ChevronDownIcon className="size-4" aria-hidden />
            </span>
          </summary>
          <div className="mt-3 space-y-3">
            <p className="text-xs text-muted-foreground">
              Role: {source.role} · State: {source.match_state}
            </p>
            {open ? (
              <SourceEmailBody
                transactionId={transactionId}
                emailId={source.email_id}
              />
            ) : null}
            {onSplit ? (
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={splitDisabled}
                onClick={onSplit}
              >
                Split email
              </Button>
            ) : null}
          </div>
        </details>
      </CardContent>
    </Card>
  )
}

type InspectorProps = {
  transactionId: string
  accounts: AccountOption[]
  localCurrency: string
  timezone: string | null | undefined
  onClose: () => void
  onTransactionMutated?: () => void
}

function InspectorBody(props: InspectorProps) {
  const detailQuery = useQuery(transactionInspectorQuery(props.transactionId))
  const { data: categories } = useQuery(categoriesQuery())
  const { data: tags } = useQuery(tagsQuery())
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

  return (
    <InspectorForm
      {...props}
      detail={detail}
      categories={categories}
      tags={tags}
    />
  )
}

function InspectorForm({
  transactionId,
  accounts,
  localCurrency,
  timezone,
  onClose,
  onTransactionMutated,
  detail,
  categories,
  tags,
}: InspectorProps & {
  detail: TransactionInspectorView
  categories?: CategorySummary[]
  tags?: TagSummary[]
}) {
  const queryClient = useQueryClient()

  const tx = detail.transaction
  const [remark, setRemark] = useState(tx.remark ?? tx.merchant)
  const [merchant, setMerchant] = useState(tx.merchant)
  const [remarkUserEdited, setRemarkUserEdited] = useState(
    tx.remark_user_edited
  )
  const noteId = useId()
  const detailFormId = useId()
  const remarkId = useId()
  const [amount, setAmount] = useState(() =>
    groupAmountInput(minorToDecimalInput(tx.amount_minor, tx.currency))
  )
  const [category, setCategory] = useState(tx.category)
  const [selectedTagIds, setSelectedTagIds] = useState(() =>
    (tx.tags ?? []).map((tag) => tag.id)
  )
  const [draftTagNames, setDraftTagNames] = useState<string[]>([])
  const [hasUnsavedMagicEdits, setHasUnsavedMagicEdits] = useState(false)
  const [accountId, setAccountId] = useState(
    tx.account_id ?? UNASSIGNED_ACCOUNT
  )
  const [date, setDate] = useState(() =>
    utcToDateInputValue(tx.occurred_at_utc, timezone)
  )
  const [time, setTime] = useState(() =>
    utcToTimeInputValue(tx.occurred_at_utc, timezone)
  )
  const [note, setNote] = useState(tx.note ?? "")
  const [convertToLocalCurrency, setConvertToLocalCurrency] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const [statusMessage, setStatusMessage] = useState<string | null>(null)

  const { isEnriching, runEnrichment } = useMagicEnrich()
  const lastLoadedTxRef = useRef({
    key: `${tx.updated_at_utc}|${timezone ?? ""}`,
    currency: tx.currency,
    date,
    time,
    timezone,
  })
  const editedFields = useRef(new Set<DraftField>())

  const syncFormFromTransaction = useCallback(
    (tx: TransactionListItem) => {
      const nextKey = `${tx.updated_at_utc}|${timezone ?? ""}`
      const date = utcToDateInputValue(tx.occurred_at_utc, timezone)
      const time = utcToTimeInputValue(tx.occurred_at_utc, timezone)
      lastLoadedTxRef.current = {
        key: nextKey,
        currency: tx.currency,
        date,
        time,
        timezone,
      }
      editedFields.current.clear()
      setRemark(tx.remark ?? tx.merchant)
      setMerchant(tx.merchant)
      setRemarkUserEdited(tx.remark_user_edited)
      setAmount(
        groupAmountInput(minorToDecimalInput(tx.amount_minor, tx.currency))
      )
      setCategory(tx.category)
      setSelectedTagIds((tx.tags ?? []).map((t) => t.id))
      setDraftTagNames([])
      setHasUnsavedMagicEdits(false)
      setAccountId(tx.account_id ?? UNASSIGNED_ACCOUNT)
      setDate(date)
      setTime(time)
      setNote(tx.note ?? "")
      setConvertToLocalCurrency(false)
    },
    [timezone]
  )

  useEffect(() => {
    const nextKey = `${tx.updated_at_utc}|${timezone ?? ""}`
    if (
      editedFields.current.size === 0 &&
      !isEnriching &&
      lastLoadedTxRef.current.key !== nextKey
    ) {
      syncFormFromTransaction(tx)
    }
  }, [tx, timezone, isEnriching, syncFormFromTransaction])

  const invalidateAll = () => {
    invalidateTransactionRelatedQueries(queryClient)
    onTransactionMutated?.()
  }

  const updateMutation = useMutation({
    mutationFn: updateTransactionApi,
    onSuccess: (updatedTx) => {
      syncFormFromTransaction(updatedTx)
      queryClient.setQueryData<TransactionInspectorView>(
        transactionInspectorQuery(updatedTx.id).queryKey,
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

  const categoryIconMap = useMemo(
    () => buildCategoryIconMap(categories),
    [categories]
  )

  const categoryOptions = useMemo(
    () => buildCategorySelectOptions(categories, category),
    [categories, category]
  )

  const detailAccountId = tx.account_id
  const detailAccountName = tx.account_name
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

  function editDraft<Value>(
    field: DraftField,
    setter: (value: Value) => void,
    value: Value
  ) {
    editedFields.current.add(field)
    setter(value)
  }

  function handleSave(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    if (isEnriching) return
    setFormError(null)
    setStatusMessage(null)
    try {
      const edits = editedFields.current
      const loaded = lastLoadedTxRef.current
      const saveAmount = edits.has("amount") || convertToLocalCurrency
      if (saveAmount && loaded?.currency !== tx.currency) {
        throw new Error(
          "The transaction currency changed. Reopen it before editing the amount."
        )
      }
      const targetCurrency = convertToLocalCurrency
        ? localCurrency
        : tx.currency
      const saveDate =
        edits.has("date") && (date !== loaded?.date || time !== loaded?.time)

      if (saveDate && (!date.trim() || !time.trim())) {
        throw new Error("Enter a date and time in Details before saving.")
      }

      const saveMerchant = edits.has("merchant")
      const saveRemark = edits.has("remark") || saveMerchant
      updateMutation.mutate({
        id: tx.id,
        remark: saveRemark ? remark.trim() : undefined,
        merchant: saveMerchant ? merchant : undefined,
        remark_user_edited: saveRemark ? remarkUserEdited : undefined,
        amount_minor: saveAmount
          ? parseDecimalToMinorClient(amount, targetCurrency)
          : undefined,
        category: edits.has("category") ? category : undefined,
        tag_ids: edits.has("tags") ? selectedTagIds : undefined,
        tag_names: edits.has("tags") ? draftTagNames : undefined,
        account_id: edits.has("account")
          ? accountId !== UNASSIGNED_ACCOUNT
            ? accountId
            : null
          : undefined,
        occurred_at_utc: saveDate
          ? dateTimeInputToUtcIso(date, time || "00:00", loaded?.timezone)
          : undefined,
        note: edits.has("note") ? note.trim() || null : undefined,
        convertToLocalCurrency: saveAmount ? convertToLocalCurrency : undefined,
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
          tags ?? [],
          lastLoadedTxRef.current?.timezone
        )

        if (changed.has("Remark")) {
          setRemarkUserEdited(next.remarkUserEdited)
          editDraft("remark", setRemark, next.remark)
        }
        if (changed.has("Merchant for reports")) {
          editDraft("merchant", setMerchant, next.merchant)
        }
        if (changed.has("Category"))
          editDraft("category", setCategory, next.category)
        if (changed.has("Note")) editDraft("note", setNote, next.note)
        if (changed.has("Date")) editDraft("date", setDate, next.date)
        if (changed.has("Time")) editDraft("date", setTime, next.time)
        if (changed.has("Tags")) {
          editDraft("tags", setSelectedTagIds, next.selectedTagIds)
          editDraft("tags", setDraftTagNames, next.draftTagNames)
        }
        if (changed.size) setHasUnsavedMagicEdits(true)
        setStatusMessage(message)
      },
      onError: (msg) => {
        setFormError(msg)
      },
    })
  }

  return (
    <div className="space-y-5">
      <div className="rounded-xl border bg-secondary/60 p-4">
        <p className="text-sm font-medium wrap-break-word text-secondary-foreground">
          {tx.remark ?? tx.merchant}
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
      <div role="status" className="empty:absolute">
        {statusMessage && (
          <Alert role="presentation">
            <AlertDescription>{statusMessage}</AlertDescription>
          </Alert>
        )}
      </div>

      <Tabs defaultValue="details">
        <TabsList aria-label="Transaction detail sections">
          <TabsTab value="details">Details</TabsTab>
          <TabsTab value="note">
            Note
            <span className="text-xs text-muted-foreground">
              {note.trim() ? "Has content" : "Empty"}
            </span>
          </TabsTab>
          <TabsTab value="sources">
            <MailIcon className="size-3.5" /> Source emails (
            {detail.sources.length})
          </TabsTab>
        </TabsList>
        <TabsPanel value="details" keepMounted>
          <form id={detailFormId} onSubmit={handleSave} className="space-y-4">
            {detail.sources.length >= 1 && (
              <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border bg-muted/40 px-3 py-2">
                <p className="text-xs text-muted-foreground">
                  Fill remark, category, tags, date, time, and note from{" "}
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

            <div className="grid gap-4 sm:grid-cols-2" inert={isEnriching}>
              <Field>
                <FieldLabel htmlFor={remarkId}>Remark</FieldLabel>
                <Input
                  id={remarkId}
                  value={remark}
                  placeholder={tx.merchant}
                  onChange={(e) => {
                    const value = e.target.value
                    setRemarkUserEdited(
                      tx.remark_user_edited ||
                        value.trim() !== (tx.remark ?? tx.merchant).trim()
                    )
                    editDraft("remark", setRemark, value)
                  }}
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
                  onChange={(e) =>
                    editDraft("amount", setAmount, e.target.value)
                  }
                />
              </Field>

              <Field>
                <FieldLabel>Category</FieldLabel>
                <Select
                  items={categoryOptions}
                  value={category}
                  onValueChange={(v) =>
                    editDraft("category", setCategory, v ?? DEFAULT_CATEGORY)
                  }
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
                  tags={tags ?? []}
                  selectedTagIds={selectedTagIds}
                  onChange={(ids) => editDraft("tags", setSelectedTagIds, ids)}
                  draftTagNames={draftTagNames}
                  onDraftTagNamesChange={(names) =>
                    editDraft("tags", setDraftTagNames, names)
                  }
                  disabled={isEnriching || updateMutation.isPending}
                />
              </Field>

              <Field>
                <FieldLabel>Account</FieldLabel>
                <Select
                  items={accountItems}
                  value={accountId}
                  onValueChange={(v) =>
                    editDraft("account", setAccountId, v ?? UNASSIGNED_ACCOUNT)
                  }
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
                  onChange={(value) => editDraft("date", setDate, value)}
                  ariaLabel="Occurred date"
                />
              </Field>

              <Field>
                <FieldLabel>Occurred time</FieldLabel>
                <Input
                  type="time"
                  required
                  value={time}
                  onChange={(e) => editDraft("date", setTime, e.target.value)}
                />
              </Field>
            </div>

            {tx.isForeignCurrency && (
              <div className="flex items-center gap-2">
                <Checkbox
                  id="convert-local"
                  checked={convertToLocalCurrency}
                  onCheckedChange={(checked) =>
                    editDraft(
                      "conversion",
                      setConvertToLocalCurrency,
                      Boolean(checked)
                    )
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
        <TabsPanel value="note">
          <Field>
            <FieldLabel htmlFor={noteId}>Note</FieldLabel>
            <Textarea
              id={noteId}
              rows={8}
              className="min-h-40"
              disabled={isEnriching || updateMutation.isPending}
              value={note}
              onChange={(e) => editDraft("note", setNote, e.target.value)}
            />
          </Field>
          <Button
            type="submit"
            form={detailFormId}
            formNoValidate
            disabled={updateMutation.isPending || isEnriching}
          >
            {updateMutation.isPending ? "Saving..." : "Save changes"}
          </Button>
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
                  <SourceEmail
                    source={source}
                    transactionId={tx.id}
                    timezone={timezone}
                    onSplit={
                      detail.sources.length > 1
                        ? () =>
                            splitMutation.mutate({
                              transactionId: tx.id,
                              emailId: source.email_id,
                            })
                        : undefined
                    }
                    splitDisabled={splitMutation.isPending || isEnriching}
                  />
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
  onTransactionMutated,
}: {
  transactionId: string | null
  accounts: AccountOption[]
  localCurrency: string
  timezone: string | null | undefined
  onClose: () => void
  onTransactionMutated?: () => void
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
              onTransactionMutated={onTransactionMutated}
            />
          )}
        </DrawerBody>
      </DrawerContent>
    </Drawer>
  )
}
