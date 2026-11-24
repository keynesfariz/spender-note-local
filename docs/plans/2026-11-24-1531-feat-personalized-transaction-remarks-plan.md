---
title: Personalized Transaction Remarks - Plan
type: feat
date: 2026-11-24
topic: personalized-transaction-remarks
artifact_contract: ce-unified-plan/v1
product_contract_source: ce-brainstorm
execution: code
---

# Personalized Transaction Remarks - Plan

## Goal Capsule

- **Objective** — Transaction rows show remarks in the user's preferred style. Users can edit long notes and see what Magic changed before saving.
- **Product authority** — The confirmed Product Contract below governs this change to Magic and transaction display.
- **Open blockers** — None.

---

## Product Contract

### Summary

Show an editable **Remark** as the transaction title. Magic learns formatting from saved corrections and uses the current purchase's linked emails for the facts. Give long notes their own tab and show which draft fields Magic updated.

### Problem Frame

The transaction list displays Merchant, which also drives merchant spending groups. Magic fills that field from linked emails using a prompt that strips delivery platform names. That instruction favors a short vendor label and can reduce a useful display title like `GrabFood: Moon Chicken` to `GrabFood`. The named transaction has four linked emails, including a receipt that names Moon Chicken. Magic currently receives the emails without the saved title or prior corrections.

Transaction Details gives Note a single-line input, which makes long itemized receipts hard to read and edit. Magic shows a generic completion message, so the user has to inspect the form to find changed fields.

### Key Decisions

- **Give the list a dedicated Remark** (session-settled: user-approved — chosen over repurposing Merchant: display titles can carry platform context while merchant spending groups use a vendor identity). Governs R1, R2, R3.
- **Learn automatically from saved corrections** (session-settled: user-directed — chosen over marking examples individually or maintaining naming rules: editing a remark is enough to teach Magic). Governs R5, R6.

### Requirements

**Transaction display**

- R1. Show Remark as the title in the transaction list and Transaction Details. Display the existing title for records that lack a Remark.
- R2. Replace the editable Merchant field with Remark in Transaction Details and Combine. Keep Note available for order details.
- R3. Use merchant identity for merchant spending groups independently of the Remark text.
- R4. Preserve existing user-entered titles when introducing Remark.

**Magic suggestions**

- R5. Use relevant saved remark edits as style examples for later Magic runs. A discarded draft or an unedited Magic suggestion does not teach a preference.
- R6. Include recognizable historical corrections such as `GrabFood: Moon Chicken` as examples. A transaction-wide edit flag alone is insufficient evidence that the title was corrected.
- R7. Populate names from the current transaction's linked emails. Use prior examples for phrasing and structure.
- R8. For a new remark that follows the learned delivery pattern, use the identified service alone when the attached emails give no restaurant name.
- R9. Preserve a user-entered remark already present when Magic runs. Magic can still suggest other fields.
- R10. Apply new Remark suggestions to the draft for review. Persist the final value through Save or Combine.

**Notes and review**

- R11. Add a **Note** tab beside **Details** and **Source Emails** in Transaction Details. Use a multiline textarea that preserves line breaks.
- R12. Show whether the current Note draft is empty or has content on the tab itself. Treat whitespace-only text as empty. Convey both states without relying on color.
- R13. Keep the same unsaved draft across tab switches and make Save available while editing the Note tab.
- R14. After a successful Magic run in Transaction Details or Combine, show a visible summary naming the fields whose draft values changed during that run. Base the summary on the values before and after applying Magic so existing manual edits and repeated suggestions do not appear as Magic changes.
- R15. If Magic leaves the draft unchanged, show “Magic made no changes.” In Transaction Details, keep the summary visible across tabs so a Note update is discoverable from Details.

### Key Flows

- F1. The user saves a remark edit in Transaction Details. On a later purchase, Magic combines relevant saved examples with the linked email evidence. The user reviews or adjusts the draft remark and saves it to the list. Covers R1, R5, R7, R10.
- F2. The user runs Magic on a transaction with a remark they already entered. The remark stays in the draft while Magic can fill other fields. Covers R9.
- F3. The user runs Magic from Details and sees that Note changed. They open Note, review or edit the multiline draft, and save it from that tab. Covers R11–R15.

### Acceptance Examples

