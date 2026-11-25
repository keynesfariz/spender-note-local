"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState, type SubmitEvent } from "react"

import { DatePicker } from "@/components/date-picker"
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
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { createAccountApi, updateAccountApi } from "@/lib/api/accounts"
import {
  type AccountIdentifierInput,
  type AccountIdentifierKind,
  type AccountSummary,
  type AccountType,
} from "@/lib/api/types.generated"
import {
  dateInputToUtcIso,
  getTodayDateString,
  minorToDecimalInput,
  parseDecimalToMinorClient,
  utcToDateInputValue,
} from "@/lib/format"

interface IdentifierDraft {
  key: string
  kind: AccountIdentifierKind
  value: string
}

const IDENTIFIER_PLACEHOLDERS: Record<AccountIdentifierKind, string> = {
  card_last4: "1234",
  bank_account: "e.g. 1234-5678",
  alias: "e.g. GoPay Coins",
}

const ACCOUNT_TYPE_OPTIONS: ReadonlyArray<{
  value: AccountType
  label: string
}> = [
  { value: "credit_card", label: "Credit Card" },
  { value: "debit_card", label: "Debit Card" },
  { value: "digital_wallet", label: "Digital Wallet" },
]

const IDENTIFIER_KIND_OPTIONS: ReadonlyArray<{
  value: AccountIdentifierKind
  label: string
}> = [
  { value: "card_last4", label: "Card last 4" },
  { value: "bank_account", label: "Bank account" },
  { value: "alias", label: "Alias / Wallet" },
]

