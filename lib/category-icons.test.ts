import assert from "node:assert/strict"
import { test } from "node:test"
import { dynamicIconImports } from "lucide-react/dynamic"

import { resolveCategoryIconName, searchCategoryIcons } from "./category-icons"

test("saved category icon names resolve to loadable Lucide glyphs", async () => {
  const examples = [
    ["Home", "home"],
    ["ShoppingCart", "shopping-cart"],
    ["Building2", "building-2"],
    ["Gamepad2", "gamepad-2"],
    ["  Croissant  ", "croissant"],
    ["croissant", "croissant"],
  ]

  for (const [storedName, expected] of examples) {
    const name = resolveCategoryIconName(storedName)
    assert.equal(name, expected)
    assert.ok(name)
    assert.ok((await dynamicIconImports[name]()).__iconData)
  }
})

test("missing and unknown names can use the category icon fallback", () => {
  for (const name of [
    null,
    undefined,
    "",
    "   ",
    "unknown-icon",
    "constructor",
  ]) {
    assert.equal(resolveCategoryIconName(name), undefined)
  }
})

test("catalog search accepts display names and ranks exact matches first", () => {
  const results = searchCategoryIcons("shopping-cart")
  assert.equal(results[0], "shopping-cart")
  assert.deepEqual(searchCategoryIcons("  SHOPPING CART "), results)
  assert.deepEqual(searchCategoryIcons("ShoppingCart"), results)
  assert.equal(searchCategoryIcons("croissant")[0], "croissant")
  assert.deepEqual(searchCategoryIcons("unknown-icon"), [])
})

test("search keeps digits that are part of canonical icon names", () => {
  assert.equal(searchCategoryIcons("grid-2x2")[0], "grid-2x2")
  assert.equal(searchCategoryIcons("GRID 3X3 OFF")[0], "grid-3x3-off")
  assert.equal(searchCategoryIcons("AXIS-3D")[0], "axis-3d")
})

test("the unfiltered catalog is ordered and includes icons across pages", () => {
  const names = searchCategoryIcons("")
  assert.deepEqual(names, names.toSorted())
  assert.ok(names.length > 48)
  assert.ok(names.includes("croissant"))
  assert.ok(names.includes("zap"))
})
