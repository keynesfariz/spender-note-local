import { CheckLine, MailIcon } from "lucide-react"
import { useId, useMemo, type ComponentProps, type ReactNode } from "react"

import { DatePicker } from "@/components/date-picker"
import { DynamicIcon } from "@/components/dynamic-icon"
import { TagMultiSelect } from "@/components/tag-multi-select"
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
import {
  DEFAULT_CATEGORY,
  buildCategoryIconMap,
  buildCategorySelectOptions,
} from "@/lib/categories"

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
  const categoryIconMap = useMemo(
    () => buildCategoryIconMap(categories),
    [categories]
  )
  const categoryOptions = useMemo(
    () => buildCategorySelectOptions(categories, category),
    [categories, category]
  )
  const accountItems = useMemo(() => {
    const items = [
      { value: UNASSIGNED_ACCOUNT, label: "Unassigned" },
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
  }, [accounts, accountId, missingAccountLabel])
  const descriptionField = (
    <Field>
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
  )

  return (
    <div className="grid gap-4 sm:grid-cols-2" inert={inert}>
      {descriptionField}
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
        <FieldLabel>Category</FieldLabel>
        <Select
          items={categoryOptions}
          value={category}
          onValueChange={(value) => onCategoryChange(value ?? DEFAULT_CATEGORY)}
        >
          <SelectTrigger className="w-full" aria-label="Category">
            <DynamicIcon
              name={categoryIconMap.get(category)}
              className="size-4 text-muted-foreground"
            />
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {categoryOptions.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                <span className="flex items-center gap-2">
                  <DynamicIcon
                    name={categoryIconMap.get(option.value)}
                    className="size-4 text-muted-foreground"
                  />
                  <span>{option.label}</span>
                </span>
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Field>

      <Field className="sm:col-span-2">
        <FieldLabel>Tags</FieldLabel>
        {tagPicker && <TagMultiSelect key={tagPickerKey} {...tagPicker} />}
      </Field>

      <Field>
        <FieldLabel>Account</FieldLabel>
        <Select
          items={accountItems}
          value={accountId}
          onValueChange={(value) =>
            onAccountChange(value ?? UNASSIGNED_ACCOUNT)
          }
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
        {paymentMethod && (
          <FieldDescription>
            Extracted payment method: {paymentMethod}
          </FieldDescription>
        )}
      </Field>

      <Field className="min-w-0">
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
  sourceCount,
  sources,
  ...tabs
}: {
  details: ReactNode
  note: ComponentProps<typeof TransactionNoteField>
  noteActions?: ReactNode
  sourceCount: number
  sources?: ReactNode
} & Pick<ComponentProps<typeof Tabs>, "value" | "onValueChange">) {
  return (
    <Tabs className="gap-4" defaultValue="details" {...tabs}>
      <TabsList
        className="max-w-full overflow-x-auto"
        aria-label="Transaction detail sections"
      >
        <TabsTrigger value="details">Details</TabsTrigger>
        <TabsTrigger value="note">
          <span>Note</span>
          {note.value.trim() ? <CheckLine /> : null}
        </TabsTrigger>
        <TabsTrigger value="sources">
          <MailIcon className="size-3.5" /> Source emails ({sourceCount})
        </TabsTrigger>
      </TabsList>
      <TabsContent value="details" keepMounted>
        {details}
      </TabsContent>
      <TabsContent value="note" className="space-y-6">
        <TransactionNoteField {...note} />
        {noteActions}
      </TabsContent>
      <TabsContent value="sources" className="space-y-3">
        {sources ?? (
          <p className="text-sm text-muted-foreground">
            No source emails attached (manual transaction).
          </p>
        )}
      </TabsContent>
    </Tabs>
  )
}
