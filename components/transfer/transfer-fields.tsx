import { memo, useId } from "react"
import { DatePicker } from "@/components/date-picker"
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
import type { AccountSummary } from "@/lib/api/types.generated"
import {
  SkippedPaymentEvidence,
  TransferMatchPicker,
} from "./transfer-match-picker"
import {
  type Draft,
  type TransferTarget,
  TRANSFER_ACTION_OPTIONS,
  dateTimePickerProps,
} from "./transfer-draft"

export const TransferFields = memo(function TransferFields({
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
