import assert from "node:assert/strict"
import { test } from "node:test"

import {
  getReviewAction,
  getReviewPosition,
  resolveReviewNeighbor,
} from "./transaction-review"

function page(number: number, ids: string[], totalCount: number) {
  return {
    page: number,
    pageSize: 2,
    totalCount,
    items: ids.map((id) => ({
      id,
      occurred_at_utc: "2026-11-25T00:00:00Z",
      created_at_utc: "2026-11-25T00:00:00Z",
    })),
  }
}

test("review navigation uses the neighboring page in either direction", async () => {
  const first = page(1, ["d", "c"], 4)
  const second = page(2, ["b", "a"], 4)
  const load = async (number: number) => (number === 1 ? first : second)
  const lastOnFirst = getReviewPosition(first, "c")!
  const firstOnSecond = getReviewPosition(second, "b")!

  assert.equal(
    (await resolveReviewNeighbor(lastOnFirst, "next", load))?.row.id,
    "b"
  )
  assert.equal(
    (await resolveReviewNeighbor(firstOnSecond, "previous", load))?.row.id,
    "c"
  )
  assert.equal(getReviewPosition(first, "d")?.previous, null)
  assert.equal(getReviewPosition(second, "a")?.next, null)
  assert.equal(getReviewPosition(page(1, ["a"], 1), "a")?.next, null)
})

test("review navigation follows the captured ID when marking shifts the page", async () => {
  const position = getReviewPosition(page(1, ["c", "b"], 3), "b")!
  const target = await resolveReviewNeighbor(position, "next", async () =>
    page(2, ["a"], 3)
  )
  const updatedPosition = {
    ...position,
    next: target && { page: 2, index: 0, id: target.row.id },
  }
  const destination = await resolveReviewNeighbor(
    updatedPosition,
    "next",
    async (number) => (number === 1 ? page(1, ["c", "a"], 2) : page(2, [], 2))
  )

  assert.equal(destination?.row.id, "a")
  assert.equal(destination?.page, 1)
})

test("previous follows its captured ID when a date edit shifts it to a later page", async () => {
  const position = getReviewPosition(page(2, ["b", "a"], 4), "b")!
  const updatedPosition = {
    ...position,
    previous: { page: 1, index: 1, id: "c" },
  }
  const destination = await resolveReviewNeighbor(
    updatedPosition,
    "previous",
    async (number) =>
      number === 1 ? page(1, ["b", "d"], 4) : page(2, ["c", "a"], 4)
  )

  assert.equal(destination?.row.id, "c")
  assert.equal(destination?.page, 2)
})

test("review action reflects draft, review status, and the end of the list", () => {
  assert.equal(getReviewAction(false, false, true), "mark-and-next")
  assert.equal(getReviewAction(false, false, false), "mark")
  assert.equal(getReviewAction(true, false, true), "save-and-next")
  assert.equal(getReviewAction(true, true, true), "save-and-next")
  assert.equal(getReviewAction(true, false, false), null)
  assert.equal(getReviewAction(false, true, true), null)
})

test("navigation finds the shifted successor when a standalone save did not read its page", async () => {
  const position = getReviewPosition(page(1, ["c", "b"], 3), "b")!
  const destination = await resolveReviewNeighbor(
    position,
    "next",
    async (number) => (number === 1 ? page(1, ["c", "a"], 2) : page(2, [], 2)),
    true
  )

  assert.equal(destination?.row.id, "a")
  assert.equal(destination?.page, 1)
})
