import assert from "node:assert/strict"
import { test } from "node:test"

import { minorToDecimalInput, parseDecimalToMinorClient } from "./format"

test("amount entry accepts valid separators without inflating decimal IDR", () => {
  assert.equal(parseDecimalToMinorClient("Rp 10000,00", "IDR"), 10000)
  assert.equal(parseDecimalToMinorClient("10.000,00", "IDR"), 10000)
  assert.equal(parseDecimalToMinorClient("10,000.00", "IDR"), 10000)
  assert.equal(parseDecimalToMinorClient("10,000", "IDR"), 10000)
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
