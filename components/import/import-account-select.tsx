import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

type AccountSelectItem = {
  value: string
  label: string
}

export function AccountSelect({
  id,
  className,
  items,
  value,
  onValueChange,
  disabled,
}: {
  id: string
  className?: string
  items: AccountSelectItem[]
  value: string
  onValueChange: (value: string) => void
  disabled?: boolean
}) {
  return (
    <Select
      items={items}
      disabled={disabled}
      value={value}
      onValueChange={(val) => onValueChange(val ?? "")}
    >
      <SelectTrigger id={id} className={className}>
        <SelectValue placeholder="Select account" />
      </SelectTrigger>
      <SelectContent>
        {items.map((item) => (
          <SelectItem key={item.value} value={item.value}>
            {item.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
