---
title: Frontend domain ownership and readability
artifact_contract: ce-unified-plan/v1
product_contract_source: ce-brainstorm
execution: code
---

## Goal Capsule

**Objective** — Developers can find and change a feature's UI and behavior from its domain folder. Common behavior has one implementation, and large files have clear responsibilities.

**Means** — Keep all frontend components under `components/` and organize feature-specific components into domain subfolders.

**Product authority** — The user chose a single pass across the frontend and asked for a walkthrough in small steps.

**Open blockers** — None for requirements planning.

## Product Contract

### Problem Frame

Feature-specific screens sit alongside generic controls and app infrastructure directly under `components/`. Transaction code alone spans multiple large files, including the 1,127-line inspector. Finding the right place to make a change requires knowing how these files are wired together.

### Requirements

- R1. Cover Dashboard, Transactions, Accounts, Categories, Import, and Settings in a single frontend refactor with small, independently verifiable checkpoints.
- R2. Keep all frontend components under `components/`. Place feature-owned components in domain subfolders such as `components/accounts/` and `components/transactions/`. Reserve the top level of `components/` for shared UI and shared app infrastructure.
- R3. Give cross-route feature components a single owner and keep imports from other domains explicit. Trace imports from all six pages and the app shell, including dynamic imports. Identify each shared feature, name its owner and consumers, and keep its UI in the owning domain folder.
- R4. Review hand-written frontend files above roughly 300 lines. Split along meaningful responsibilities and keep larger cohesive files where a split would make the flow harder to follow.
- R5. Consolidate duplicated UI or logic where consumers depend on the same behavior. Keep distinct workflow rules with their owners.
- R6. Preserve routes, UI behavior, and API contracts throughout the refactor, including URL state and existing lazy-loading boundaries.

### Key Decisions

- **Frontend-wide scope** (session-settled: user-directed — chosen over a Transactions pilot and a rollout across separate domain efforts). Work through the agreed scope as one pass and explain each checkpoint.
- **Cohesion guides file size**. Use roughly 300 lines to identify candidates for review. Give each extracted piece a clear purpose.
- **Transfers have a dedicated domain folder**. Accounts and Transactions both use the transfer workflow, so both import its components from `components/transfer/`.

### Frontend ownership map

The user approved this ownership map for the frontend-wide refactor.

| Home | Components and consumers |
| --- | --- |
| `components/accounts/` | Accounts view and account create/edit, reconcile, and merge dialogs. |
| `components/transactions/` | Transactions view, list, inspector, create dialog, form, Combine dialog, Magic enrichment, reviewed indicator, and skipped list. These controls serve multiple transaction flows within the same domain. |
| `components/transfer/` | Transfer dialog. Accounts and Transactions open it directly, and the transaction inspector also uses it. The transfer workflow owns the dialog. |
| `components/categories/` | Categories and Tags management view and category picker and badge. Transactions consumes the picker in create/edit and Combine, and the badge in its main and skipped lists. |
| `components/tags/` | Tag picker. Transaction forms and Combine use it to edit tags. Categories and Tags remains the route-level composition for managing both libraries. |
| `components/dashboard/` | Dashboard view, monthly summary, and spending breakdown panels. |
| `components/import/` | Manual Import view and any parts extracted from it. |
| `components/settings/` | Settings view and settings tabs. Keep the triage rules editor here as Settings UI. The Transactions undo toast and skipped list belong to Transactions. |
| `components/locale/` | Locale setup alert. Accounts, Dashboard, Import, and Transactions use it. The locale settings tab remains the Settings entry point. |
| `components/sync/` | Sync toast mounted by the app shell. Its global placement reflects where notifications appear, while its behavior belongs to sync. |
| `components/` and `components/ui/` | App shell, providers, theme provider, page layout, generic drawer and confirmation content, date picker, pagination, search input, icon helpers, and UI primitives. |

Check this map against the import graph before moving files. Record any additional shared feature with its consumers and owning workflow. A shared route consumer is evidence to inspect ownership. Generic UI belongs with shared components, and workflow-specific UI belongs with the domain that owns its behavior.

### Scope Boundaries

The work covers frontend organization, component decomposition, and verified duplication. Backend rewrites, UI redesign, dependency upgrades, and new features are outside this refactor.

### Success Criteria

- A developer can start from any of the six pages and follow its main UI into the owning subfolder under `components/`. Each shared feature in the ownership map resolves to one folder from all of its entry points. Files directly under `components/` provide reusable UI or shared infrastructure.
- Reviewers can distinguish file moves, responsibility splits, and duplicate removal. Each retained large file has a clear reason to stay cohesive.
- Lint, type checking, and the production build pass. Verify representative workflows across all six pages, with focused checks for transaction draft and discard behavior, Magic enrichment, Combine, and transfers from both entry points.

## Planning Contract

Keep page entry points in `app/`. Preserve server and client boundaries, dynamic imports and preload callers, and the ownership map above. Use direct imports to the owning folder and local relative imports within a domain where consistent with nearby code. Keep existing public component exports and props through the move checkpoints. Review `node_modules/next/dist/docs/01-app/01-getting-started/05-server-and-client-components.md` before editing client entry points.

Treat each unit as a review checkpoint within the same frontend-wide refactor. Report what moved or changed, valuable exceptions to splitting, and the checks that ran before moving on. Run `bun run typecheck` and lint affected files at each code checkpoint. Start with a clean baseline or record existing failures so regressions can be attributed.

