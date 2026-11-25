"use client"

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useId, useMemo, useState, type SubmitEvent } from "react"

import { DatePicker } from "@/components/date-picker"
import { CreateFormDrawer } from "@/components/detail-drawer-content"
import { DynamicIcon } from "@/components/dynamic-icon"
import { TagMultiSelect } from "@/components/tag-multi-select"
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
import { categoriesQuery } from "@/lib/api/categories"
import { tagsQuery } from "@/lib/api/tags"
import {
  addManualTransactionApi,
  invalidateTransactionRelatedQueries,
} from "@/lib/api/transactions"
import { type AccountOption } from "@/lib/api/types.generated"
import {
  DEFAULT_CATEGORY,
  buildCategoryIconMap,
  buildCategorySelectOptions,
} from "@/lib/categories"
import {
  dateInputToUtcIso,
  getTodayDateString,
  parseDecimalToMinorClient,
} from "@/lib/format"

const UNASSIGNED_ACCOUNT = "__unassigned__"

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

  const categoryIconMap = useMemo(
    () => buildCategoryIconMap(catListQuery.data),
    [catListQuery.data]
  )

  const categoryOptions = useMemo(
    () => buildCategorySelectOptions(catListQuery.data, category),
    [catListQuery.data, category]
  )

  const accountItems = useMemo(
    () => [
      { value: UNASSIGNED_ACCOUNT, label: "Unassigned" },
      ...accounts.map((acct) => ({ value: acct.id, label: acct.name })),
    ],
    [accounts]
  )

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

  function handleSubmit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!open || addMutation.isPending || tagCreatePending) return
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
      submitLabel={addMutation.isPending ? "Adding..." : "Add transaction"}
      submitDisabled={addMutation.isPending || tagCreatePending}
    >
      <form id={formId} onSubmit={handleSubmit} className="space-y-4">
        {formError && (
          <Alert variant="destructive">
            <AlertDescription>{formError}</AlertDescription>
          </Alert>
        )}

        <div className="grid gap-4 sm:grid-cols-2">
          <Field>
            <FieldLabel htmlFor={`${formId}-amount`}>
              Amount ({currency})
            </FieldLabel>
            <Input
              id={`${formId}-amount`}
              type="text"
              inputMode="decimal"
              required
              placeholder="0.00"
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
            />
          </Field>

          <Field>
            <FieldLabel htmlFor={`${formId}-merchant`}>Merchant</FieldLabel>
            <Input
              id={`${formId}-merchant`}
              type="text"
              required
              placeholder="Coffee Shop"
              value={merchant}
              onChange={(e) => setMerchant(e.target.value)}
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
            {open && (
              <TagMultiSelect
                key={String(timezone)}
                tags={tagListQuery.data ?? []}
                selectedTagIds={selectedTagIds}
                onChange={setSelectedTagIds}
                onCreatePendingChange={setTagCreatePending}
                disabled={addMutation.isPending}
              />
            )}
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

          <Field>
            <FieldLabel>Date</FieldLabel>
            <DatePicker
              value={date}
              onChange={setDate}
              ariaLabel="Transaction date"
            />
          </Field>

          <Field>
            <FieldLabel htmlFor={`${formId}-note`}>Note</FieldLabel>
            <Input
              id={`${formId}-note`}
              type="text"
              placeholder="Optional note"
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />
          </Field>
        </div>
      </form>
    </CreateFormDrawer>
  )
}
