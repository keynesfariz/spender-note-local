import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { PaginationControls } from "@/components/pagination-controls"
import {
  TRANSACTIONS_PAGE_SIZE,
  transactionsQuery,
} from "@/lib/api/transactions"
import { skippedEmailBodyQuery } from "@/lib/api/triage"
import { formatMinorCurrency, formatUtcInTimezone } from "@/lib/format"

export function SkippedPaymentEvidence({ emailId }: { emailId: string }) {
  const [showEvidence, setShowEvidence] = useState(false)
  const evidence = useQuery(skippedEmailBodyQuery(emailId, showEvidence))

  return (
    <details onToggle={(event) => setShowEvidence(event.currentTarget.open)}>
      <summary className="cursor-pointer text-sm font-medium">
        Read source email
      </summary>
      {evidence.isPending && showEvidence && (
        <p role="status" className="mt-2 text-sm">
          Loading email…
        </p>
      )}
      {evidence.isError && (
        <p role="alert" className="mt-2 text-sm text-destructive">
          {evidence.error.message}
        </p>
      )}
      {evidence.data && (
        <div className="mt-2 max-h-56 overflow-auto rounded-md border bg-muted/30 p-3 text-sm wrap-break-word whitespace-pre-wrap">
          {evidence.data.cleanedText || "No email body available."}
        </div>
      )}
    </details>
  )
}

export function TransferMatchPicker({
  active,
  cardPayment,
  matchId,
  name,
  onMatchChange,
  timezone,
}: {
  active: boolean
  cardPayment?: boolean
  matchId: string
  name: string
  onMatchChange: (id: string) => void
  timezone?: string | null
}) {
  const [matchPage, setMatchPage] = useState(1)
  const matches = useQuery({
    ...transactionsQuery({
      kind: "transfer",
      ledgerRange: "all",
      destinationAccountType: cardPayment ? "credit_card" : undefined,
      page: matchPage,
    }),
    enabled: active,
  })

  if (!active) return null

  const candidates = matches.data?.items ?? []

  return (
    <div className="space-y-3">
      {matches.isPending && (
        <p role="status" className="text-sm">
          Loading transfers…
        </p>
      )}
      {matches.isError && (
        <p role="alert" className="text-sm text-destructive">
          {matches.error.message}
        </p>
      )}
      {!matches.isPending && candidates.length === 0 && (
        <p className="text-sm text-muted-foreground">
          No eligible transfers on this page.
        </p>
      )}
      {candidates.map((item) => (
        <label
          key={item.id}
          className="flex cursor-pointer items-start gap-3 rounded-md border p-3 text-sm"
        >
          <input
            type="radio"
            name={name}
            value={item.id}
            checked={matchId === item.id}
            onChange={() => onMatchChange(item.id)}
            className="mt-1"
          />
          <span>
            <span className="block font-medium">
              {item.account_name} → {item.destination_account_name}
            </span>
            {formatMinorCurrency(item.amount_minor, item.currency)}
            {item.destination_amount_minor != null &&
              item.destination_currency && (
                <>
                  {" "}
                  →{" "}
                  {formatMinorCurrency(
                    item.destination_amount_minor,
                    item.destination_currency
                  )}
                </>
              )}
            <span className="block text-xs text-muted-foreground">
              {formatUtcInTimezone(
                item.source_posted_at_utc ?? item.occurred_at_utc,
                timezone
              )}
              {" → "}
              {formatUtcInTimezone(
                item.destination_posted_at_utc ?? item.occurred_at_utc,
                timezone
              )}
            </span>
          </span>
        </label>
      ))}
      <div className="flex flex-wrap items-center gap-2">
        <PaginationControls
          page={matchPage}
          totalPages={Math.max(
            1,
            Math.ceil(
              (matches.data?.totalCount ?? 0) /
                (matches.data?.pageSize || TRANSACTIONS_PAGE_SIZE)
            )
          )}
          isLoading={matches.isFetching}
          onPageChange={setMatchPage}
        />
      </div>
    </div>
  )
}