## Implementation Units

### U1. Verify the move list and baseline (R1–R3, R6)

Check the approved map against `app/**/page.tsx`, `app/layout.tsx`, and all consumers of the files in `components/`. Include type imports, dynamic imports, preload callbacks, and references in tests or tooling. Confirm the destination for each current component and flag any historical links in repo guidance that would break. Run baseline `bun run lint`, `bun run typecheck`, and `bun test lib`. Record failures before editing.

### U2. Move shared feature UI (R2, R3, R6)

Move `transfer-form-dialog.tsx`, `category-field.tsx`, `category-badge.tsx`, `tag-multi-select.tsx`, `locale-setup-alert.tsx`, and `sync-toast.tsx` into the approved folders. Update consumers in the same checkpoint. Keep transfer's lazy loading in Transactions and its existing entry points in Accounts and the inspector. Check that imports resolve from both transfer routes, all four locale-alert consumers, and the app shell.

### U3. Move route-owned UI (R1–R3, R6)

After U2, move Transactions and Accounts first, then Categories and Dashboard, then Import and Settings. Treat those groups as separate review checkpoints. Update each page and its full component import graph, including dynamic targets for create, Combine, the inspector, and the skipped list. Move `triage-undo-toast.tsx` with Transactions and `settings-view.tsx` alongside the existing Settings tabs. Verify all six route imports and keep top-level shared components and `components/ui/` accessible to all domains.

### U4. Split the transaction and transfer workflows (R4, R6)

After U3, review these boundaries in separate checkpoints.

1. **Inspector.** Extract the source-email viewer from `transaction-inspector-drawer.tsx` first. Separate form sections from the draft and mutation orchestration next. Keep draft resets, optimistic updates, dirty checks, and save-and-advance under one owner. Verify dirty navigation, cancel and confirm discard, review selection, and pending-tag coordination with Magic enrichment.
2. **List and page.** Pull presentational filters or selection actions out of `transaction-list.tsx` and dialog rendering out of `transactions-view.tsx` where the resulting inputs stay small. Preserve URL state, query keys, bulk selection, and preload timing.
3. **Transfer and Combine.** Move transfer matching UI and skipped-payment evidence into sibling files in `components/transfer/`. Keep target-specific draft initialization and submission coordinated by the dialog. Split Combine by review and confirmation content where cohesive. Preserve matching, account and currency eligibility, and final selected tag resolution.

### U5. Split remaining large components (R4, R6)

After U3, use the existing named sections as the first extraction candidates.

- **Categories and Tags.** Extract the existing category and tag editors and deletion dialogs from `categories-tags-view.tsx` into their owning folders. Keep tab selection and management-page composition in the view. Check edit and delete with dependent transactions.
- **Import.** Separate the input and preview rendering in `manual-import-view.tsx`. Keep draft versioning and submit coordination with the view. Verify that replacing JSON while a file read is pending cannot submit a stale preview.
- **Accounts and Settings.** Review `accounts-view.tsx`, account create/edit and reconciliation, and the large Settings tabs. Extract cohesive tables, form sections, or dialogs where they have a narrow interface. Check create/edit, reconciliation, sender and LLM settings, and triage rule edits after their respective splits.
- **Remaining candidates.** Scan hand-written frontend files above roughly 300 lines, including the tag picker and Magic enrichment. Document why retained files stay together. Use responsibility and readability to judge each split.

### U6. Consolidate verified duplication (R5, R6)

After U4 and U5, compare create, edit, and Combine field by field. Use `components/transactions/transaction-form.tsx` and shared domain pickers where behavior already matches. Record differences in labels, requiredness, placement, submitted values, and pending state before extracting anything. Compare category and tag management and account dialogs for repeated presentation or validation with the same contract. Keep each workflow's draft and mutation rules local. Check every caller of each consolidated component.

### U7. Verify and review the full pass (R1–R6)

Run the commands and workflows below. Compare the resulting folder tree to the approved map and search for imports to old paths, including lazy imports. Review moves separately from altered behavior or code consolidation. Update references in active repo guidance where a move leaves a broken path.

## Verification Contract

- **Automated checks.** Run `bun run lint`, `bun run typecheck`, `bun test lib`, `bun run build`, and `git diff --check`. Existing tests cover transaction patching and review, manual import, category icons, and formatting. Add focused coverage only for a specific extraction regression the existing tests miss.
- **Transactions.** Exercise create, edit, discard stay/discard continue, save-and-advance, Magic enrichment with tag selection, Combine, skipped list, and filter and selection behavior after navigation.
- **Accounts and transfer.** Exercise account editing and reconciliation. Open a transfer from Accounts and Transactions, and confirm the inspector entry point still opens in the right mode. Check transfer match selection and submit.
- **Other pages.** Check Dashboard period and breakdown display, Categories and Tags create/edit/delete, Import JSON preview and submit, and Settings tabs including a triage rule edit. Check locale setup links and a sync status transition through the shell. Use local test data for mutating checks.

## Definition of Done

All six areas use the approved component ownership map. Every move and extraction has a checked import path and a relevant workflow result. Large files have been reviewed for responsibility boundaries, and each consolidation uses one implementation for behavior its callers share. The final walkthrough summarizes each checkpoint, the verification results, and any cases that could not be exercised.
