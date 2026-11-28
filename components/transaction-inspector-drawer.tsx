"use client"

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  CheckIcon,
  ChevronDownIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  EllipsisIcon,
  SaveIcon,
  Trash2Icon,
} from "lucide-react"
import {
  useId,
  useImperativeHandle,
  useLayoutEffect,
  useRef,
  useState,
  type Ref,
  type SubmitEvent,
} from "react"

import { DetailDrawerContent } from "@/components/detail-drawer-content"
import { IconTooltip } from "@/components/icon-tooltip"
import {
  MagicEnrichControl,
  patchTagsQueryCache,
  resolveMagicDraft,
  useMagicEnrich,
  type MagicField,
} from "@/components/magic-enrich"
import { DestructiveConfirmationContent } from "@/components/destructive-confirmation-content"
import { ReviewedIcon } from "@/components/reviewed"
import {
  TransactionForm,
  TransactionFormFields,
  UNASSIGNED_ACCOUNT,
} from "@/components/transaction-form"
import { TransferFormDialog } from "@/components/transfer-form-dialog"
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
import { ButtonGroup } from "@/components/ui/button-group"
import { Card, CardContent } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Drawer, DrawerFooter } from "@/components/ui/drawer"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import type { useTransactionReview } from "@/hooks/use-transaction-review"
import { accountsQuery } from "@/lib/api/accounts"
import { categoriesQuery } from "@/lib/api/categories"
import { tagsQuery } from "@/lib/api/tags"
import {
  transactionInspectorQuery,
  transactionSourceBodyQuery,
} from "@/lib/api/transaction-inspector"
import {
  deleteTransactionApi,
  invalidateTransactionRelatedQueries,
  markTransactionsReviewedApi,
  saveTransfersApi,
  splitTransactionSourceApi,
  updateTransactionApi,
} from "@/lib/api/transactions"
import { invalidateTriageRelatedQueries } from "@/lib/api/triage"
import {
  type AccountOption,
  type CategorySummary,
  type TagSummary,
  type TransactionInspectorSource,
  type TransactionInspectorView,
  type TransactionListItem,
} from "@/lib/api/types.generated"
import {
  formatTransactionAmount,
  formatUtcInTimezone,
  groupAmountInput,
  minorToDecimalInput,
  transactionKindLabel,
  utcToDateInputValue,
  utcToTimeInputValue,
} from "@/lib/format"
import {
  buildInspectorPatch,
  type InspectorDraftField,
} from "@/lib/transaction-inspector-patch"
import { type ReviewDirection } from "@/lib/transaction-review"

function loadDraft(tx: TransactionListItem, timezone?: string | null) {
  const date = utcToDateInputValue(tx.occurred_at_utc, timezone)
  const time = utcToTimeInputValue(tx.occurred_at_utc, timezone)
  return {
    baseline: {
      key: `${tx.updated_at_utc}|${timezone ?? ""}`,
      currency: tx.currency,
      date,
      time,
      timezone,
    },
    edits: new Set<InspectorDraftField>(),
    remark: tx.remark ?? tx.merchant,
    merchant: tx.merchant,
    remarkUserEdited: tx.remark_user_edited,
    amount: groupAmountInput(minorToDecimalInput(tx.amount_minor, tx.currency)),
    category: tx.category,
    selectedTagIds: (tx.tags ?? []).map((tag) => tag.id),
    draftTagNames: [] as string[],
    accountId: tx.account_id ?? UNASSIGNED_ACCOUNT,
    date,
    time,
    note: tx.note ?? "",
    convertToLocalCurrency: false,
  }
}

type DraftValues = Omit<ReturnType<typeof loadDraft>, "baseline" | "edits">

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
      <CardContent className="space-y-2 text-sm">
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

type Review = ReturnType<typeof useTransactionReview>
type NavigationGuard = { hasDraft: boolean; busy: boolean }

