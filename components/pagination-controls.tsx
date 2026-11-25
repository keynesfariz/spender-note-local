import { Button } from "@/components/ui/button"

export function PaginationControls({
  page,
  totalPages,
  isLoading,
  onPageChange,
}: {
  page: number
  totalPages: number
  isLoading: boolean
  onPageChange: (page: number) => void
}) {
  return (
    <>
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={page <= 1 || isLoading}
        onClick={() => onPageChange(page - 1)}
      >
        Previous page
      </Button>
      <span className="tabular-nums">
        Page {page} of {totalPages}
      </span>
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={page >= totalPages || isLoading}
        onClick={() => onPageChange(page + 1)}
      >
        Next page
      </Button>
    </>
  )
}
