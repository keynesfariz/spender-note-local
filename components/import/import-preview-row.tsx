import { memo } from "react"
import { Badge } from "@/components/ui/badge"
import { Checkbox } from "@/components/ui/checkbox"
import { formatMinorCurrency } from "@/lib/format"
import { type ParsedManualImportRow } from "@/lib/manual-import"

export const ImportPreviewRow = memo(function ImportPreviewRow({
  row,
  isChecked,
  disabled,
  onToggle,
}: {
  row: ParsedManualImportRow
  isChecked: boolean
  disabled: boolean
  onToggle: (index: number, checked: boolean) => void
}) {
  return (
    <tr
      className={
        row.valid ? undefined : "bg-destructive/5 text-muted-foreground"
      }
    >
      <td className="px-3 py-2.5 align-top">
        <Checkbox
          aria-label={`Select row ${row.index + 1}`}
          checked={isChecked}
          disabled={disabled || !row.valid}
          onCheckedChange={(checked) => onToggle(row.index, Boolean(checked))}
        />
      </td>
      <td className="px-3 py-2.5 align-top text-xs whitespace-nowrap">
        {row.dateDisplay}
      </td>
      <td className="px-3 py-2.5 align-top">
        <div className="font-medium text-foreground">{row.merchant}</div>
        {row.note && (
          <div className="text-xs text-muted-foreground">{row.note}</div>
        )}
        {row.error && (
          <div className="mt-1 text-xs font-medium text-destructive">
            {row.error}
          </div>
        )}
      </td>
      <td className="px-3 py-2.5 align-top">
        <Badge variant={row.kind === "income" ? "secondary" : "outline"}>
          {row.kind}
        </Badge>
      </td>
      <td className="px-3 py-2.5 align-top">
        <div className="flex flex-wrap items-center gap-1">
          <Badge variant="outline">{row.resolvedCategory}</Badge>
          {!row.categoryMatched && row.rawCategory && (
            <span className="text-xs text-muted-foreground">
              (was &ldquo;{row.rawCategory}&rdquo;)
            </span>
          )}
          {row.matchedTags.map((tag) => (
            <Badge key={tag} variant="secondary">
              {tag}
            </Badge>
          ))}
          {row.droppedTags.length > 0 && (
            <span className="text-xs text-muted-foreground">
              dropped: {row.droppedTags.join(", ")}
            </span>
          )}
        </div>
      </td>
      <td className="px-3 py-2.5 text-right align-top font-medium whitespace-nowrap tabular-nums">
        {row.valid
          ? formatMinorCurrency(row.amountMinor, row.currency)
          : `${row.amountDisplay} ${row.currency}`}
      </td>
    </tr>
  )
})
