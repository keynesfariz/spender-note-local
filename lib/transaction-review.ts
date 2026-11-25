import type {
  PaginatedTransactionsResponse,
  TransactionListItem,
} from "./api/types.generated"

type ReviewRow = Pick<
  TransactionListItem,
  "id" | "occurred_at_utc" | "created_at_utc"
>
type ReviewPage = Pick<
  PaginatedTransactionsResponse,
  "page" | "pageSize" | "totalCount"
> & { items: ReviewRow[] }

export type ReviewDirection = "previous" | "next"

type ReviewNeighbor = { page: number; index: number; id?: string }

export type ReviewPosition = {
  page: number
  index: number
  row: ReviewRow
  previous: ReviewNeighbor | null
  next: ReviewNeighbor | null
}

export function getReviewPosition(
  { page, pageSize, totalCount, items }: ReviewPage,
  id: string
): ReviewPosition | null {
  const index = items.findIndex((row) => row.id === id)
  if (index < 0) return null

  const start = (page - 1) * pageSize
  function neighbor(offset: number): ReviewNeighbor | null {
    const rank = start + index + offset
    if (rank < 0 || rank >= totalCount) return null
    const targetPage = Math.floor(rank / pageSize) + 1
    return {
      page: targetPage,
      index: rank % pageSize,
      id: targetPage === page ? items[index + offset]?.id : undefined,
    }
  }

  return {
    page,
    index,
    row: items[index],
    previous: neighbor(-1),
    next: neighbor(1),
  }
}

const reviewOrder = ["occurred_at_utc", "created_at_utc", "id"] as const

function compareReviewRows(a: ReviewRow, b: ReviewRow) {
  for (const field of reviewOrder) {
    if (a[field] !== b[field]) return a[field] > b[field] ? -1 : 1
  }
  return 0
}

export async function resolveReviewNeighbor(
  position: ReviewPosition,
  direction: ReviewDirection,
  loadPage: (page: number) => Promise<ReviewPage>,
  allowOrderFallback = false
): Promise<ReviewPosition | null> {
  const target = position[direction]
  if (!target) return null

  const page = await loadPage(target.page)
  if (!target.id && !allowOrderFallback) {
    const row = page.items[target.index]
    return row &&
      compareReviewRows(row, position.row) * (direction === "next" ? 1 : -1) > 0
      ? getReviewPosition(page, row.id)
      : null
  }

  if (target.id) {
    const destination = getReviewPosition(page, target.id)
    if (destination) return destination
  }

  // A save can shift an unchanged neighbor by at most one row.
  const adjacentPage =
    target.index === 0 && target.page > 1
      ? target.page - 1
      : target.index === page.pageSize - 1 &&
          target.page * page.pageSize < page.totalCount
        ? target.page + 1
        : null
  const adjacent = adjacentPage ? await loadPage(adjacentPage) : null
  if (target.id) return adjacent && getReviewPosition(adjacent, target.id)

  const searchPages = [page, ...(adjacent ? [adjacent] : [])].sort((a, b) =>
    direction === "next" ? a.page - b.page : b.page - a.page
  )
  for (const candidate of searchPages) {
    const isNeighbor = (row: ReviewRow) =>
      row.id !== position.row.id &&
      compareReviewRows(row, position.row) * (direction === "next" ? 1 : -1) > 0
    const row =
      direction === "next"
        ? candidate.items.find(isNeighbor)
        : candidate.items.findLast(isNeighbor)
    if (row) return getReviewPosition(candidate, row.id)
  }
  return null
}

export function getReviewAction(
  hasDraft: boolean,
  reviewed: boolean,
  canAdvance: boolean
) {
  if (hasDraft) return canAdvance ? "save-and-next" : null
  if (reviewed) return null
  return canAdvance ? "mark-and-next" : "mark"
}
