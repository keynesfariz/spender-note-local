import { Checkbox } from "@/components/ui/checkbox"
import { ImportPreviewRow } from "./import-preview-row"
import { type ParsedManualImportRow } from "@/lib/manual-import"

export function ImportPreviewTable({
  allValidSelected,
  importPending,
  validCount,
  toggleAllValid,
  rows,
  deselectedIndices,
  toggleRowSelection,
}: {
  allValidSelected: boolean
  importPending: boolean
  validCount: number
  toggleAllValid: (checked: boolean) => void
  rows: ParsedManualImportRow[]
  deselectedIndices: ReadonlySet<number>
  toggleRowSelection: (index: number, checked: boolean) => void
}) {
  return (
    <div className="overflow-x-auto rounded-md border">
      <table className="w-full text-sm">
        <thead className="border-b bg-muted/40 text-left text-xs font-medium text-muted-foreground">
          <tr>
            <th className="w-10 px-3 py-2.5">
              <Checkbox
                aria-label="Select all valid rows"
                checked={allValidSelected}
                disabled={importPending || validCount === 0}
                onCheckedChange={(checked) => toggleAllValid(Boolean(checked))}
              />
            </th>
            <th className="px-3 py-2.5">Date</th>
            <th className="px-3 py-2.5">Merchant</th>
            <th className="px-3 py-2.5">Kind</th>
            <th className="px-3 py-2.5">Category &amp; Tags</th>
            <th className="px-3 py-2.5 text-right">Amount</th>
          </tr>
        </thead>
        <tbody className="divide-y">
          {rows.map((row) => (
            <ImportPreviewRow
              key={row.index}
              row={row}
              disabled={importPending}
              isChecked={row.valid && !deselectedIndices.has(row.index)}
              onToggle={toggleRowSelection}
            />
          ))}
        </tbody>
      </table>
    </div>
  )
}
