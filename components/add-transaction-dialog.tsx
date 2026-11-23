"use client"

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useEffect, useMemo, useState, type FormEvent } from "react"

import { DatePicker } from "@/components/date-picker"
import { DynamicIcon } from "@/components/dynamic-icon"
import { TagMultiSelect } from "@/components/tag-multi-select"
import { Alert, AlertDescription } from "@/components/ui/alert"
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  addManualTransactionApi,
  categoriesQuery,
  tagsQuery,
  type AccountSummary,
} from "@/lib/api"
import { DEFAULT_CATEGORY_DEFINITIONS } from "@/lib/categories"
import {
  dateInputToUtcIso,
  parseDecimalToMinorClient,
  utcToDateInputValue,
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
  accounts: AccountSummary[]
  currency: string
  timezone: string | null | undefined
}) {
  const queryClient = useQueryClient()
  const catListQuery = useQuery(categoriesQuery())
  const tagListQuery = useQuery(tagsQuery())

  const [amount, setAmount] = useState("")
  const [merchant, setMerchant] = useState("")
  const [category, setCategory] = useState<string>("Other")
  const [selectedTagIds, setSelectedTagIds] = useState<string[]>([])
  const [accountId, setAccountId] = useState(UNASSIGNED_ACCOUNT)
  const [date, setDate] = useState("")
  const [note, setNote] = useState("")
  const [formError, setFormError] = useState<string | null>(null)

  const categoryIconMap = useMemo(() => {
    const map = new Map<string, string>(
      DEFAULT_CATEGORY_DEFINITIONS.map((c) => [c.name, c.icon])
    )
    for (const cat of catListQuery.data ?? []) {
      map.set(cat.name, cat.icon)
    }
    return map
  }, [catListQuery.data])

  const categoryOptions = useMemo(() => {
    const opts = (catListQuery.data ?? DEFAULT_CATEGORY_DEFINITIONS).map(
      (c) => ({
        value: c.name,
        label: c.name,
      })
    )
    if (category && !opts.some((opt) => opt.value === category)) {
      opts.push({ value: category, label: category })
    }
    return opts
  }, [catListQuery.data, category])

  const accountItems = useMemo(
    () => [
      { value: UNASSIGNED_ACCOUNT, label: "Unassigned" },
      ...accounts.map((acct) => ({ value: acct.id, label: acct.name })),
    ],
    [accounts]
  )

  useEffect(() => {
    if (!open) return
    setAmount("")
    setMerchant("")
    setCategory("Other")
    setSelectedTagIds([])
    setAccountId(UNASSIGNED_ACCOUNT)
    setDate(utcToDateInputValue(new Date().toISOString(), timezone))
    setNote("")
    setFormError(null)
  }, [open, timezone])

  const addMutation = useMutation({
    mutationFn: addManualTransactionApi,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["spending"] })
      void queryClient.invalidateQueries({ queryKey: ["transactions"] })
      void queryClient.invalidateQueries({ queryKey: ["accounts"] })
      void queryClient.invalidateQueries({ queryKey: ["categories"] })
      void queryClient.invalidateQueries({ queryKey: ["tags"] })
      onOpenChange(false)
    },
    onError: (err) => {
      setFormError(
        err instanceof Error ? err.message : "Failed to add transaction."
      )
    },
  })

  function handleSubmit(event: FormEvent) {
    event.preventDefault()
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
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Add manual transaction</DialogTitle>
          <DialogDescription>
            Record a cash or manual expense in {currency}.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4">
          {formError && (
            <Alert variant="destructive">
              <AlertDescription>{formError}</AlertDescription>
            </Alert>
          )}

          <div className="grid gap-4 sm:grid-cols-2">
            <Field>
              <FieldLabel>Amount ({currency})</FieldLabel>
              <Input
                type="text"
                inputMode="decimal"
                required
                placeholder="0.00"
                value={amount}
                onChange={(e) => setAmount(e.target.value)}
              />
            </Field>

            <Field>
              <FieldLabel>Merchant</FieldLabel>
              <Input
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
                onValueChange={(v) => setCategory(v ?? "Other")}
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
              <FieldLabel>Tags</FieldLabel>
              <TagMultiSelect
                tags={tagListQuery.data ?? []}
                selectedTagIds={selectedTagIds}
                onChange={setSelectedTagIds}
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
              <FieldLabel>Note</FieldLabel>
              <Input
                type="text"
                placeholder="Optional note"
                value={note}
                onChange={(e) => setNote(e.target.value)}
              />
            </Field>
          </div>

          <DialogFooter>
            <DialogClose render={<Button type="button" variant="outline" />}>
              Cancel
            </DialogClose>
            <Button type="submit" disabled={addMutation.isPending}>
              {addMutation.isPending ? "Adding..." : "Add transaction"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
