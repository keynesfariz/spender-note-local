import { useQueryClient } from "@tanstack/react-query"
import { useCallback, useLayoutEffect, useMemo, useRef, useState } from "react"

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

type ReviewSession = {
  selection: Selection
  pending: boolean
  error?: string
  savedPosition?: ReviewPosition
  resolvedPosition?: ReviewPosition
}

type ReviewAction =
  | { direction: ReviewDirection; write?: never }
  | { direction?: "next"; write: () => Promise<unknown> }

export function useTransactionReview({
  params,
  page,
  scope,
  isRelativeRange,
  onPageChange,
}: {
  params: LedgerParams
  page: PaginatedTransactionsResponse | undefined
  scope: string
  isRelativeRange: boolean
  onPageChange: (page: number) => void
}) {
  const queryClient = useQueryClient()
  const [session, setSession] = useState<ReviewSession | null>(null)
  const selection = session?.selection ?? null
  const openSelection = useRef<Selection | null>(null)
  const pendingSelection = useRef<Selection | null>(null)
  const activeScope = useRef(scope)

  useLayoutEffect(() => {
    activeScope.current = scope
    return () => {
      activeScope.current = ""
    }
  }, [scope])

  const isDateFiltered = params.ledgerRange !== "all"
  const reviewParams = useMemo<LedgerParams>(
    () =>
      isDateFiltered
        ? {
            ...params,
            ledgerRange: "all",
            startDate: undefined,
            endDate: undefined,
          }
        : params,
    [isDateFiltered, params]
  )

  const select = useCallback((next: Selection | null) => {
    openSelection.current = next
    setSession(next ? { selection: next, pending: false } : null)
  }, [])

  const open = useCallback(
    (id: string) => {
      const nextSelection: Selection = {
        id,
        scope,
        position: !isDateFiltered && page ? getReviewPosition(page, id) : null,
        dayBoundary: page?.ledger?.nextDayUtc,
      }
      select(nextSelection)

      if (!isDateFiltered) return

      const targetRow = page?.items.find((row) => row.id === id)
      void (async () => {
        const fetchReviewPage = (pageNumber: number) =>
          queryClient.fetchQuery(
            transactionsQuery({ ...reviewParams, page: pageNumber })
          )

        const applyFound = (found: ReviewPosition) => {
          setSession((previous) =>
            previous?.selection === nextSelection
              ? { ...previous, resolvedPosition: found }
              : previous
          )
        }

        const firstPage = await fetchReviewPage(1)
        if (openSelection.current !== nextSelection) return
        const foundFirst = getReviewPosition(firstPage, id)
        if (foundFirst) {
          applyFound(foundFirst)
          return
        }

        const totalPages = Math.ceil(firstPage.totalCount / firstPage.pageSize)
        if (totalPages <= 1) return

        if (!targetRow) {
          for (let pageNumber = 2; pageNumber <= totalPages; pageNumber += 1) {
            if (openSelection.current !== nextSelection) return
            const result = await fetchReviewPage(pageNumber)
            if (openSelection.current !== nextSelection) return
            const found = getReviewPosition(result, id)
            if (found) {
              applyFound(found)
              return
            }
          }
          return
        }

        const compareDesc = (
          a: Pick<typeof targetRow, "occurred_at_utc" | "created_at_utc" | "id">,
          b: Pick<typeof targetRow, "occurred_at_utc" | "created_at_utc" | "id">
        ) => {
          if (a.occurred_at_utc !== b.occurred_at_utc) {
            return a.occurred_at_utc > b.occurred_at_utc ? -1 : 1
          }
          if (a.created_at_utc !== b.created_at_utc) {
            return a.created_at_utc > b.created_at_utc ? -1 : 1
          }
          if (a.id !== b.id) return a.id > b.id ? -1 : 1
          return 0
        }

        let low = 2
        let high = totalPages
        while (low <= high && openSelection.current === nextSelection) {
          const mid = Math.floor((low + high) / 2)
          const result = await fetchReviewPage(mid)
          if (openSelection.current !== nextSelection) return
          const found = getReviewPosition(result, id)
          if (found) {
            applyFound(found)
            return
          }
          const firstItem = result.items[0]
          const lastItem = result.items[result.items.length - 1]
          if (!firstItem || !lastItem) return
          if (compareDesc(targetRow, firstItem) < 0) {
            high = mid - 1
          } else if (compareDesc(targetRow, lastItem) > 0) {
            low = mid + 1
          } else {
            return
          }
        }
      })()
    },
    [isDateFiltered, page, queryClient, reviewParams, scope, select]
  )

  const heldPosition = session?.savedPosition ?? null
  const livePosition =
    !isDateFiltered && page && selection
      ? getReviewPosition(page, selection.id)
      : null
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
        session?.resolvedPosition ??
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
    function updateSession(changes: Partial<Omit<ReviewSession, "selection">>) {
      setSession((previous) =>
        previous?.selection === current ? { ...previous, ...changes } : previous
      )
    }
    pendingSelection.current = current
    updateSession({ pending: true, error: undefined })

    const loadPage = async (number: number) => {
      const result = await queryClient.fetchQuery({
        ...transactionsQuery({ ...reviewParams, page: number }),
        staleTime: 0,
      })
      if (
        !isDateFiltered &&
        isRelativeRange &&
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
      if (!isDateFiltered && page?.page !== destination.page)
        onPageChange(destination.page)
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
      updateSession({ savedPosition: captured })
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
        updateSession({
          error:
            cause instanceof Error
              ? cause.message
              : "Could not move to another transaction.",
        })
    } finally {
      if (pendingSelection.current === current) {
        pendingSelection.current = null
        updateSession({ pending: false })
      }
    }
  }

  return {
    transactionId: selection?.id ?? null,
    previous: Boolean(position?.previous),
    next: Boolean(position?.next),
    pending: session?.pending ?? false,
    error: session?.error,
    open,
    close: () => {
      if (selection) select(null)
    },
    navigate: (direction: ReviewDirection) => run({ direction }),
    write: (write: () => Promise<unknown>, advance: boolean) =>
      run({ write, direction: advance ? "next" : undefined }),
  }
}