- AE1. The existing transaction `05e5f5fe-6eaf-4c0f-837a-dc26ef15ab31` continues to show `GrabFood: Moon Chicken` after the change and after running Magic again. Covers R1, R4, R9.
- AE2. Given the saved GrabFood example and a new GrabFood order whose receipt names Warung Sari, Magic suggests `GrabFood: Warung Sari`. After Save or Combine, that text appears in the list. Covers R5–R7, R10.
- AE3. Given a new GrabFood order whose emails identify only the service, Magic suggests `GrabFood`. An unrelated saved restaurant name does not appear in the suggestion. Covers R7, R8.
- AE4. Given two transactions for the same merchant identity, editing either remark leaves both purchases in the same merchant spending group. Covers R3.
- AE5. Given a discarded draft correction and a historical transaction whose only edit was to another field, neither is treated as a saved remark correction. Covers R5, R6.
- AE6. Given a blank Note, its tab shows the empty state. Pasting a multiline receipt changes the cue to show content. Switching tabs retains the text, and clearing it returns the cue to empty. Covers R11–R13.
- AE7. Given a manually entered Remark and a Magic run that updates only Category and Note, the summary names Category and Note. The Note tab shows the draft's new content state. Running Magic again with the same applied values shows “Magic made no changes.” Covers R9, R12, R14, R15.

### Scope Boundaries

- Learning applies when the user runs Magic. Gmail sync and earlier transactions receive no automatic suggestions as a result of a new correction.
- Duplicate Merge keeps its existing workflow. Remark suggestions are available in Transaction Details and Combine.

### Sources / Research

- The original [Magic LLM enrichment plan](2026-11-23-2222-feat-magic-llm-transaction-enrichment-plan.md) records why the vendor name drove both the title and merchant grouping.
- The [enrichment prompt](../../internal/llm/enrich.go#L102), [linked-email assembly](../../internal/domain/enrichment.go#L181), and [merchant grouping](../../internal/domain/transactions.go#L175) define the current behavior.
- The [Transaction Details drawer](../../components/transaction-inspector-drawer.tsx#L444) and [Combine dialog](../../components/manual-merge-dialog.tsx#L242) show where Magic reports completion today.
- The named transaction was checked against local SQLite. Two card-charge emails and two receipts are linked. One receipt contains the restaurant name.

---

## Planning Contract

### Key Technical Decisions

- Store the optional remark and its user-edit provenance with the transaction. Leave historical merchant data intact. Backfill a recognizable legacy compound title when linked source evidence supports both components and the title differs from the linked extractions.
- Keep the existing Go routes and merchant payloads compatible. Extend list/detail, update, combine, and Magic preview contracts. Give Magic a bounded set of saved remark examples matched against current source facts. Keep example text separate from instructions.
- Retain one draft across inspector tabs. Use the existing live status region and compare applied field values for each run.

## Implementation Units

### U1. Persist and display transaction remarks

- **Covers** R1–R4, R6.
- Add a reversible SQLite migration, extend row projections and search, and carry remarks through PATCH and Combine. Keep merchant spending queries independent of the remark.
- Verify that a historical title survives migration, an imported title stays a fallback, and saving a remark preserves merchant grouping.

### U2. Suggest and preserve personalized remarks

- **Covers** R5–R10. Depends on U1.
- Extend the enrichment prompt and preview for remarks. Supply relevant user-authored examples. Track whether the saved remark came from a draft edit or from Magic. Preserve a user-entered remark when applying the preview.
- Verify that the named historical example is eligible, other-field edits are not learned, and a new purchase's receipt supplies its own restaurant.

### U3. Add the Note tab and Magic feedback

- **Covers** R11–R15. Depends on U2.
- Use the drawer's existing tabs and form submission. Label the multiline editor, show the current draft's content state, and compare applied draft values in Details and Combine.
- Verify that multiline notes survive tab changes and save, a Note update is visible from Details, and repeated identical suggestions report no changes.

## Verification Contract

- Run scoped Go tests, regenerate and check API types, and run frontend typechecking, lint on changed frontend files, and the Next.js build.
- Exercise the migration using an isolated copy of the ledger. Check the GrabFood record's preserved title and eligibility alongside a fixture with an unrelated edit.
- Check the note editor, shared save flow, and Magic change reporting against the acceptance examples.

## Definition of Done

- The remark remains independent of merchant grouping, the historical example remains visible, and Magic only learns from saved user examples.
- Details and Combine report applied changes before saving. Transaction Details exposes an editable multiline Note with a content cue.
