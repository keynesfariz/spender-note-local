import { useQuery } from "@tanstack/react-query"
import { ChevronDownIcon } from "lucide-react"
import { useState } from "react"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { transactionSourceBodyQuery } from "@/lib/api/transaction-inspector"
import { type TransactionInspectorSource } from "@/lib/api/types.generated"
import { formatUtcInTimezone } from "@/lib/format"

function SourceEmailBody({
  transactionId,
  emailId,
}: {
  transactionId: string
  emailId: string
}) {
  const body = useQuery(transactionSourceBodyQuery(transactionId, emailId))

  if (body.isPending) {
    return (
      <p role="status" className="text-xs text-muted-foreground">
        Loading email…
      </p>
    )
  }

  if (body.isError) {
    return (
      <div className="space-y-2">
        <p role="alert" className="text-xs text-destructive">
          {body.error.message}
        </p>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => void body.refetch()}
        >
          Retry
        </Button>
      </div>
    )
  }

  return (
    <pre className="max-h-64 overflow-auto rounded-md border bg-muted p-3 font-mono text-xs wrap-break-word whitespace-pre-wrap">
      {body.data.cleaned_text}
    </pre>
  )
}

export function SourceEmail({
  source,
  transactionId,
  timezone,
  onSplit,
  splitDisabled,
}: {
  source: TransactionInspectorSource
  transactionId: string
  timezone: string | null | undefined
  onSplit?: () => void
  splitDisabled: boolean
}) {
  const [open, setOpen] = useState(false)

  return (
    <Card>
      <CardContent className="space-y-2 text-sm">
        <details
          className="group"
          onToggle={(event) => setOpen(event.currentTarget.open)}
        >
          <summary className="flex cursor-pointer list-none items-start gap-3 rounded outline-none focus-visible:ring-2 focus-visible:ring-ring [&::-webkit-details-marker]:hidden">
            <div className="min-w-0 flex-1 wrap-break-word">
              <p className="font-medium">
                {source.subject || "Untitled email"}
              </p>
              <p className="mt-1 text-xs text-muted-foreground">
                {source.from_address} ·{" "}
                {formatUtcInTimezone(source.received_at_utc, timezone)}
              </p>
            </div>
            <span className="mt-0.5 size-4 shrink-0 text-muted-foreground transition-transform group-open:rotate-180">
              <ChevronDownIcon className="size-4" aria-hidden />
            </span>
          </summary>
          <div className="mt-3 space-y-3">
            <p className="text-xs text-muted-foreground">
              Role: {source.role} · State: {source.match_state}
            </p>
            {open ? (
              <SourceEmailBody
                transactionId={transactionId}
                emailId={source.email_id}
              />
            ) : null}
            {onSplit ? (
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={splitDisabled}
                onClick={onSplit}
              >
                Split email
              </Button>
            ) : null}
          </div>
        </details>
      </CardContent>
    </Card>
  )
}
