"use client"

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  memo,
  useCallback,
  useId,
  useRef,
  useState,
  type ReactNode,
  type SubmitEvent,
} from "react"

import { DatePicker } from "@/components/date-picker"
import { CreateFormDrawer } from "@/components/detail-drawer-content"
import { PaginationControls } from "@/components/pagination-controls"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { accountsQuery } from "@/lib/api/accounts"
import { transactionInspectorQuery } from "@/lib/api/transaction-inspector"
import {
  saveTransfersApi,
  TRANSACTIONS_PAGE_SIZE,
  transactionsQuery,
  type TransferMutationItem,
} from "@/lib/api/transactions"
import {
  invalidateTriageRelatedQueries,
  skippedEmailBodyQuery,
} from "@/lib/api/triage"
import type {
  AccountSummary,
  TransactionListItem,
} from "@/lib/api/types.generated"
import {
  dateTimeInputToUtcIso,
  formatMinorCurrency,
  formatUtcInTimezone,
  getTodayDateString,
  minorToDecimalInput,
  parseDecimalToMinorClient,
  utcToDateInputValue,
  utcToTimeInputValue,
} from "@/lib/format"

export type TransferTarget = {
  title?: string
  transaction?: TransactionListItem
  emailIds?: string[]
  skippedEmailId?: string
  amountMinor?: number
  currency?: string
  occurredAtUtc?: string
  cardPayment?: boolean
}

const TRANSFER_ACTION_OPTIONS = [
  { value: "record", label: "Record transfer" },
  { value: "match", label: "Match an existing transfer" },
] as const

type LocalDateTime = {
  date: string
  time: string
}

type Draft = {
  requestId: string
  mode: "record" | "match"
  from: string
  to: string
  amount: string
  received: string
  dateTime: LocalDateTime
  sourcePosted: LocalDateTime
  destinationPosted: LocalDateTime
  note: string
  matchId: string
}

function localInput(
  instant: string | null | undefined,
  timezone?: string | null
): LocalDateTime {
  if (!instant) return { date: "", time: "" }
  const date = utcToDateInputValue(instant, timezone)
  const time = utcToTimeInputValue(instant, timezone, true)
  return date && time ? { date, time } : { date: "", time: "" }
}

function toUtc(
  value: LocalDateTime,
  original: string | null | undefined,
  timezone?: string | null
) {
  if (original) {
    const loaded = localInput(original, timezone)
    if (loaded.date === value.date && loaded.time === value.time)
      return original
  }
  return dateTimeInputToUtcIso(value.date, value.time, timezone)
}

function dateTimePickerProps(
  current: LocalDateTime,
  onChange: (value: LocalDateTime) => void
) {
  return {
    value: current.date,
    onChange: (date: string) =>
      onChange({ date, time: date ? current.time : "" }),
    time: {
      value: current.time,
      onChange: (time: string) => onChange({ ...current, time }),
      step: 1,
    },
  }
}

function initialDraft(target: TransferTarget, timezone?: string | null): Draft {
  const tx = target.transaction
  const original = localInput(
    tx?.occurred_at_utc ?? target.occurredAtUtc,
    timezone
  )
  const amount = tx?.amount_minor ?? target.amountMinor
  const currency = tx?.currency ?? target.currency
  return {
    requestId: crypto.randomUUID(),
    mode: "record",
    from: tx?.account_id ?? "",
    to: tx?.destination_account_id ?? "",
    amount: amount && currency ? minorToDecimalInput(amount, currency) : "",
    received:
      tx?.destination_amount_minor != null && tx.destination_currency
        ? minorToDecimalInput(
            tx.destination_amount_minor,
            tx.destination_currency
          )
        : "",
    dateTime: original.date
      ? original
      : { date: getTodayDateString(timezone), time: "00:00" },
    sourcePosted: localInput(tx?.source_posted_at_utc, timezone),
    destinationPosted: localInput(tx?.destination_posted_at_utc, timezone),
    note: tx?.note ?? "",
    matchId: "",
  }
}

