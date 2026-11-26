"use client"

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useId, useRef, useState, type SubmitEvent } from "react"

import { CreateFormDrawer } from "@/components/detail-drawer-content"
import { PaginationControls } from "@/components/pagination-controls"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { accountsQuery } from "@/lib/api/accounts"
import { transactionInspectorQuery } from "@/lib/api/transaction-inspector"
import {
  saveTransfersApi,
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
  accountId?: string | null
  amountMinor?: number
  currency?: string
  occurredAtUtc?: string
  cardPayment?: boolean
}

type Draft = {
  requestId: string
  mode: "record" | "match"
  from: string
  to: string
  amount: string
  received: string
  dateTime: string
  sourcePosted: string
  destinationPosted: string
  note: string
  matchId: string
}

function localInput(
  instant: string | null | undefined,
  timezone?: string | null
) {
  if (!instant) return ""
  const date = utcToDateInputValue(instant, timezone)
  const time = utcToTimeInputValue(instant, timezone)
  return date && time ? `${date}T${time}:${instant.slice(17, 19)}` : ""
}

function toUtc(
  value: string,
  original: string | null | undefined,
  timezone?: string | null
) {
  if (original && localInput(original, timezone) === value) return original
  const [date = "", time = ""] = value.split("T")
  return dateTimeInputToUtcIso(date, time, timezone)
}

function initialDraft(target: TransferTarget, timezone?: string | null): Draft {
  const tx = target.transaction
  const original = tx?.occurred_at_utc ?? target.occurredAtUtc
  const amount = tx?.amount_minor ?? target.amountMinor
  const currency = tx?.currency ?? target.currency
  return {
    requestId: crypto.randomUUID(),
    mode: "record",
    from: tx?.account_id ?? target.accountId ?? "",
    to: tx?.destination_account_id ?? "",
    amount: amount && currency ? minorToDecimalInput(amount, currency) : "",
    received:
      tx?.destination_amount_minor != null && tx.destination_currency
        ? minorToDecimalInput(
            tx.destination_amount_minor,
            tx.destination_currency
          )
        : "",
    dateTime:
      localInput(original, timezone) || `${getTodayDateString(timezone)}T00:00`,
    sourcePosted: localInput(tx?.source_posted_at_utc, timezone),
    destinationPosted: localInput(tx?.destination_posted_at_utc, timezone),
    note: tx?.note ?? "",
    matchId: "",
  }
}

