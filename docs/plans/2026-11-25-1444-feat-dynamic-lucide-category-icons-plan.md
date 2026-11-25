---
title: Dynamic Lucide Category Icons - Plan
type: feat
date: 2026-11-25
artifact_contract: ce-unified-plan/v1
product_contract_source: ce-plan-bootstrap
execution: code
---

# Dynamic Lucide Category Icons - Plan

## Goal Capsule

- **Objective** Let users find, select, and see any icon in the installed Lucide catalog when creating or editing a category.
- **Means** Render by name and list choices from Lucide's catalog. See KTD1 and KTD3.
- **Authority** The Product Contract governs behavior. The Planning Contract governs implementation within those requirements.
- **Execution profile** One frontend change in a single PR if the user requests publication.
- **Stop condition** If resolving a saved icon would require changing existing data, surface the incompatible name and its expected glyph before expanding scope.
- **Owner** The implementer delivers the change and completes the Verification Contract. The user controls publication.

## Product Contract

### Summary

Use the installed Lucide catalog as the source for the category icon picker and renderer. Keep the existing preview and grid, with a name search for finding an icon.

### Problem Frame

A Lucide icon currently needs a named import and a registry entry to render, plus a separate picker entry before a user can choose it. The current `Croissant` edit demonstrates the gap. It adds the icon to the renderer while the picker still omits it.

### Requirements

**Picker**

- R1. Make every icon name in the installed catalog discoverable by search and selectable in the existing category icon grid.
- R2. Keep the current create and edit flows, including the preview and selected state.

**Rendering**

- R3. Render both existing saved names and newly selected names on category rows, transaction badges, and category selectors using the existing size and color styling.
- R4. Keep a visible `Tag` fallback for unknown or empty names and reserve the icon's layout space during loading.

### Scope Boundaries

- Repository changes are limited to the category icon frontend and its verification.
- The current category API and database values remain the persistence contract. Bundle size has no acceptance target.

## Planning Contract

### Key Technical Decisions

- KTD1. **Use the public Lucide dynamic entry point.** Wrap Lucide's `DynamicIcon` in `components/dynamic-icon.tsx` and use its `iconNames` export as the single catalog. Keep the wrapper's existing props and decorative SVG semantics so callers continue to supply the same classes. Lucide 1.79.0 exposes both through `lucide-react/dynamic`. Its renderer loads glyphs after mount, so reserve the existing icon dimensions until they arrive. [Lucide Dynamic Icon docs](https://lucide.dev/guide/react/advanced/dynamic-icon-component)
- KTD2. **Resolve saved names without rewriting them.** Match a catalog name directly, then try a trimmed, kebab-case conversion for legacy names such as `ShoppingCart` and `Building2`. Validate the result against the catalog before calling Lucide. All 49 names in the current registry, including the in-progress `Croissant` entry, have a matching key. Preserve the stored value when editing a category without picking another icon. A new choice saves the catalog key through the existing mutation. Use a stable-size loading placeholder and reset Lucide's renderer when the resolved name changes so the preview cannot show the prior glyph while the new one loads.
- KTD3. **Search the full catalog and render one page of previews.** Use deterministic name ordering and case-insensitive matching that accepts names such as `Shopping Cart` or `shopping-cart`. Rank exact matches first. Keep the grid and show up to 48 results at a time, with previous and next controls when needed. Reset the result page when the search changes or the dialog opens. Identify the selected icon by the resolved key and keep its preview visible when search or pagination hides its tile. This limits simultaneous requests without restricting which names are available.

## Implementation Units

### U1. Render catalog and saved icon names

- **Goal** Resolve a stored category icon name to the right Lucide glyph.
- **Requirements** R3, R4.
- **Dependencies** None.
- **Files** Update `components/dynamic-icon.tsx`. Add `lib/category-icons.ts` and `lib/category-icons.test.ts`.
- **Approach** Add the catalog and resolver from KTD1 and KTD2 in the shared module. Replace the renderer's manual component map. Keep the current picker options until U2 switches its consumer. Use the `Tag` SVG for an unresolved name and apply the existing size, shrink, and color classes to the loading and loaded states.
- **Patterns** Follow the TypeScript tests in `lib/format.test.ts` and the icon consumers in `components/transaction-list.tsx` and `components/add-transaction-dialog.tsx`.
- **Test scenarios**
  - `Home`, `ShoppingCart`, `Building2`, `Gamepad2`, and `croissant` resolve to loadable catalog keys.
  - Empty values and names outside the catalog take the fallback path. A padded valid name still resolves.
  - A saved legacy name renders in an existing row or selector. Switching the preview while the next glyph loads holds its space and clears the prior glyph.
- **Verification** Module tests protect name resolution. A UI smoke check covers async SVG loading and layout, which the module tests cannot observe.

### U2. Replace the manual picker list

- **Goal** Make the installed catalog available from the existing category form.
- **Requirements** R1, R2, R3.
- **Dependencies** U1.
- **Files** Update `components/categories-tags-view.tsx` and remove the old picker options from `components/dynamic-icon.tsx`. Extend `lib/category-icons.ts` and `lib/category-icons.test.ts` for search.
- **Approach** Feed the existing grid and tooltips from KTD3. Keep the selected preview independent of search and use the existing category mutation for saves under KTD2.
- **Patterns** Reuse the create and edit handlers, icon grid styling, and accessible option labels in `components/categories-tags-view.tsx`.
- **Test scenarios**
  - Search for `Croissant` and choose it. Save the category, then reopen it and verify the same icon is selected and visible.
  - Search terms with spaces, hyphens, or different casing find the same icon. A query with no matches shows an empty state without changing the selected icon.
  - Browse multiple result pages and change the search. Results are bounded to one page and the new search returns to its first page.
  - Edit a category with stored `ShoppingCart` and save a name change. Its icon stays selected and the existing icon value is preserved.
- **Verification** Module tests cover catalog filtering and ordering. The create and edit smoke checks cover selection and persistence through the existing UI.

## Verification Contract

- Run `bun test lib/category-icons.test.ts`, `bun run typecheck`, and `bun run lint`.
- Run `bun run build` to verify Next.js can compile the dynamic icon entry point.
- Smoke check the category dialog, a saved category row, a transaction badge, and a category selector. Confirm the new icon survives save and reopen.

## Definition of Done

- A catalog icon can be found, chosen, and rendered without adding a named import or maintaining the old lists.
- Legacy names and fallback behavior pass U1 verification. The create and edit flows pass U2 verification.
- The frontend checks and build pass, and any temporary test or investigation code is removed.
