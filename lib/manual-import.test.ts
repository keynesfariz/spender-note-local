import assert from "node:assert/strict"
import { test } from "node:test"

import { buildManualImportPrompt, parseManualImportJson } from "./manual-import"

test("buildManualImportPrompt includes account, locale, categories, tags, custom instructions, and valid JSON example", () => {
  const prompt = buildManualImportPrompt({
    accountName: "BCA Debit",
    accountType: "debit_card",
    currency: "IDR",
    timezone: "Asia/Jakarta",
    categories: ["Food", "Transport", "Other"],
    tags: ["work", "coffee"],
    customInstructions: "Ignore any transactions labelled 'cash cow'.",
  })

  assert.ok(prompt.includes("BCA Debit (debit_card)"))
  assert.ok(prompt.includes("IDR"))
  assert.ok(prompt.includes("Asia/Jakarta"))
  assert.ok(prompt.includes("Food, Transport, Other"))
  assert.ok(prompt.includes("work, coffee"))
  assert.ok(prompt.includes("Ignore any transactions labelled 'cash cow'."))

  const fenceMatch = prompt.match(/```json\n([\s\S]*?)\n```/)
  assert.ok(fenceMatch !== null)
  const parsedExample = parseManualImportJson(fenceMatch[1], {
    defaultCurrency: "IDR",
    timezone: "Asia/Jakarta",
    categories: ["Food & Drink", "Other"],
    tags: ["coffee"],
  })
  assert.equal(parsedExample.error, undefined)
  assert.equal(parsedExample.rows.length, 2)
  assert.equal(
    parsedExample.rows.every((r) => r.valid),
    true
  )
})

test("parseManualImportJson strips markdown fences, matches categories/tags case-insensitively, and normalizes date-only timestamps", () => {
  const fenced = [
    "```json",
    JSON.stringify({
      transactions: [
        {
          date: "2026-11-15",
          amount: 45000,
          kind: "expense",
          merchant: "Kopi Kenangan",
          category: "Dining Out",
          tags: ["Work", "invented-tag"],
          note: "Team coffee",
        },
      ],
    }),
    "```",
  ].join("\n")

  const result = parseManualImportJson(fenced, {
    defaultCurrency: "IDR",
    timezone: "Asia/Jakarta",
    categories: ["Food", "Transport", "Other"],
    tags: ["work", "reimbursable"],
  })
  assert.equal(result.error, undefined)
  assert.equal(result.rows.length, 1)

  const row = result.rows[0]
  assert.equal(row.valid, true)
  assert.equal(row.resolvedCategory, "Other")
  assert.equal(row.categoryMatched, false)
  assert.deepEqual(row.matchedTags, ["work"])
  assert.deepEqual(row.droppedTags, ["invented-tag"])
  assert.equal(row.occurredAtUtc, "2026-11-15T05:00:00Z")
  assert.equal(row.amountMinor, 45000)
})

test("parseManualImportJson flags invalid rows while keeping valid rows selectable", () => {
  const payload = JSON.stringify([
    { date: "2026-11-15", amount: 10000, kind: "expense", merchant: "A" },
    { date: "2026-11-15", amount: 0, kind: "expense", merchant: "Bad Amount" },
    {
      date: "2026-11-15",
      amount: 20000,
      kind: "transfer",
      merchant: "Bad Kind",
    },
    { date: "2026-11-15", amount: 30000, kind: "income", merchant: "Salary" },
  ])

  const result = parseManualImportJson(payload, {
    defaultCurrency: "IDR",
    timezone: "Asia/Jakarta",
    categories: ["Food", "Transport", "Other"],
    tags: ["work", "reimbursable"],
  })
  assert.equal(result.error, undefined)
  assert.equal(result.rows.length, 4)
  assert.equal(result.rows.filter((r) => r.valid).length, 2)
  assert.equal(result.rows[1].valid, false)
  assert.equal(result.rows[2].valid, false)
})

test("parseManualImportJson returns a top-level error on invalid JSON syntax", () => {
  const result = parseManualImportJson("{not valid json", {
    defaultCurrency: "IDR",
    timezone: "Asia/Jakarta",
    categories: ["Other"],
    tags: [],
  })
  assert.equal(result.rows.length, 0)
  assert.ok(result.error?.includes("Invalid JSON syntax"))
})