function TransferFields({
  draft,
  setDraft,
  target,
  accounts,
  disabled,
  timezone,
}: {
  draft: Draft
  setDraft: (patch: Partial<Draft>) => void
  target: TransferTarget
  accounts: AccountSummary[]
  disabled: boolean
  timezone?: string | null
}) {
  const id = useId()
  const selectClass =
    "h-9 w-full rounded-md border border-input bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
  const [matchPage, setMatchPage] = useState(1)
  const [showEvidence, setShowEvidence] = useState(false)
  const matches = useQuery({
    ...transactionsQuery({
      kind: "transfer",
      ledgerRange: "all",
      showSkipped: false,
      page: matchPage,
    }),
    enabled: draft.mode === "match",
  })
  const evidence = useQuery(
    skippedEmailBodyQuery(target.skippedEmailId ?? "", showEvidence)
  )
  const hasEmailEvidence = Boolean(
    target.emailIds?.length ||
    target.skippedEmailId ||
    (target.transaction?.kind === "expense" &&
      target.transaction.sourceCount > 0)
  )
  const from = accounts.find((account) => account.id === draft.from)
  const to = accounts.find((account) => account.id === draft.to)
  const crossCurrency = from && to && from.currency !== to.currency
  const cardIds = new Set(
    accounts
      .filter((account) => account.type === "credit_card")
      .map((account) => account.id)
  )
  const candidates = (matches.data?.items ?? []).filter(
    (item) =>
      !target.cardPayment ||
      (item.destination_account_id && cardIds.has(item.destination_account_id))
  )

  return (
    <fieldset disabled={disabled} className="space-y-4 rounded-lg border p-4">
      {target.title && (
        <legend className="px-1 text-sm font-medium">{target.title}</legend>
      )}
      {target.skippedEmailId && (
        <details
          onToggle={(event) => setShowEvidence(event.currentTarget.open)}
        >
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
      )}
      {hasEmailEvidence && (
        <Field>
          <FieldLabel htmlFor={`${id}-action`}>Action</FieldLabel>
          <select
            className={selectClass}
            id={`${id}-action`}
            value={draft.mode}
            onChange={(event) =>
              setDraft({
                mode: event.target.value === "match" ? "match" : "record",
              })
            }
          >
            <option value="record">Record transfer</option>
            <option value="match">Match an existing transfer</option>
          </select>
        </Field>
      )}
      {draft.mode === "match" ? (
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
                name={`${id}-match`}
                value={item.id}
                checked={draft.matchId === item.id}
                onChange={() => setDraft({ matchId: item.id })}
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
                    (matches.data?.pageSize || 25)
                )
              )}
              isLoading={matches.isFetching}
              onPageChange={setMatchPage}
            />
          </div>
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2">
          <Field>
            <FieldLabel htmlFor={`${id}-from`}>From account</FieldLabel>
            <select
              className={selectClass}
              id={`${id}-from`}
              required
              value={draft.from}
              onChange={(event) => {
                const id = event.target.value
                const next = accounts.find((account) => account.id === id)
                const currencyChanged =
                  (from?.currency ??
                    target.currency ??
                    target.transaction?.currency) !== next?.currency
                setDraft({
                  from: id,
                  to: draft.to === id ? "" : draft.to,
                  amount:
                    currencyChanged &&
                    (from || target.currency || target.transaction)
                      ? ""
                      : draft.amount,
                  received: draft.to === id ? "" : draft.received,
                })
              }}
            >
              <option value="">Choose an account</option>
              {accounts.map((account) => (
                <option key={account.id} value={account.id}>
                  {account.name} ({account.currency})
                </option>
              ))}
            </select>
          </Field>
          <Field>
            <FieldLabel htmlFor={`${id}-to`}>
              {target.cardPayment ? "Credit card" : "To account"}
            </FieldLabel>
            <select
              className={selectClass}
              id={`${id}-to`}
              required
              value={draft.to}
              onChange={(event) => {
                const id = event.target.value
                const next = accounts.find((account) => account.id === id)
                setDraft({
                  to: id,
                  received:
                    to?.currency !== next?.currency ? "" : draft.received,
                })
              }}
            >
              <option value="">Choose an account</option>
              {accounts
                .filter(
                  (account) =>
                    account.id !== draft.from &&
                    (!target.cardPayment || account.type === "credit_card")
                )
                .map((account) => (
                  <option key={account.id} value={account.id}>
                    {account.name} ({account.currency})
                  </option>
                ))}
            </select>
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
            <Input
              id={`${id}-date`}
              type="datetime-local"
              step={1}
              required
              value={draft.dateTime}
              onChange={(event) => setDraft({ dateTime: event.target.value })}
            />
          </Field>
          <details
            className="space-y-3 sm:col-span-2"
            open={
              Boolean(draft.sourcePosted || draft.destinationPosted) ||
              undefined
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
              <Input
                id={`${id}-source-posted`}
                type="datetime-local"
                step={1}
                value={draft.sourcePosted}
                onChange={(event) =>
                  setDraft({ sourcePosted: event.target.value })
                }
              />
            </Field>
            <Field>
              <FieldLabel htmlFor={`${id}-destination-posted`}>
                Destination posting date and time
              </FieldLabel>
              <Input
                id={`${id}-destination-posted`}
                type="datetime-local"
                step={1}
                value={draft.destinationPosted}
                onChange={(event) =>
                  setDraft({ destinationPosted: event.target.value })
                }
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
}

export function TransferFormDialog({
  targets,
  timezone,
  onClose,
  onSuccess,
}: {
  targets: TransferTarget[]
  timezone?: string | null
  onClose: () => void
  onSuccess?: () => void
}) {
  const formId = useId()
  const queryClient = useQueryClient()
  const accounts = useQuery(accountsQuery())
  const [drafts, setDrafts] = useState(() =>
    targets.map((target) => initialDraft(target, timezone))
  )
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
          const from = accounts.data?.find(
            (account) => account.id === draft.from
          )
          const to = accounts.data?.find((account) => account.id === draft.to)
          if (!from || !to || from.id === to.id)
            throw new Error("Choose two different accounts.")
          const amount = parseDecimalToMinorClient(draft.amount, from.currency)
          return {
            ...common,
            action: "record",
            id: editing ? tx.id : undefined,
            request_id:
              !editing && !emailIds?.length ? draft.requestId : undefined,
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
            source_posted_at_utc: draft.sourcePosted
              ? toUtc(draft.sourcePosted, tx?.source_posted_at_utc, timezone)
              : undefined,
            destination_posted_at_utc: draft.destinationPosted
              ? toUtc(
                  draft.destinationPosted,
                  tx?.destination_posted_at_utc,
                  timezone
                )
              : undefined,
            note: draft.note.trim() || null,
          }
        })
      )
      return saveTransfersApi(items)
    },
    onSuccess: () => {
      invalidateTriageRelatedQueries(queryClient)
      onSuccess?.()
      onClose()
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
      open
      onOpenChange={(open) => {
        if (!open && !mutation.isPending) onClose()
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
            setDraft={(patch) =>
              setDrafts((current) =>
                current.map((item, position) =>
                  position === index ? { ...item, ...patch } : item
                )
              )
            }
          />
        ))}
    </CreateFormDrawer>
  )
}
