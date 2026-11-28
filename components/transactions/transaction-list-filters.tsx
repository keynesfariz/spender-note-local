import { RotateCcwIcon } from "lucide-react"
import { DynamicIcon } from "@/components/dynamic-icon"
import { DebouncedSearchInput } from "@/components/debounced-search-input"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
const TRANSACTION_SOURCE_OPTIONS = [
  { value: "all", label: "All sources" },
  { value: "gmail", label: "Gmail" },
  { value: "manual", label: "Manual" },
] as const

export type TransactionSourceFilter =
  (typeof TRANSACTION_SOURCE_OPTIONS)[number]["value"]

export function TransactionListFilters({
  search,
  searchResetKey,
  onSearchChange,
  category,
  onCategoryChange,
  tag,
  onTagChange,
  accountId,
  onAccountIdChange,
  source,
  onSourceChange,
  unreviewedOnly,
  onUnreviewedOnlyChange,
  onResetFilters,
  categoryFilterItems,
  tagFilterItems,
  accountFilterItems,
  categoryIconMap,
}: {
  search: string
  searchResetKey: number
  onSearchChange: (value: string) => void
  category: string
  onCategoryChange: (value: string) => void
  tag: string
  onTagChange: (value: string) => void
  accountId: string
  onAccountIdChange: (value: string) => void
  source: TransactionSourceFilter
  onSourceChange: (value: TransactionSourceFilter) => void
  unreviewedOnly: boolean
  onUnreviewedOnlyChange: (value: boolean) => void
  onResetFilters: () => void
  categoryFilterItems: { value: string; label: string }[]
  tagFilterItems: { value: string; label: string }[]
  accountFilterItems: { value: string; label: string }[]
  categoryIconMap: ReadonlyMap<string, string>
}) {
  return (
    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-[repeat(5,minmax(0,1fr))_auto]">
      <DebouncedSearchInput
        key={searchResetKey}
        label="Search transactions"
        placeholder="Search remarks, merchants, or notes..."
        value={search}
        onSearch={onSearchChange}
      />

      <Select
        items={categoryFilterItems}
        value={category}
        onValueChange={(value) => onCategoryChange(value ?? "")}
      >
        <SelectTrigger className="w-full" aria-label="Filter by category">
          <SelectValue placeholder="All categories" />
        </SelectTrigger>
        <SelectContent>
          {categoryFilterItems.map((item) => (
            <SelectItem key={item.value} value={item.value}>
              {item.value === "" ? (
                item.label
              ) : (
                <span className="flex items-center gap-2">
                  <DynamicIcon
                    name={categoryIconMap.get(item.value)}
                    className="size-3.5 text-muted-foreground"
                  />
                  <span>{item.label}</span>
                </span>
              )}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <PlainTransactionFilter
        items={tagFilterItems}
        value={tag}
        onValueChange={(value) => onTagChange(value ?? "")}
        label="Filter by tag"
        placeholder="All tags"
      />

      <PlainTransactionFilter
        items={accountFilterItems}
        value={accountId}
        onValueChange={(value) => onAccountIdChange(value ?? "all")}
        label="Filter by account"
        placeholder="All accounts"
      />

      <PlainTransactionFilter
        items={TRANSACTION_SOURCE_OPTIONS}
        value={source}
        onValueChange={(value) => onSourceChange(value ?? "all")}
        label="Filter by source"
        placeholder="All sources"
      />

      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 sm:col-span-2 lg:col-span-1">
        <div className="flex items-center gap-2">
          <Checkbox
            id="unreviewed-filter"
            checked={unreviewedOnly}
            onCheckedChange={(checked) =>
              onUnreviewedOnlyChange(Boolean(checked))
            }
          />
          <Label htmlFor="unreviewed-filter">Needs review</Label>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={onResetFilters}
          disabled={
            !search &&
            !category &&
            !tag &&
            accountId === "all" &&
            source === "all" &&
            !unreviewedOnly
          }
        >
          <RotateCcwIcon aria-hidden="true" />
          Reset filters
        </Button>
      </div>
    </div>
  )
}

function PlainTransactionFilter<Value extends string>({
  items,
  value,
  onValueChange,
  label,
  placeholder,
}: {
  items: readonly { value: Value; label: string }[]
  value: Value
  onValueChange: (value: Value | null) => void
  label: string
  placeholder: string
}) {
  return (
    <Select items={items} value={value} onValueChange={onValueChange}>
      <SelectTrigger className="w-full" aria-label={label}>
        <SelectValue placeholder={placeholder} />
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
