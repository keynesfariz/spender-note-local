import assert from "node:assert/strict"
import { test } from "node:test"

import {
  dateTimeInputToUtcIso,
  minorToDecimalInput,
  parseDecimalToMinorClient,
  utcToTimeInputValue,
} from "./format"

test("amount entry accepts valid separators without inflating decimal IDR", () => {
  assert.equal(parseDecimalToMinorClient("Rp 10000,00", "IDR"), 10000)
  assert.equal(parseDecimalToMinorClient("10.000,00", "IDR"), 10000)
  assert.equal(parseDecimalToMinorClient("10,000.00", "IDR"), 10000)
  assert.equal(parseDecimalToMinorClient("10,000", "IDR"), 10000)
  assert.equal(parseDecimalToMinorClient("1.234", "IDR"), 1234)
  assert.throws(() => parseDecimalToMinorClient("1.234", "USD"), /at most 2/)
  assert.equal(parseDecimalToMinorClient("12,34", "EUR"), 1234)
  assert.equal(parseDecimalToMinorClient("1.234,56", "EUR"), 123456)
  assert.equal(parseDecimalToMinorClient("-1,234.56", "USD"), -123456)
  assert.throws(() => parseDecimalToMinorClient("1,2,3", "USD"))
  assert.throws(() => parseDecimalToMinorClient("12,34.50", "USD"))
  assert.throws(() => parseDecimalToMinorClient("10000,01", "IDR"))
})

test("amount entry and display use the shared currency precision", () => {
  assert.equal(parseDecimalToMinorClient("100", "CLP"), 100)
  assert.equal(minorToDecimalInput(100, "CLP"), "100")
  assert.equal(parseDecimalToMinorClient("1.234", "LYD"), 1234)
  assert.equal(minorToDecimalInput(1234, "LYD"), "1.234")
  assert.equal(parseDecimalToMinorClient("1.2345", "CLF"), 12345)
  assert.equal(minorToDecimalInput(12345, "CLF"), "1.2345")
})

test("time inputs can preserve local seconds across a date edit", () => {
  const timestamp = "1900-01-01T00:00:00Z"
  const timezone = "Asia/Jakarta"
  assert.equal(utcToTimeInputValue(timestamp, timezone), "07:07")
  const timeWithSeconds = utcToTimeInputValue(timestamp, timezone, true)
  assert.equal(timeWithSeconds, "07:07:12")
  assert.equal(
    dateTimeInputToUtcIso("1900-01-02", timeWithSeconds, timezone),
    "1900-01-02T00:00:00Z"
  )
})