type InspectorProps = {
  transactionId: string
  accounts: AccountOption[]
  localCurrency: string
  timezone: string | null | undefined
  onTransactionMutated?: () => void
  review: Review
  navigationGuard: Ref<NavigationGuard>
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
  onTransactionMutated,
  review,
  navigationGuard,
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
  const isTransfer = tx.kind === "transfer"
  const isIncome = tx.kind === "income"
  const [transferEditorOpen, setTransferEditorOpen] = useState(false)
  const [nonTransactionDialogOpen, setNonTransactionDialogOpen] =
    useState(false)
  const [draft, setDraft] = useState(() => loadDraft(tx, timezone))
  const {
    baseline,
    edits,
    remark,
    merchant,
    remarkUserEdited,
    amount,
    category,
    selectedTagIds,
    draftTagNames,
    accountId,
    date,
    time,
    note,
    convertToLocalCurrency,
  } = draft
  const hasDraft = edits.size > 0
  const detailFormId = useId()
  const [tagCreatePending, setTagCreatePending] = useState(false)
  const incomeAccounts = useQuery({ ...accountsQuery(), enabled: isIncome })
  const receivingCurrency =
    incomeAccounts.data?.find((account) => account.id === accountId)
      ?.currency ?? tx.currency
  const amountCurrency = isIncome
    ? receivingCurrency
    : tx.isForeignCurrency && !convertToLocalCurrency
      ? tx.currency
      : localCurrency
  const [formError, setFormError] = useState<string | null>(null)
  const [statusMessage, setStatusMessage] = useState<string | null>(null)
  const [magicStatus, setMagicStatus] = useState<string | null>(null)
  const [magicChangedFields, setMagicChangedFields] = useState<
    ReadonlySet<MagicField>
  >(() => new Set())
  const { isEnriching, runEnrichment } = useMagicEnrich()

  const sourceKey = `${tx.updated_at_utc}|${timezone ?? ""}`
  if (!hasDraft && !isEnriching && baseline.key !== sourceKey) {
    setDraft(loadDraft(tx, timezone))
  }

  const invalidateAll = () => {
    invalidateTransactionRelatedQueries(queryClient)
    onTransactionMutated?.()
  }

  function patchInspector(
    update: (transaction: TransactionListItem) => TransactionListItem
  ) {
    queryClient.setQueryData(
      transactionInspectorQuery(transactionId).queryKey,
      (previous) =>
        previous
          ? { ...previous, transaction: update(previous.transaction) }
          : previous
    )
  }

  const updateMutation = useMutation({
    mutationFn: updateTransactionApi,
    onSuccess: (updatedTx) => {
      setDraft(loadDraft(updatedTx, timezone))
      patchInspector(() => updatedTx)
      patchTagsQueryCache(
        queryClient,
        updatedTx.tags ?? [],
        updatedTx.updated_at_utc
      )
      setFormError(null)
      setMagicStatus(null)
      setMagicChangedFields(new Set())
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
      review.close()
    },
    onError: (err) => {
      setFormError(
        err instanceof Error ? err.message : "Failed to delete transaction."
      )
    },
  })

  const dispositionMutation = useMutation({
    mutationFn: () =>
      saveTransfersApi([
        {
          action: "non_transaction",
          expected_transaction_id: tx.id,
          email_ids: detail.sources.map((source) => source.email_id),
        },
      ]),
    onSuccess: () => {
      invalidateTriageRelatedQueries(queryClient)
      onTransactionMutated?.()
      review.close()
    },
    onError: (err) => setFormError(err.message),
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
    mutationFn: async () => {
      const { updatedCount } = await markTransactionsReviewedApi([
        transactionId,
      ])
      if (!updatedCount)
        throw new Error("This transaction could not be marked reviewed.")
    },
    onSuccess: () => {
      patchInspector((transaction) => ({ ...transaction, reviewed: 1 }))
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

  function editDraft(
    field: InspectorDraftField,
    values:
      Partial<DraftValues> | ((current: DraftValues) => Partial<DraftValues>)
  ) {
    setDraft((current) => ({
      ...current,
      ...(typeof values === "function" ? values(current) : values),
      edits: new Set([...current.edits, field]),
    }))
  }

  const saveDisabled =
    review.pending ||
    isEnriching ||
    tagCreatePending ||
    deleteMutation.isPending ||
    splitMutation.isPending ||
    dispositionMutation.isPending ||
    transferEditorOpen

  useImperativeHandle(
    navigationGuard,
    () => ({ hasDraft, busy: saveDisabled }),
    [hasDraft, saveDisabled]
  )

  function submitTransaction(advance: boolean) {
    if (saveDisabled) return
    setFormError(null)
    setStatusMessage(null)
    try {
      const patch = buildInspectorPatch(
        {
          ...draft,
          accountId: accountId === UNASSIGNED_ACCOUNT ? null : accountId,
        },
        {
          id: tx.id,
          currency: tx.currency,
          isIncome,
          receivingCurrency,
          localCurrency,
        }
      )
      void review.write(() => updateMutation.mutateAsync(patch), advance)
    } catch (err) {
      setFormError(err instanceof Error ? err.message : "Invalid input.")
    }
  }

  function handleSave(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    const advance =
      event.nativeEvent.submitter?.getAttribute("value") === "save-and-next"
    submitTransaction(advance)
  }

  function handleMagicEnrich() {
    if (saveDisabled) return
    runEnrichment({
      transactionIds: [tx.id],
      onStart: () => {
        setFormError(null)
        setStatusMessage(null)
        setMagicStatus(null)
        setMagicChangedFields(new Set())
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
          baseline.timezone
        )

        if (changed.has("Remark")) {
          editDraft("remark", {
            remark: next.remark,
            remarkUserEdited: next.remarkUserEdited,
          })
        }
        if (changed.has("Merchant for reports")) {
          editDraft("merchant", { merchant: next.merchant })
        }
        if (changed.has("Category"))
          editDraft("category", { category: next.category })
        if (changed.has("Note")) editDraft("note", { note: next.note })
        if (changed.has("Date")) editDraft("date", { date: next.date })
        if (changed.has("Time")) editDraft("date", { time: next.time })
        if (changed.has("Tags")) {
          editDraft("tags", {
            selectedTagIds: next.selectedTagIds,
            draftTagNames: next.draftTagNames,
          })
        }
        setMagicStatus(message)
        setMagicChangedFields(changed)
      },
      onError: (msg) => {
        setFormError(msg)
      },
    })
  }

  const reviewed = tx.reviewed === 1

  function handleMarkReviewed() {
    setFormError(null)
    setStatusMessage(null)
    void review.write(() => markReviewedMutation.mutateAsync(), false)
  }

  function reviewActions(form?: string) {
    const skipNativeValidation = Boolean(form)
    const canAdvance = review.next
    const hasSecondaryActions = !reviewed || canAdvance
    return (
      <div className="flex items-center gap-2">
        {reviewed ? <ReviewedIcon /> : null}
        {hasSecondaryActions ? (
          <ButtonGroup>
            {!reviewed ? (
              <IconTooltip label="Mark reviewed">
                <Button
                  type="button"
                  variant="outline"
                  size="icon-sm"
                  aria-label="Mark reviewed"
                  disabled={saveDisabled}
                  onClick={handleMarkReviewed}
                >
                  <CheckIcon className="size-4" aria-hidden="true" />
                </Button>
              </IconTooltip>
            ) : null}
            {canAdvance ? (
              <IconTooltip label="Save changes">
                <Button
                  type="submit"
                  form={form}
                  formNoValidate={skipNativeValidation}
                  variant="outline"
                  size="icon-sm"
                  aria-label="Save changes"
                  disabled={saveDisabled}
                >
                  <SaveIcon className="size-4" aria-hidden="true" />
                </Button>
              </IconTooltip>
            ) : null}
          </ButtonGroup>
        ) : null}
        <Button
          type="submit"
          form={form}
          formNoValidate={skipNativeValidation}
          value={canAdvance ? "save-and-next" : undefined}
          size="sm"
          disabled={saveDisabled}
        >
          {updateMutation.isPending
            ? "Saving..."
            : canAdvance
              ? "Save changes & next"
              : "Save changes"}
        </Button>
      </div>
    )
  }

  const deleteAction = (
    <AlertDialog>
      <IconTooltip label="Delete transaction">
        <AlertDialogTrigger
          render={
            <Button
              type="button"
              variant="destructive"
              size="icon-sm"
              aria-label="Delete transaction"
              disabled={saveDisabled}
            />
          }
        >
          <Trash2Icon className="size-4" aria-hidden="true" />
        </AlertDialogTrigger>
      </IconTooltip>
      <DestructiveConfirmationContent
        title="Delete this transaction?"
        description="This permanently deletes the transaction and marks any linked source emails as dismissed."
        confirmLabel="Delete forever"
        confirmDisabled={deleteMutation.isPending}
        onConfirm={() => deleteMutation.mutate(tx.id)}
      />
    </AlertDialog>
  )

  const magicEnrichDescription = `Fill remark, category, tags, date, time, and note from ${detail.sources.length} ${detail.sources.length === 1 ? "attached email" : "attached emails"}.`

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border bg-secondary/60 p-4">
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium wrap-break-word text-secondary-foreground">
            <Badge variant="outline" className="mr-2">
              {transactionKindLabel(tx.kind)}
            </Badge>
            {tx.remark ?? tx.merchant}
          </p>
          <p className="mt-1 text-2xl font-semibold tracking-tight text-foreground tabular-nums">
            {formatTransactionAmount(tx)}
          </p>
          {isTransfer && (
            <p className="text-sm text-muted-foreground">
              {tx.account_name} → {tx.destination_account_name}
            </p>
          )}
          <p className="mt-1 text-xs text-muted-foreground">
            {formatUtcInTimezone(tx.occurred_at_utc, timezone)}
          </p>
        </div>
        {!isTransfer && detail.sources.length >= 1 && (
          <MagicEnrichControl
            isEnriching={isEnriching}
            disabled={saveDisabled}
            onClick={handleMagicEnrich}
            description={magicEnrichDescription}
            statusMessage={magicStatus}
          />
        )}
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

      {transferEditorOpen && (
        <TransferFormDialog
          targets={[
            {
              transaction: tx,
              title: tx.remark ?? tx.merchant,
              cardPayment: !isTransfer,
              emailIds: isTransfer
                ? undefined
                : detail.sources.map((source) => source.email_id),
            },
          ]}
          timezone={timezone}
          onClose={() => setTransferEditorOpen(false)}
          onSuccess={() => {
            onTransactionMutated?.()
            review.close()
          }}
        />
      )}

      <TransactionForm
        note={{
          disabled: isTransfer || isEnriching || review.pending,
          value: note,
          onChange: (value) => editDraft("note", { note: value }),
          valid: magicChangedFields.has("Note"),
        }}
        noteActions={isTransfer ? undefined : reviewActions(detailFormId)}
        details={
          isTransfer ? (
            <div className="flex flex-wrap gap-2">
              <Button
                type="button"
                disabled={saveDisabled}
                onClick={() => setTransferEditorOpen(true)}
              >
                Edit transfer
              </Button>
              {deleteAction}
            </div>
          ) : (
            <form id={detailFormId} onSubmit={handleSave} className="space-y-4">
              <TransactionFormFields
                description={{
                  value: remark,
                  placeholder: tx.merchant,
                  onChange: (value) => {
                    editDraft("remark", {
                      remark: value,
                      remarkUserEdited:
                        tx.remark_user_edited ||
                        value.trim() !== (tx.remark ?? tx.merchant).trim(),
                    })
                  },
                }}
                inert={isEnriching || review.pending}
                currency={amountCurrency}
                amount={amount}
                onAmountChange={(value) =>
                  editDraft("amount", { amount: value })
                }
                amountInputProps={{
                  onBlur: () =>
                    setDraft((current) => ({
                      ...current,
                      amount: groupAmountInput(current.amount),
                    })),
                  className: "tabular-nums",
                }}
                categories={categories}
                category={category}
                onCategoryChange={(value) =>
                  editDraft("category", { category: value })
                }
                tagPicker={{
                  tags: tags ?? [],
                  selectedTagIds,
                  onChange: (ids) =>
                    editDraft("tags", (current) => ({
                      selectedTagIds:
                        typeof ids === "function"
                          ? ids(current.selectedTagIds)
                          : ids,
                    })),
                  draftTagNames,
                  onDraftTagNamesChange: (names) =>
                    editDraft("tags", (current) => ({
                      draftTagNames:
                        typeof names === "function"
                          ? names(current.draftTagNames)
                          : names,
                    })),
                  onCreatePendingChange: setTagCreatePending,
                  disabled: isEnriching || review.pending,
                }}
                accounts={accounts}
                receivingAccount={isIncome}
                accountId={accountId}
                onAccountChange={(value) => {
                  if (isIncome) {
                    const next = incomeAccounts.data?.find(
                      (account) => account.id === value
                    )
                    if (!next) return
                    if (next.currency !== receivingCurrency)
                      editDraft("amount", { amount: "" })
                  }
                  editDraft("account", { accountId: value })
                }}
                missingAccountLabel={
                  tx.account_id === accountId ? tx.account_name : null
                }
                paymentMethod={tx.payment_method_raw}
                date={date}
                onDateChange={(value) => editDraft("date", { date: value })}
                dateLabel="Occurred date and time"
                time={{
                  value: time,
                  onChange: (value) => editDraft("date", { time: value }),
                }}
                validFields={magicChangedFields}
              />

              {tx.isForeignCurrency && !isIncome && (
                <div className="flex items-center gap-2">
                  <Checkbox
                    id="convert-local"
                    checked={convertToLocalCurrency}
                    disabled={review.pending}
                    onCheckedChange={(checked) =>
                      editDraft("conversion", {
                        convertToLocalCurrency: Boolean(checked),
                      })
                    }
                  />
                  <Label htmlFor="convert-local">
                    Convert to local currency ({localCurrency})
                  </Label>
                </div>
              )}

              <div className="space-y-2 pt-2">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div className="flex items-center gap-2">
                    {deleteAction}
                    {!isIncome && detail.sources.length > 0 && (
                      <DropdownMenu>
                        <DropdownMenuTrigger
                          render={
                            <Button
                              type="button"
                              variant="outline"
                              size="icon-sm"
                              aria-label="More actions"
                              disabled={saveDisabled || hasDraft}
                            />
                          }
                        >
                          <EllipsisIcon className="size-4" aria-hidden="true" />
                        </DropdownMenuTrigger>
                        <DropdownMenuContent side="top" align="start">
                          <DropdownMenuItem
                            onClick={() => setTransferEditorOpen(true)}
                          >
                            Mark as card payment
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            onClick={() => setNonTransactionDialogOpen(true)}
                          >
                            Mark as non-transaction
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    )}
                  </div>

                  {reviewActions()}
                </div>

                {!isIncome && detail.sources.length > 0 && hasDraft && (
                  <p className="text-xs text-muted-foreground">
                    Save your edits before reclassifying this expense.
                  </p>
                )}
              </div>

              <AlertDialog
                open={nonTransactionDialogOpen}
                onOpenChange={setNonTransactionDialogOpen}
              >
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>
                      Mark as non-transaction?
                    </AlertDialogTitle>
                    <AlertDialogDescription>
                      Use this for a statement notice or another email that did
                      not record a movement of money. The expense will be
                      removed.
                    </AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>Cancel</AlertDialogCancel>
                    <AlertDialogAction
                      disabled={dispositionMutation.isPending}
                      onClick={() => dispositionMutation.mutate()}
                    >
                      Mark as non-transaction
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            </form>
          )
        }
        sourceTab={{
          count: detail.sources.length,
          content:
            detail.sources.length > 0 ? (
              <>
                {!isTransfer &&
                  !isIncome &&
                  tx.manual_override === 1 &&
                  detail.sources.length > 1 && (
                    <Alert>
                      <AlertDescription>
                        This transaction has a locked amount (edited or
                        combined). Splitting an email creates a separate
                        transaction without reducing this transaction&apos;s
                        amount — adjust the amount manually if needed.
                      </AlertDescription>
                    </Alert>
                  )}
                <ul className="space-y-3">
                  {detail.sources.map((source) => (
                    <li key={source.id}>
                      <SourceEmail
                        source={source}
                        transactionId={tx.id}
                        timezone={timezone}
                        onSplit={
                          !isTransfer && !isIncome && detail.sources.length > 1
                            ? () =>
                                splitMutation.mutate({
                                  transactionId: tx.id,
                                  emailId: source.email_id,
                                })
                            : undefined
                        }
                        splitDisabled={saveDisabled}
                      />
                    </li>
                  ))}
                </ul>
              </>
            ) : undefined,
        }}
      />
    </div>
  )
}

export function TransactionInspectorDrawer({
  review,
  accounts,
  localCurrency,
  timezone,
  onTransactionMutated,
}: {
  review: Review
  accounts: AccountOption[]
  localCurrency: string
  timezone: string | null | undefined
  onTransactionMutated?: () => void
}) {
  const { transactionId } = review
  const navigationGuard = useRef<NavigationGuard>(null)
  const navigation = useRef<HTMLElement>(null)
  const previousTransactionId = useRef(transactionId)
  const [discard, setDiscard] = useState<{
    id: string
    direction: ReviewDirection
  } | null>(null)
  const discardDirection =
    discard?.id === transactionId ? discard.direction : null

  useLayoutEffect(() => {
    const previousId = previousTransactionId.current
    previousTransactionId.current = transactionId
    if (
      transactionId &&
      previousId &&
      transactionId !== previousId &&
      !navigation.current?.contains(document.activeElement)
    ) {
      navigation.current?.focus({ preventScroll: true })
    }
  }, [transactionId])

  function navigate(direction: ReviewDirection) {
    if (!transactionId || navigationGuard.current?.busy) return
    if (navigationGuard.current?.hasDraft) {
      setDiscard({ id: transactionId, direction })
      return
    }
    void review.navigate(direction)
  }

  return (
    <Drawer
      swipeDirection="right"
      open={transactionId !== null}
      onOpenChange={(open) => {
        if (!open) {
          setDiscard(null)
          review.close()
        }
      }}
    >
      <DetailDrawerContent
        title="Transaction details"
        description="Inspect attached source emails, mark reviewed, or edit transaction fields."
        scrollResetKey={transactionId}
        footer={
          <DrawerFooter className="items-center border-t px-5 py-3 pb-[max(0.75rem,env(safe-area-inset-bottom))] sm:px-6">
            <nav
              ref={navigation}
              tabIndex={-1}
              aria-label="Transaction navigation"
              className="flex flex-wrap items-center justify-center gap-2"
            >
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="aria-disabled:opacity-50"
                disabled={!review.previous || review.pending}
                focusableWhenDisabled
                onClick={() => navigate("previous")}
              >
                <ChevronLeftIcon aria-hidden="true" data-icon="inline-start" />
                Previous
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="aria-disabled:opacity-50"
                disabled={!review.next || review.pending}
                focusableWhenDisabled
                onClick={() => navigate("next")}
              >
                Next
                <ChevronRightIcon aria-hidden="true" data-icon="inline-end" />
              </Button>
            </nav>
            {review.error ? (
              <p role="alert" className="text-center text-sm text-destructive">
                {review.error}
              </p>
            ) : null}
          </DrawerFooter>
        }
      >
        {transactionId && (
          <InspectorBody
            key={transactionId}
            transactionId={transactionId}
            accounts={accounts}
            localCurrency={localCurrency}
            timezone={timezone}
            onTransactionMutated={onTransactionMutated}
            review={review}
            navigationGuard={navigationGuard}
          />
        )}
        <AlertDialog
          open={discardDirection !== null}
          onOpenChange={(open) => {
            if (!open) setDiscard(null)
          }}
        >
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>Discard unsaved changes?</AlertDialogTitle>
              <AlertDialogDescription>
                Your changes to this transaction will be lost if you move on.
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>Keep editing</AlertDialogCancel>
              <AlertDialogAction
                variant="destructive"
                onClick={() => {
                  setDiscard(null)
                  if (discardDirection) void review.navigate(discardDirection)
                }}
              >
                Discard and move
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </DetailDrawerContent>
    </Drawer>
  )
}
