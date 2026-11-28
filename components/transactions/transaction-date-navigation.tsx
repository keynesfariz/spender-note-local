import { ChevronLeftIcon, ChevronRightIcon } from "lucide-react"
import { DateRangePicker } from "@/components/date-picker"
import { Button } from "@/components/ui/button"

export function TransactionDateNavigation({
  isAllDates,
  handleShiftRange,
  activeRange,
  updateRange,
  handleToggleAllDates,
}: {
  isAllDates: boolean
  handleShiftRange: (direction: -1 | 1) => void
  activeRange: { from: string; to: string }
  updateRange: (range: { from: string; to: string }) => void
  handleToggleAllDates: () => void
}) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={isAllDates}
        onClick={() => handleShiftRange(-1)}
      >
        <ChevronLeftIcon className="mr-1 size-4" />
        Previous
      </Button>

      <DateRangePicker
        from={activeRange.from}
        to={activeRange.to}
        onChange={updateRange}
      />

      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={isAllDates}
        onClick={() => handleShiftRange(1)}
      >
        Next
        <ChevronRightIcon className="ml-1 size-4" />
      </Button>

      <Button
        type="button"
        variant={isAllDates ? "default" : "outline"}
        size="sm"
        onClick={handleToggleAllDates}
      >
        All dates
      </Button>
    </div>
  )
}
