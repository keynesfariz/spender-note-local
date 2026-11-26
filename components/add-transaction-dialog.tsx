"use client"

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useId, useState, type SubmitEvent } from "react"
import { flushSync } from "react-dom"

import { CreateFormDrawer } from "@/components/detail-drawer-content"
import {
  TransactionForm,
  TransactionFormFields,
  UNASSIGNED_ACCOUNT,
} from "@/components/transaction-form"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Field, FieldLabel } from "@/components/ui/field"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { accountsQuery } from "@/lib/api/accounts"
import { categoriesQuery } from "@/lib/api/categories"
import { tagsQuery } from "@/lib/api/tags"
import {
  addManualTransactionApi,
  invalidateTransactionRelatedQueries,
} from "@/lib/api/transactions"
import { type AccountOption } from "@/lib/api/types.generated"
import { DEFAULT_CATEGORY } from "@/lib/categories"
import {
  dateTimeInputToUtcIso,
  getTodayDateString,
  groupAmountInput,
  parseDecimalToMinorClient,
} from "@/lib/format"

const TRANSACTION_TYPE_OPTIONS = [
  { value: "expense", label: "Expense" },
  { value: "income", label: "Income" },
] as const

export function AddTransactionDialog({
  open,
  onOpenChange,
  accounts,
  currency,
  timezone,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  accounts: AccountOption[]
  currency: string
  timezone: string | null | undefined
}) {
  const queryClient = useQueryClient()
  const formId = useId()
  const catListQuery = useQuery({ ...categoriesQuery(), enabled: open })
  const tagListQuery = useQuery({ ...tagsQuery(), enabled: open })

  const [kind, setKind] = useState<"expense" | "income">("expense")
  const accountQuery = useQuery({
    ...accountsQuery(),
    enabled: open && kind === "income",
  })
  const [amount, setAmount] = useState("")
  const [remark, setRemark] = useState("")
  const [category, setCategory] = useState<string>(DEFAULT_CATEGORY)
  const [selectedTagIds, setSelectedTagIds] = useState<string[]>([])
  const [tagCreatePending, setTagCreatePending] = useState(false)
  const [accountId, setAccountId] = useState(UNASSIGNED_ACCOUNT)
  const [date, setDate] = useState("")
  const [time, setTime] = useState("00:00")
  const [tab, setTab] = useState("details")
  const [note, setNote] = useState("")
  const [formError, setFormError] = useState<string | null>(null)

  const [previous, setPrevious] = useState({ open: false, timezone })
  if (previous.open !== open || previous.timezone !== timezone) {
    setPrevious({ open, timezone })
    setTagCreatePending(false)
    if (open) {
      setKind("expense")
      setAmount("")
      setRemark("")
      setCategory(DEFAULT_CATEGORY)
      setSelectedTagIds([])
      setAccountId(UNASSIGNED_ACCOUNT)
      setDate(getTodayDateString(timezone))
      setTime("00:00")
      setTab("details")
      setNote("")
      setFormError(null)
    }
  }

  const addMutation = useMutation({
    mutationFn: addManualTransactionApi,
    onSuccess: () => {
      invalidateTransactionRelatedQueries(queryClient)
      onOpenChange(false)
    },
    onError: (err) => {
      setFormError(
        err instanceof Error ? err.message : "Failed to add transaction."
      )
    },
  })

  const receivingAccount = accountQuery.data?.find(
    (account) => account.id === accountId
  )
  const entryCurrency =
    kind === "income" ? (receivingAccount?.currency ?? currency) : currency
  const submitDisabled =
    addMutation.isPending ||
    tagCreatePending ||
    (kind === "income" && !receivingAccount)

  function handleSubmit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!open || submitDisabled) return
    setFormError(null)
    try {
      const amountMinor = parseDecimalToMinorClient(amount, entryCurrency)
      const occurredAtUtc = dateTimeInputToUtcIso(date, time, timezone)
      addMutation.mutate({
        kind,
        amount_minor: amountMinor,
        occurred_at_utc: occurredAtUtc,
        merchant: remark.trim() || (kind === "income" ? "Income" : ""),
        category,
        tag_ids: selectedTagIds,
        account_id: accountId !== UNASSIGNED_ACCOUNT ? accountId : null,
        note: note.trim() || null,
      })
    } catch (err) {
      setTab("details")
      setFormError(err instanceof Error ? err.message : "Invalid input.")
    }
  }

  return (
    <CreateFormDrawer
      open={open}
      onOpenChange={onOpenChange}
      title="Add manual transaction"
      description="Record an expense or credit a receiving account with income."
      formId={formId}
      onSubmit={handleSubmit}
      onInvalid={() => {
        if (tab !== "details") {
          // Reveal the invalid input before the browser tries to focus it.
          flushSync(() => setTab("details"))
        }
      }}
      submitLabel={addMutation.isPending ? "Adding..." : "Add transaction"}
      submitDisabled={submitDisabled}
    >
      {formError && (
        <Alert variant="destructive">
          <AlertDescription>{formError}</AlertDescription>
        </Alert>
      )}

      <TransactionForm
        value={tab}
        onValueChange={setTab}
        note={{ value: note, onChange: setNote }}
        details={
          <div className="space-y-4">
            <Field>
              <FieldLabel htmlFor={`${formId}-kind`}>
                Transaction type
              </FieldLabel>
              <Select
                items={TRANSACTION_TYPE_OPTIONS}
                value={kind}
                onValueChange={(value) => {
                  if (value) setKind(value)
                }}
              >
                <SelectTrigger id={`${formId}-kind`}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {TRANSACTION_TYPE_OPTIONS.map(({ value, label }) => (
                    <SelectItem key={value} value={value}>
                      {label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            {kind === "income" && accountQuery.isError && (
              <Alert variant="destructive">
                <AlertDescription>
                  {accountQuery.error.message}
                </AlertDescription>
              </Alert>
            )}
            <TransactionFormFields
              description={{
                required: kind === "expense",
                value: remark,
                onChange: setRemark,
                placeholder: kind === "income" ? "Optional" : "Coffee Shop",
              }}
              currency={entryCurrency}
              amount={amount}
              onAmountChange={setAmount}
              amountInputProps={{
                onBlur: () => setAmount(groupAmountInput(amount)),
                className: "tabular-nums",
                placeholder: "0.00",
              }}
              categories={catListQuery.data}
              category={category}
              onCategoryChange={setCategory}
              tagPicker={
                open
                  ? {
                      tags: tagListQuery.data ?? [],
                      selectedTagIds,
                      onChange: setSelectedTagIds,
                      onCreatePendingChange: setTagCreatePending,
                      disabled: addMutation.isPending,
                    }
                  : undefined
              }
              tagPickerKey={String(timezone)}
              accounts={accounts}
              accountId={accountId}
              receivingAccount={kind === "income"}
              onAccountChange={setAccountId}
              date={date}
              onDateChange={setDate}
              dateLabel="Occurred date and time"
              time={{ value: time, onChange: setTime }}
            />
          </div>
        }
      />
    </CreateFormDrawer>
  )
}