function SkippedPaymentEvidence({ emailId }: { emailId: string }) {
  const [showEvidence, setShowEvidence] = useState(false)
  const evidence = useQuery(skippedEmailBodyQuery(emailId, showEvidence))

  return (
    <details onToggle={(event) => setShowEvidence(event.currentTarget.open)}>
      <summary className="cursor-pointer text-sm font-medium">
        Read source email
      </summary>
      {evidence.isPending && showEvidence && (
        <p role="status" className="mt-2 text-sm">
          Loading email…
        </p>
      )}
      {evidence.isError && (
        <p role="alert" className="mt-2 text-sm text-destructive">
          {evidence.error.message}
        </p>
      )}
      {evidence.data && (
        <div className="mt-2 max-h-56 overflow-auto rounded-md border bg-muted/30 p-3 text-sm wrap-break-word whitespace-pre-wrap">
          {evidence.data.cleanedText || "No email body available."}
        </div>
      )}
    </details>
  )
}

function TransferMatchPicker({
  active,
  cardPayment,
  matchId,
  name,
  onMatchChange,
  timezone,
}: {
  active: boolean
  cardPayment?: boolean
  matchId: string
  name: string
  onMatchChange: (id: string) => void
  timezone?: string | null
}) {
  const [matchPage, setMatchPage] = useState(1)
  const matches = useQuery({
    ...transactionsQuery({
      kind: "transfer",
      ledgerRange: "all",
      destinationAccountType: cardPayment ? "credit_card" : undefined,
      page: matchPage,
    }),
    enabled: active,
  })

  if (!active) return null

  const candidates = matches.data?.items ?? []

  return (
    <div className="space-y-3">
      {matches.isPending && (
        <p role="status" className="text-sm">
          Loading transfers…
        </p>
      )}
      {matches.isError && (
        <p role="alert" className="text-sm text-destructive">
          {matches.error.message}
        </p>
      )}
      {!matches.isPending && candidates.length === 0 && (
        <p className="text-sm text-muted-foreground">
          No eligible transfers on this page.
        </p>
      )}
      {candidates.map((item) => (
        <label
          key={item.id}
          className="flex cursor-pointer items-start gap-3 rounded-md border p-3 text-sm"
        >
          <input
            type="radio"
            name={name}
            value={item.id}
            checked={matchId === item.id}
            onChange={() => onMatchChange(item.id)}
            className="mt-1"
          />
          <span>
            <span className="block font-medium">
              {item.account_name} → {item.destination_account_name}
            </span>
            {formatMinorCurrency(item.amount_minor, item.currency)}
            {item.destination_amount_minor != null &&
              item.destination_currency && (
                <>
                  {" "}
                  →{" "}
                  {formatMinorCurrency(
                    item.destination_amount_minor,
                    item.destination_currency
                  )}
                </>
              )}
            <span className="block text-xs text-muted-foreground">
              {formatUtcInTimezone(
                item.source_posted_at_utc ?? item.occurred_at_utc,
                timezone
              )}
              {" → "}
              {formatUtcInTimezone(
                item.destination_posted_at_utc ?? item.occurred_at_utc,
                timezone
              )}
            </span>
          </span>
        </label>
      ))}
      <div className="flex flex-wrap items-center gap-2">
        <PaginationControls
          page={matchPage}
          totalPages={Math.max(
            1,
            Math.ceil(
              (matches.data?.totalCount ?? 0) /
                (matches.data?.pageSize || TRANSACTIONS_PAGE_SIZE)
            )
          )}
          isLoading={matches.isFetching}
          onPageChange={setMatchPage}
        />
      </div>
    </div>
  )
}

