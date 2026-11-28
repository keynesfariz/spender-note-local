import assert from "node:assert/strict"
import { test } from "node:test"

import {
  buildInspectorPatch,
  type InspectorDraftField,
} from "./transaction-inspector-patch"

const draft = {
  baseline: {
    currency: "USD",
    date: "2026-11-24",
    time: "08:00",
    timezone: "Asia/Jakarta",
  },
  edits: new Set<InspectorDraftField>(),
  remark: " existing ",
  merchant: "Store",
  remarkUserEdited: true,
  amount: "123",
  category: "Shopping",
  selectedTagIds: [],
  draftTagNames: [],
  accountId: null,
  date: "2026-11-24",
  time: "08:00",
  note: " ",
  convertToLocalCurrency: false,
}

const transaction = {
  id: "tx",
  currency: "USD",
  isIncome: false,
  receivingCurrency: "JPY",
  localCurrency: "KWD",
}

test("serializes only edited fields and preserves explicit clears", () => {
  assert.equal(
    JSON.stringify(buildInspectorPatch(draft, transaction)),
    '{"id":"tx"}'
  )

  const edits = new Set<InspectorDraftField>(["tags", "account", "note"])
  const payload = JSON.parse(
    JSON.stringify(buildInspectorPatch({ ...draft, edits }, transaction))
  )
  assert.deepEqual(payload, {
    id: "tx",
    tag_ids: [],
    tag_names: [],
    account_id: null,
    note: null,
  })
})

test("uses receiving currency for income and local currency for conversion", () => {
  const edits = new Set<InspectorDraftField>(["amount"])
  assert.equal(
    buildInspectorPatch({ ...draft, edits }, transaction).amount_minor,
    12300
  )
  assert.equal(
    buildInspectorPatch({ ...draft, edits }, { ...transaction, isIncome: true })
      .amount_minor,
    123
  )
  const patch = buildInspectorPatch(
    { ...draft, convertToLocalCurrency: true },
    transaction
  )
  assert.equal(patch.amount_minor, 123000)
  assert.equal(patch.convertToLocalCurrency, true)

  assert.throws(
    () =>
      buildInspectorPatch(
        { ...draft, edits },
        { ...transaction, currency: "EUR" }
      ),
    /currency changed/
  )
})

test("uses the baseline timezone for edited date and time", () => {
  const edits = new Set<InspectorDraftField>(["date"])
  assert.equal(
    buildInspectorPatch({ ...draft, edits }, transaction).occurred_at_utc,
    undefined
  )
  assert.equal(
    buildInspectorPatch({ ...draft, edits, time: "07:15" }, transaction)
      .occurred_at_utc,
    "2026-11-24T00:15:00Z"
  )
  assert.throws(
    () => buildInspectorPatch({ ...draft, edits, time: "" }, transaction),
    /Enter the occurred date and time/
  )
})
