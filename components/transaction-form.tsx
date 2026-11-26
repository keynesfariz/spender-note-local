import { MailIcon, StickyNote, StickyNoteCheck } from "lucide-react"
import { useId, useMemo, type ComponentProps, type ReactNode } from "react"

import { CategoryField } from "@/components/category-field"
import { DatePicker } from "@/components/date-picker"
import { TagMultiSelect } from "@/components/tag-multi-select"
import { Badge } from "@/components/ui/badge"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import type { AccountOption, CategorySummary } from "@/lib/api/types.generated"

export const UNASSIGNED_ACCOUNT = "__unassigned__"

type TransactionFormFieldsProps = {
  description: {
    required?: boolean
    value: string
    onChange: (value: string) => void
    placeholder: string
  }
  currency: string
  amount: string
  onAmountChange: (value: string) => void
  amountInputProps?: Pick<
    ComponentProps<typeof Input>,
    "className" | "onBlur" | "placeholder"
  >
  categories?: CategorySummary[]
  category: string
  onCategoryChange: (value: string) => void
  tagPicker?: ComponentProps<typeof TagMultiSelect>
  tagPickerKey?: string
  accounts: AccountOption[]
  accountId: string
  onAccountChange: (value: string) => void
  receivingAccount?: boolean
  missingAccountLabel?: string | null
  paymentMethod?: string | null
  date: string
  onDateChange: (value: string) => void
  dateLabel: string
  dateAriaLabel?: string
  time?: ComponentProps<typeof DatePicker>["time"]
  inert?: boolean
}

export function TransactionFormFields({
  description,
  currency,
  amount,
  onAmountChange,
  amountInputProps,
  categories,
  category,
  onCategoryChange,
  tagPicker,
  tagPickerKey,
  accounts,
  accountId,
  onAccountChange,
  receivingAccount,
  missingAccountLabel,
  paymentMethod,
  date,
  onDateChange,
  dateLabel,
  dateAriaLabel = dateLabel,
  time,
  inert,
}: TransactionFormFieldsProps) {
  const amountId = useId()
  const descriptionId = useId()
  const dateId = useId()
  const accountItems = useMemo(() => {
    const items = [
      {
        value: UNASSIGNED_ACCOUNT,
        label: receivingAccount ? "Choose an account" : "Unassigned",
      },
      ...accounts.map((account) => ({
        value: account.id,
        label: account.name,
      })),
    ]
    if (!items.some((item) => item.value === accountId)) {
      items.push({
        value: accountId,
        label: missingAccountLabel || "Unknown account",
      })
    }
    return items
  }, [accounts, accountId, missingAccountLabel, receivingAccount])

  return (
    <div className="grid gap-4 sm:grid-cols-2" inert={inert}>
      <Field className="sm:col-span-2">
        <FieldLabel htmlFor={descriptionId}>Remark</FieldLabel>
        <Input
          id={descriptionId}
          type="text"
          required={description.required}
          value={description.value}
          placeholder={description.placeholder}
          onChange={(event) => description.onChange(event.target.value)}
        />
      </Field>
      <Field>
        <FieldLabel htmlFor={amountId}>Amount ({currency})</FieldLabel>
        <Input
          {...amountInputProps}
          id={amountId}
          type="text"
          inputMode="decimal"
          required
          value={amount}
          onChange={(event) => onAmountChange(event.target.value)}
        />
      </Field>
      <Field>
        <FieldLabel>
          {receivingAccount ? "Receiving account" : "Account"}
        </FieldLabel>
        <Select
          items={accountItems}
          value={accountId}
          onValueChange={(value) =>
            onAccountChange(value ?? UNASSIGNED_ACCOUNT)
          }
        >
          <SelectTrigger
            className="w-full"
            aria-label={receivingAccount ? "Receiving account" : "Account"}
          >
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
        {paymentMethod && (
          <FieldDescription>
            Extracted payment method: {paymentMethod}
          </FieldDescription>
        )}
      </Field>

      <Field>
        <FieldLabel htmlFor={dateId}>{dateLabel}</FieldLabel>
        <DatePicker
          id={dateId}
          value={date}
          onChange={onDateChange}
          ariaLabel={dateAriaLabel}
          time={time}
          disabled={inert}
        />
      </Field>

      <CategoryField
        categories={categories}
        value={category}
        onChange={onCategoryChange}
      />

      <Field className="sm:col-span-2">
        <FieldLabel>Tags</FieldLabel>
        {tagPicker && <TagMultiSelect key={tagPickerKey} {...tagPicker} />}
      </Field>
    </div>
  )
}

function TransactionNoteField({
  value,
  onChange,
  disabled,
}: {
  value: string
  onChange: (value: string) => void
  disabled?: boolean
}) {
  const id = useId()

  return (
    <Field>
      <FieldLabel htmlFor={id}>Note</FieldLabel>
      <Textarea
        id={id}
        rows={8}
        className="min-h-40"
        disabled={disabled}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
    </Field>
  )
}

export function TransactionForm({
  details,
  note,
  noteActions,
  sourceTab,
  ...tabs
}: {
  details: ReactNode
  note: ComponentProps<typeof TransactionNoteField>
  noteActions?: ReactNode
  sourceTab?: { count: number; content?: ReactNode }
} & Pick<ComponentProps<typeof Tabs>, "value" | "onValueChange">) {
  return (
    <Tabs className="gap-4" defaultValue="details" {...tabs}>
      <TabsList
        variant="line"
        className="max-w-full overflow-x-auto"
        aria-label="Transaction detail sections"
      >
        <TabsTrigger value="details">Details</TabsTrigger>
        <TabsTrigger value="note">
          {note.value.trim() ? <StickyNoteCheck /> : <StickyNote />}
          <span>Note</span>
        </TabsTrigger>
        {sourceTab ? (
          <TabsTrigger value="sources">
            <MailIcon className="size-3.5" />
            <span>Source emails</span>
            <Badge variant="secondary" className="tabular-nums">
              {sourceTab.count}
            </Badge>
          </TabsTrigger>
        ) : null}
      </TabsList>
      <TabsContent value="details" keepMounted>
        {details}
      </TabsContent>
      <TabsContent value="note" className="space-y-6">
        <TransactionNoteField {...note} />
        {noteActions}
      </TabsContent>
      {sourceTab ? (
        <TabsContent value="sources" className="space-y-3">
          {sourceTab.content ?? (
            <p className="text-sm text-muted-foreground">
              No source emails attached (manual transaction).
            </p>
          )}
        </TabsContent>
      ) : null}
    </Tabs>
  )
}