const TransferFields = memo(function TransferFields({
  index,
  draft,
  updateDraft,
  target,
  accounts,
  disabled,
  timezone,
}: {
  index: number
  draft: Draft
  updateDraft: (index: number, patch: Partial<Draft>) => void
  target: TransferTarget
  accounts: AccountSummary[]
  disabled: boolean
  timezone?: string | null
}) {
  const id = useId()
  const setDraft = (patch: Partial<Draft>) => updateDraft(index, patch)
  const hasEmailEvidence = Boolean(
    target.emailIds?.length ||
    target.skippedEmailId ||
    (target.transaction?.kind === "expense" &&
      target.transaction.sourceCount > 0)
  )
  const from = accounts.find((account) => account.id === draft.from)
  const to = accounts.find((account) => account.id === draft.to)
  const crossCurrency = from && to && from.currency !== to.currency
  const accountItems = accounts.map((account) => ({
    value: account.id,
    label: `${account.name} (${account.currency})`,
    type: account.type,
  }))
  const destinationItems = accountItems.filter(
    (account) =>
      account.value !== draft.from &&
      (!target.cardPayment || account.type === "credit_card")
  )

  return (
    <fieldset disabled={disabled} className="space-y-4">
      {target.title && (
        <legend className="px-1 text-sm font-medium">{target.title}</legend>
      )}
      {target.skippedEmailId && (
        <SkippedPaymentEvidence emailId={target.skippedEmailId} />
      )}
      {hasEmailEvidence && (
        <Field>
          <FieldLabel htmlFor={`${id}-action`}>Action</FieldLabel>
          <Select
            items={TRANSFER_ACTION_OPTIONS}
            value={draft.mode}
            disabled={disabled}
            onValueChange={(mode) => {
              if (mode) setDraft({ mode })
            }}
          >
            <SelectTrigger id={`${id}-action`} className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {TRANSFER_ACTION_OPTIONS.map(({ value, label }) => (
                <SelectItem key={value} value={value}>
                  {label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
      )}
      <TransferMatchPicker
        active={draft.mode === "match"}
        key={target.cardPayment ? "card-payment" : "transfer"}
        cardPayment={target.cardPayment}
        matchId={draft.matchId}
        name={`${id}-match`}
        onMatchChange={(matchId) => setDraft({ matchId })}
        timezone={timezone}
      />
      {draft.mode === "record" && (
        <div className="grid gap-4 sm:grid-cols-2">
          <Field>
            <FieldLabel htmlFor={`${id}-from`}>From account</FieldLabel>
            <Select
              items={accountItems}
              required
              disabled={disabled}
              value={draft.from || null}
              onValueChange={(value) => {
                const accountId = value ?? ""
                const next = accounts.find(
                  (account) => account.id === accountId
                )
                const currencyChanged =
                  (from?.currency ??
                    target.currency ??
                    target.transaction?.currency) !== next?.currency
                setDraft({
                  from: accountId,
                  to: draft.to === accountId ? "" : draft.to,
                  amount:
                    currencyChanged &&
                    (from || target.currency || target.transaction)
                      ? ""
                      : draft.amount,
                  received: draft.to === accountId ? "" : draft.received,
                })
              }}
            >
              <SelectTrigger id={`${id}-from`} className="w-full">
                <SelectValue placeholder="Choose an account" />
              </SelectTrigger>
              <SelectContent>
                {accountItems.map(({ value, label }) => (
                  <SelectItem key={value} value={value}>
                    {label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <Field>
            <FieldLabel htmlFor={`${id}-to`}>
              {target.cardPayment ? "Credit card" : "To account"}
            </FieldLabel>
            <Select
              items={destinationItems}
              required
              disabled={disabled}
              value={draft.to || null}
              onValueChange={(value) => {
                const accountId = value ?? ""
                const next = accounts.find(
                  (account) => account.id === accountId
                )
                setDraft({
                  to: accountId,
                  received:
                    to?.currency !== next?.currency ? "" : draft.received,
                })
              }}
            >
              <SelectTrigger id={`${id}-to`} className="w-full">
                <SelectValue placeholder="Choose an account" />
              </SelectTrigger>
              <SelectContent>
                {destinationItems.map(({ value, label }) => (
                  <SelectItem key={value} value={value}>
                    {label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <Field>
            <FieldLabel htmlFor={`${id}-amount`}>
              Amount sent{from && ` (${from.currency})`}
            </FieldLabel>
            <Input
              id={`${id}-amount`}
              required
              inputMode="decimal"
              value={draft.amount}
              onChange={(event) => setDraft({ amount: event.target.value })}
            />
          </Field>
          {crossCurrency && (
            <Field>
              <FieldLabel htmlFor={`${id}-received`}>
                Amount received ({to.currency})
              </FieldLabel>
              <Input
                id={`${id}-received`}
                required
                inputMode="decimal"
                value={draft.received}
                onChange={(event) => setDraft({ received: event.target.value })}
              />
            </Field>
          )}
          <Field className="sm:col-span-2">
            <FieldLabel htmlFor={`${id}-date`}>
              Transaction date and time
            </FieldLabel>
            <DatePicker
              id={`${id}-date`}
              ariaLabel="Transaction date and time"
              disabled={disabled}
              {...dateTimePickerProps(draft.dateTime, (dateTime) =>
                setDraft({ dateTime })
              )}
            />
          </Field>
          <details
            className="space-y-3 sm:col-span-2"
            open={
              Boolean(
                draft.sourcePosted.date || draft.destinationPosted.date
              ) || undefined
            }
          >
            <summary className="cursor-pointer text-sm font-medium">
              Posting times if different
            </summary>
            <p className="text-sm text-muted-foreground">
              Each account uses the transaction time when its posting time is
              blank.
            </p>
            <Field>
              <FieldLabel htmlFor={`${id}-source-posted`}>
                Source posting date and time
              </FieldLabel>
              <DatePicker
                id={`${id}-source-posted`}
                ariaLabel="Source posting date and time"
                placeholder="Use transaction date and time"
                clearable
                disabled={disabled}
                {...dateTimePickerProps(draft.sourcePosted, (sourcePosted) =>
                  setDraft({ sourcePosted })
                )}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor={`${id}-destination-posted`}>
                Destination posting date and time
              </FieldLabel>
              <DatePicker
                id={`${id}-destination-posted`}
                ariaLabel="Destination posting date and time"
                placeholder="Use transaction date and time"
                clearable
                disabled={disabled}
                {...dateTimePickerProps(
                  draft.destinationPosted,
                  (destinationPosted) => setDraft({ destinationPosted })
                )}
              />
            </Field>
          </details>
          <Field className="sm:col-span-2">
            <FieldLabel htmlFor={`${id}-note`}>Note</FieldLabel>
            <Textarea
              id={`${id}-note`}
              value={draft.note}
              onChange={(event) => setDraft({ note: event.target.value })}
            />
          </Field>
        </div>
      )}
    </fieldset>
  )
})

function optionalPostingUtc(
  value: LocalDateTime,
  original: string | null | undefined,
  timezone?: string | null
) {
  return value.date || value.time ? toUtc(value, original, timezone) : undefined
}

function buildTransferItem(
  target: TransferTarget,
  draft: Draft,
  accounts: AccountSummary[] | undefined,
  emailIds: string[] | undefined,
  timezone?: string | null
): TransferMutationItem {
  const tx = target.transaction
  const editing = tx?.kind === "transfer"
  if (tx && !editing && !emailIds?.length) {
    throw new Error("This transaction has no attached email.")
  }
  const common = {
    email_ids: emailIds,
    expected_transaction_id: emailIds?.length ? tx?.id : undefined,
    card_payment: target.cardPayment || undefined,
  }
  if (draft.mode === "match") {
    if (!draft.matchId) throw new Error("Select the transfer to match.")
    return {
      ...common,
      action: "match",
      match_transaction_id: draft.matchId,
    }
  }
  const from = accounts?.find((account) => account.id === draft.from)
  const to = accounts?.find((account) => account.id === draft.to)
  if (!from || !to || from.id === to.id)
    throw new Error("Choose two different accounts.")
  const amount = parseDecimalToMinorClient(draft.amount, from.currency)
  return {
    ...common,
    action: "record",
    id: editing ? tx.id : undefined,
    request_id: !editing && !emailIds?.length ? draft.requestId : undefined,
    account_id: from.id,
    destination_account_id: to.id,
    amount_minor: amount,
    destination_amount_minor:
      from.currency === to.currency
        ? amount
        : parseDecimalToMinorClient(draft.received, to.currency),
    occurred_at_utc: toUtc(
      draft.dateTime,
      tx?.occurred_at_utc ?? target.occurredAtUtc,
      timezone
    ),
    source_posted_at_utc: optionalPostingUtc(
      draft.sourcePosted,
      tx?.source_posted_at_utc,
      timezone
    ),
    destination_posted_at_utc: optionalPostingUtc(
      draft.destinationPosted,
      tx?.destination_posted_at_utc,
      timezone
    ),
    note: draft.note.trim() || null,
  }
}

export function TransferFormDialog({
  targets,
  timezone,
  onClose,
  onSuccess,
  trigger,
}: {
  targets: TransferTarget[]
  timezone?: string | null
  onClose?: () => void
  onSuccess?: () => void
  trigger?: ReactNode
}) {
  const formId = useId()
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(!trigger)
  const accounts = useQuery({ ...accountsQuery(), enabled: open })
  const [drafts, setDrafts] = useState(() =>
    trigger ? [] : targets.map((target) => initialDraft(target, timezone))
  )
  const updateDraft = useCallback((index: number, patch: Partial<Draft>) => {
    setDrafts((current) =>
      current.map((item, position) =>
        position === index ? { ...item, ...patch } : item
      )
    )
  }, [])
  const saved = useRef(false)
  const evidenceIds = useRef(new Map<string, string[]>())
  const mutation = useMutation({
    mutationFn: async () => {
      const items = await Promise.all(
        targets.map(async (target, index): Promise<TransferMutationItem> => {
          const draft = drafts[index]
          const tx = target.transaction
          const editing = tx?.kind === "transfer"
          let emailIds = target.skippedEmailId
            ? [target.skippedEmailId]
            : target.emailIds
          if (tx && !editing && !emailIds) {
            emailIds = evidenceIds.current.get(tx.id)
            if (!emailIds) {
              const detail = await queryClient.fetchQuery(
                transactionInspectorQuery(tx.id)
              )
              emailIds = detail.sources.map((source) => source.email_id)
              evidenceIds.current.set(tx.id, emailIds)
            }
          }
          return buildTransferItem(
            target,
            draft,
            accounts.data,
            emailIds,
            timezone
          )
        })
      )
      return saveTransfersApi(items)
    },
    onSuccess: () => {
      invalidateTriageRelatedQueries(queryClient)
      saved.current = true
      setOpen(false)
    },
  })

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!mutation.isPending) mutation.mutate()
  }

  const single = targets.length === 1 ? targets[0] : null
  const title = single?.cardPayment
    ? "Mark as card payment"
    : single?.transaction?.kind === "transfer"
      ? "Edit transfer"
      : single && !single.transaction
        ? "Transfer"
        : "Mark as transfer"

  return (
    <CreateFormDrawer
      open={open}
      trigger={trigger}
      onOpenChange={(nextOpen) => {
        if (mutation.isPending) return
        if (nextOpen && trigger) {
          saved.current = false
          evidenceIds.current.clear()
          setDrafts(targets.map((target) => initialDraft(target, timezone)))
          mutation.reset()
        }
        setOpen(nextOpen)
      }}
      onOpenChangeComplete={(nextOpen) => {
        if (nextOpen) return
        if (saved.current) onSuccess?.()
        onClose?.()
      }}
      formId={formId}
      title={title}
      description={`Account balances reflect each transfer. Transfers stay out of net spending. Times use ${timezone || "UTC"}.`}
      onSubmit={submit}
      submitLabel={mutation.isPending ? "Saving…" : "Confirm"}
      submitDisabled={mutation.isPending || !accounts.data}
    >
      {accounts.isPending && (
        <p role="status" className="text-sm">
          Loading accounts…
        </p>
      )}
      {(mutation.isError || accounts.isError) && (
        <Alert variant="destructive">
          <AlertDescription>
            {mutation.error?.message ?? accounts.error?.message}
          </AlertDescription>
        </Alert>
      )}
      {accounts.data &&
        drafts.map((draft, index) => (
          <TransferFields
            key={draft.requestId}
            draft={draft}
            target={targets[index]}
            accounts={accounts.data}
            timezone={timezone}
            disabled={mutation.isPending}
            index={index}
            updateDraft={updateDraft}
          />
        ))}
    </CreateFormDrawer>
  )
}