export function AccountFormDialog({
  open,
  onOpenChange,
  account,
  currency,
  timezone,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  account?: AccountSummary | null
  currency: string
  timezone?: string | null
}) {
  const queryClient = useQueryClient()

  const [name, setName] = useState("")
  const [type, setType] = useState<AccountType>("credit_card")
  const [initialBalance, setInitialBalance] = useState("0")
  const [initialBalanceAsOf, setInitialBalanceAsOf] = useState("")
  const [statementDay, setStatementDay] = useState("")
  const [identifiers, setIdentifiers] = useState<IdentifierDraft[]>([])
  const [formError, setFormError] = useState<string | null>(null)

  const [previous, setPrevious] = useState({
    open: false,
    account,
    currency,
    timezone,
  })
  const inputsChanged =
    previous.open !== open ||
    previous.account !== account ||
    previous.currency !== currency ||
    previous.timezone !== timezone
  if (inputsChanged) {
    setPrevious({ open, account, currency, timezone })
  }
  if (inputsChanged && open) {
    setFormError(null)
    if (account) {
      setName(account.name)
      setType(account.type)
      setInitialBalance(
        account.initial_balance_minor === null
          ? ""
          : minorToDecimalInput(
              account.initial_balance_minor,
              account.currency || currency
            )
      )
      setInitialBalanceAsOf(
        utcToDateInputValue(account.initial_balance_as_of_utc, timezone)
      )
      setStatementDay(
        account.statement_day !== null && account.statement_day !== undefined
          ? String(account.statement_day)
          : ""
      )
      const existing = account.identifiers ?? []
      if (existing.length > 0) {
        setIdentifiers(
          existing.map((id) => ({ key: id.id, kind: id.kind, value: id.value }))
        )
      } else if (account.last4) {
        setIdentifiers([
          {
            key: "legacy-last4",
            kind: "card_last4",
            value: account.last4,
          },
        ])
      } else {
        setIdentifiers([])
      }
    } else {
      setName("")
      setType("credit_card")
      setInitialBalance("0")
      setInitialBalanceAsOf(getTodayDateString(timezone))
      setStatementDay("")
      setIdentifiers([])
    }
  }

  const saveMutation = useMutation({
    mutationFn: async (payload: {
      id?: string
      name: string
      type: AccountType
      initial_balance_minor: number | null
      initial_balance_as_of_utc?: string
      statement_day: number | null
      identifiers: AccountIdentifierInput[]
    }) => {
      if (payload.id) {
        return updateAccountApi({ ...payload, id: payload.id })
      }
      return createAccountApi({
        ...payload,
        initial_balance_minor: payload.initial_balance_minor ?? 0,
      })
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["accounts"] })
      void queryClient.invalidateQueries({ queryKey: ["spending"] })
      void queryClient.invalidateQueries({ queryKey: ["transactions"] })
      void queryClient.invalidateQueries({ queryKey: ["senders"] })
      onOpenChange(false)
    },
    onError: (err) => {
      setFormError(
        err instanceof Error ? err.message : "Could not save account."
      )
    },
  })

  function handleSubmit(e: SubmitEvent<HTMLFormElement>) {
    e.preventDefault()
    setFormError(null)

    const trimmedName = name.trim()
    if (!trimmedName) {
      setFormError("Account name is required.")
      return
    }

    const cleanedIdentifiers: AccountIdentifierInput[] = []
    for (const row of identifiers) {
      const trimmed = row.value.trim()
      if (!trimmed) continue
      if (row.kind === "card_last4" && !/^\d{4}$/.test(trimmed)) {
        setFormError("Card last 4 digits must be exactly 4 numeric digits.")
        return
      }
      if (
        row.kind === "bank_account" &&
        trimmed.replace(/\D/g, "").length < 4
      ) {
        setFormError("Bank account number must contain at least 4 digits.")
        return
      }
      cleanedIdentifiers.push({ kind: row.kind, value: trimmed })
    }

    let initialBalanceMinor: number | null
    if (account && initialBalance.trim() === "") {
      initialBalanceMinor = null
    } else {
      try {
        initialBalanceMinor = parseDecimalToMinorClient(
          initialBalance,
          account?.currency || currency
        )
      } catch (err) {
        setFormError(
          err instanceof Error ? err.message : "Invalid initial balance."
        )
        return
      }
    }

    let asOfUtc: string | undefined
    try {
      if (
        !account ||
        initialBalanceAsOf !==
          utcToDateInputValue(account.initial_balance_as_of_utc, timezone)
      ) {
        asOfUtc = dateInputToUtcIso(initialBalanceAsOf, timezone)
      }
    } catch (err) {
      setFormError(err instanceof Error ? err.message : "Invalid date.")
      return
    }

    let parsedStatementDay: number | null = null
    if (type === "credit_card" && statementDay.trim() !== "") {
      const n = Number(statementDay.trim())
      if (!Number.isInteger(n) || n < 1 || n > 31) {
        setFormError("Statement day must be a whole number between 1 and 31.")
        return
      }
      parsedStatementDay = n
    }

    saveMutation.mutate({
      id: account?.id,
      name: trimmedName,
      type,
      initial_balance_minor: initialBalanceMinor,
      initial_balance_as_of_utc: asOfUtc,
      statement_day: parsedStatementDay,
      identifiers: cleanedIdentifiers,
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{account ? "Edit account" : "Add account"}</DialogTitle>
          <DialogDescription>
            {account
              ? "Update account details and starting balance."
              : `Track balances in ${currency}.`}
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4">
          <Field>
            <FieldLabel>Account name</FieldLabel>
            <Input
              type="text"
              required
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. Chase Sapphire"
            />
          </Field>

          <Field>
            <FieldLabel>Account type</FieldLabel>
            <Select
              items={ACCOUNT_TYPE_OPTIONS}
              value={type}
              disabled={Boolean(account?.type_locked)}
              onValueChange={(v) => {
                if (v) setType(v as AccountType)
              }}
            >
              <SelectTrigger className="w-full" aria-label="Account type">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {ACCOUNT_TYPE_OPTIONS.map((opt) => (
                  <SelectItem key={opt.value} value={opt.value}>
                    {opt.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {account?.type_locked && (
              <FieldDescription>
                Account type is locked because transactions or reconciliations
                already exist.
              </FieldDescription>
            )}
          </Field>

          <div className="grid gap-4 sm:grid-cols-2">
            <Field>
              <FieldLabel>
                Initial balance ({account?.currency || currency})
              </FieldLabel>
              <Input
                type="text"
                inputMode="decimal"
                required={!account}
                value={initialBalance}
                onChange={(e) => setInitialBalance(e.target.value)}
                placeholder="0.00"
              />
            </Field>

            <Field>
              <FieldLabel>As of date</FieldLabel>
              <DatePicker
                value={initialBalanceAsOf}
                onChange={setInitialBalanceAsOf}
                ariaLabel="As of date"
              />
            </Field>
          </div>

          {type === "credit_card" && (
            <Field>
              <FieldLabel>Statement day (optional)</FieldLabel>
              <Input
                type="number"
                inputMode="numeric"
                min={1}
                max={31}
                step={1}
                value={statementDay}
                onChange={(e) => setStatementDay(e.target.value)}
                placeholder="e.g. 22"
              />
              <FieldDescription>
                Day of the month your credit card statement closes (1–31).
                Shorter months clamp to the last day of the month.
              </FieldDescription>
            </Field>
          )}

          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <span className="text-xs font-medium text-muted-foreground">
                Identifiers (optional)
              </span>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() =>
                  setIdentifiers((prev) => [
                    ...prev,
                    {
                      key: crypto.randomUUID(),
                      kind: type === "digital_wallet" ? "alias" : "card_last4",
                      value: "",
                    },
                  ])
                }
              >
                Add identifier
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              Add card last-4 digits, bank account numbers, or wallet aliases
              that appear on emails.
            </p>
            {identifiers.map((row) => (
              <div key={row.key} className="flex items-center gap-2">
                <Input
                  type="text"
                  aria-label="Identifier value"
                  value={row.value}
                  onChange={(e) =>
                    setIdentifiers((prev) =>
                      prev.map((r) =>
                        r.key === row.key ? { ...r, value: e.target.value } : r
                      )
                    )
                  }
                  placeholder={IDENTIFIER_PLACEHOLDERS[row.kind]}
                  className="flex-1"
                />
                <Select
                  items={IDENTIFIER_KIND_OPTIONS}
                  value={row.kind}
                  onValueChange={(v) => {
                    if (v) {
                      setIdentifiers((prev) =>
                        prev.map((r) =>
                          r.key === row.key
                            ? { ...r, kind: v as AccountIdentifierKind }
                            : r
                        )
                      )
                    }
                  }}
                >
                  <SelectTrigger className="w-36" aria-label="Identifier type">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {IDENTIFIER_KIND_OPTIONS.map((opt) => (
                      <SelectItem key={opt.value} value={opt.value}>
                        {opt.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() =>
                    setIdentifiers((prev) =>
                      prev.filter((r) => r.key !== row.key)
                    )
                  }
                >
                  Remove
                </Button>
              </div>
            ))}
          </div>

          {formError && (
            <Alert variant="destructive">
              <AlertDescription>{formError}</AlertDescription>
            </Alert>
          )}

          <DialogFooter>
            <DialogClose render={<Button type="button" variant="outline" />}>
              Cancel
            </DialogClose>
            <Button type="submit" disabled={saveMutation.isPending}>
              {saveMutation.isPending
                ? "Saving..."
                : account
                  ? "Save changes"
                  : "Create account"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
