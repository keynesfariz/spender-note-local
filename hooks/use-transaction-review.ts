import { useQueryClient } from "@tanstack/react-query"
import { useCallback, useLayoutEffect, useRef, useState } from "react"

import { transactionsQuery } from "@/lib/api/transactions"
import type { PaginatedTransactionsResponse } from "@/lib/api/types.generated"
import {
  getReviewPosition,
  resolveReviewNeighbor,
  type ReviewDirection,
  type ReviewPosition,
} from "@/lib/transaction-review"

type LedgerParams = NonNullable<Parameters<typeof transactionsQuery>[0]>

type Selection = {
  id: string
  scope: string
  position: ReviewPosition | null
  dayBoundary: string | undefined
}

type ReviewAction =
  | { direction: ReviewDirection; write?: never }
  | { direction?: "next"; write: () => Promise<unknown> }

export function useTransactionReview({
  params,
  page,
  scope,
  onPageChange,
}: {
  params: LedgerParams
  page: PaginatedTransactionsResponse | undefined
  scope: string
  onPageChange: (page: number) => void
}) {
  const queryClient = useQueryClient()
  const [selection, setSelection] = useState<Selection | null>(null)
  const [pending, setPending] = useState<Selection | null>(null)
  const [error, setError] = useState<{
    selection: Selection
    message: string
  } | null>(null)
  const [savedPosition, setSavedPosition] = useState<{
    selection: Selection
    position: ReviewPosition
  } | null>(null)
  const openSelection = useRef<Selection | null>(null)
  const pendingSelection = useRef<Selection | null>(null)
  const activeScope = useRef(scope)

  useLayoutEffect(() => {
    activeScope.current = scope
    return () => {
      activeScope.current = ""
    }
  }, [scope])

  const select = useCallback((next: Selection | null) => {
    openSelection.current = next
    setSelection(next)
    setError(null)
    setSavedPosition(null)
  }, [])

  const open = useCallback(
    (id: string) => {
      select({
        id,
        scope,
        position: page ? getReviewPosition(page, id) : null,
        dayBoundary: page?.ledger?.nextDayUtc,
      })
    },
    [page, scope, select]
  )

  const heldPosition =
    savedPosition?.selection === selection ? savedPosition.position : null
  const livePosition =
    page && selection ? getReviewPosition(page, selection.id) : null
  const stillInPlace =
    livePosition &&
    heldPosition &&
    livePosition.page === heldPosition.page &&
    livePosition.index === heldPosition.index &&
    livePosition.row.occurred_at_utc === heldPosition.row.occurred_at_utc
  const position =
    selection?.scope === scope
      ? ((stillInPlace ? livePosition : heldPosition) ??
        livePosition ??
        selection.position)
      : null

  async function run(action: ReviewAction) {
    if (
      !selection ||
      openSelection.current !== selection ||
      pendingSelection.current === selection
    ) {
      return
    }
    const current = selection
    const isCurrent = () =>
      openSelection.current === current && activeScope.current === scope
    pendingSelection.current = current
    setPending(current)
    setError(null)

    const loadPage = async (number: number) => {
      const result = await queryClient.fetchQuery({
        ...transactionsQuery({ ...params, page: number }),
        staleTime: 0,
      })
      if (
        params.ledgerRange === "dates" &&
        !params.startDate &&
        current.dayBoundary &&
        result.ledger?.nextDayUtc !== current.dayBoundary
      ) {
        throw new Error(
          "The date range changed. Reopen the transaction from the list."
        )
      }
      return result
    }

    function move(destination: ReviewPosition) {
      if (!isCurrent()) return
      if (page?.page !== destination.page) onPageChange(destination.page)
      select({
        id: destination.row.id,
        scope,
        position: destination,
        dayBoundary: current.dayBoundary,
      })
    }

    try {
      if (!action.write) {
        if (!position) return
        const target = position[action.direction]
        if (!target) return
        const destination = await resolveReviewNeighbor(
          position,
          action.direction,
          !heldPosition && livePosition && page && target.page === page.page
            ? async () => page
            : loadPage,
          Boolean(heldPosition)
        )
        if (!destination)
          throw new Error(
            "The neighboring transaction is no longer in this list."
          )
        move(destination)
        return
      }
      let captured = position
      if (action.direction && !captured?.next?.id) {
        const next =
          captured &&
          (await resolveReviewNeighbor(
            captured,
            "next",
            loadPage,
            Boolean(heldPosition)
          ))
        if (!captured || !next) {
          throw new Error("The next transaction could not be loaded.")
        }
        captured = {
          ...captured,
          next: { page: next.page, index: next.index, id: next.row.id },
        }
      }
      if (!isCurrent()) return
      try {
        await action.write()
      } catch {
        // The form owns validation and mutation errors.
        return
      }
      if (!isCurrent() || !captured) return
      setSavedPosition({ selection: current, position: captured })
      if (action.direction) {
        try {
          const destination = await resolveReviewNeighbor(
            captured,
            action.direction,
            loadPage,
            true
          )
          if (!destination)
            throw new Error("The next transaction is no longer in this list.")
          move(destination)
        } catch {
          throw new Error(
            "Transaction updated. Could not open the next transaction. Choose Next to try again."
          )
        }
      }
    } catch (cause) {
      if (isCurrent())
        setError({
          selection: current,
          message:
            cause instanceof Error
              ? cause.message
              : "Could not move to another transaction.",
        })
    } finally {
      if (pendingSelection.current === current) {
        pendingSelection.current = null
        setPending(null)
      }
    }
  }

  return {
    transactionId: selection?.id ?? null,
    previous: Boolean(position?.previous),
    next: Boolean(position?.next),
    pending: pending !== null && pending === selection,
    error: error?.selection === selection ? error?.message : undefined,
    open,
    close: () => {
      if (openSelection.current === selection) select(null)
    },
    navigate: (direction: ReviewDirection) => run({ direction }),
    write: (write: () => Promise<unknown>, advance: boolean) =>
      run({ write, direction: advance ? "next" : undefined }),
  }
}
