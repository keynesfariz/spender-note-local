"use client"

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useId, useState, type SubmitEvent } from "react"

import { CreateFormDrawer } from "@/components/detail-drawer-content"
import {
  TransactionFormFields,
  UNASSIGNED_ACCOUNT,
} from "@/components/transaction-form-fields"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { categoriesQuery } from "@/lib/api/categories"
import { tagsQuery } from "@/lib/api/tags"
import {
  addManualTransactionApi,
  invalidateTransactionRelatedQueries,
} from "@/lib/api/transactions"
import { type AccountOption } from "@/lib/api/types.generated"
import { DEFAULT_CATEGORY } from "@/lib/categories"
import {
  dateInputToUtcIso,
  getTodayDateString,
  parseDecimalToMinorClient,
} from "@/lib/format"

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

  const [amount, setAmount] = useState("")
  const [merchant, setMerchant] = useState("")
  const [category, setCategory] = useState<string>(DEFAULT_CATEGORY)
  const [selectedTagIds, setSelectedTagIds] = useState<string[]>([])
  const [tagCreatePending, setTagCreatePending] = useState(false)
  const [accountId, setAccountId] = useState(UNASSIGNED_ACCOUNT)
  const [date, setDate] = useState("")
  const [note, setNote] = useState("")
  const [formError, setFormError] = useState<string | null>(null)

  const [previous, setPrevious] = useState({ open: false, timezone })
  if (previous.open !== open || previous.timezone !== timezone) {
    setPrevious({ open, timezone })
    setTagCreatePending(false)
    if (open) {
      setAmount("")
      setMerchant("")
      setCategory(DEFAULT_CATEGORY)
      setSelectedTagIds([])
      setAccountId(UNASSIGNED_ACCOUNT)
      setDate(getTodayDateString(timezone))
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

  const submitDisabled = addMutation.isPending || tagCreatePending

  function handleSubmit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!open || submitDisabled) return
    setFormError(null)
    try {
      const amountMinor = parseDecimalToMinorClient(amount, currency)
      const occurredAtUtc = dateInputToUtcIso(date, timezone)
      addMutation.mutate({
        amount_minor: amountMinor,
        occurred_at_utc: occurredAtUtc,
        merchant: merchant.trim(),
        category,
        tag_ids: selectedTagIds,
        account_id: accountId !== UNASSIGNED_ACCOUNT ? accountId : null,
        note: note.trim() || null,
      })
    } catch (err) {
      setFormError(err instanceof Error ? err.message : "Invalid input.")
    }
  }

  return (
    <CreateFormDrawer
      open={open}
      onOpenChange={onOpenChange}
      title="Add manual transaction"
      description={`Record a cash or manual expense in ${currency}.`}
      formId={formId}
      onSubmit={handleSubmit}
      submitLabel={addMutation.isPending ? "Adding..." : "Add transaction"}
      submitDisabled={submitDisabled}
    >
      {formError && (
        <Alert variant="destructive">
          <AlertDescription>{formError}</AlertDescription>
        </Alert>
      )}

      <TransactionFormFields
        description={{
          kind: "merchant",
          value: merchant,
          onChange: setMerchant,
          placeholder: "Coffee Shop",
        }}
        currency={currency}
        amount={amount}
        onAmountChange={setAmount}
        amountInputProps={{ placeholder: "0.00" }}
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
        onAccountChange={setAccountId}
        date={date}
        onDateChange={setDate}
        dateLabel="Date"
        dateAriaLabel="Transaction date"
        note={{ value: note, onChange: setNote }}
      />
    </CreateFormDrawer>
  )
}
