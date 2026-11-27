import { useId, useMemo } from "react"

import { DynamicIcon } from "@/components/dynamic-icon"
import { Field, FieldLabel } from "@/components/ui/field"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import type { CategorySummary } from "@/lib/api/types.generated"
import {
  DEFAULT_CATEGORY,
  buildCategoryIconMap,
  buildCategorySelectOptions,
} from "@/lib/categories"

export function CategoryField({
  categories,
  value,
  onChange,
  valid,
}: {
  categories?: readonly CategorySummary[]
  value: string
  onChange: (value: string) => void
  valid?: boolean
}) {
  const id = useId()
  const icons = useMemo(() => buildCategoryIconMap(categories), [categories])
  const options = useMemo(
    () => buildCategorySelectOptions(categories, value),
    [categories, value]
  )

  return (
    <Field data-valid={valid || undefined}>
      <FieldLabel htmlFor={id}>Category</FieldLabel>
      <Select
        items={options}
        value={value}
        onValueChange={(nextValue) => onChange(nextValue ?? DEFAULT_CATEGORY)}
      >
        <SelectTrigger id={id} className="w-full">
          <DynamicIcon
            name={icons.get(value)}
            className="size-4 text-muted-foreground"
          />
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              <span className="flex items-center gap-2">
                <DynamicIcon
                  name={icons.get(option.value)}
                  className="size-4 text-muted-foreground"
                />
                <span>{option.label}</span>
              </span>
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </Field>
  )
}
