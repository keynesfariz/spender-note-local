---
title: Assign cross-route components to the domain that owns the workflow
date: 2026-11-28
category: design-patterns
module: UI Components
problem_type: design_pattern
component: frontend
severity: low
applies_when:
  - Moving frontend components into domain folders
  - Adding another page that uses an existing workflow
  - Moving a lazily loaded dialog or one of its preloads
tags: [react, frontend, component-ownership, cross-route, dynamic-imports]
---

# Assign cross-route components to the domain that owns the workflow

## Context

The frontend refactor surfaced a distinction between where a component appears and where its behavior belongs. Accounts and Transactions both open the same transfer workflow. The locale setup alert appears across several pages, and the app shell mounts the sync toast. Looking only at page imports leaves out consumers mounted by the layout and dialogs opened on demand.

## Guidance

Keep frontend components under `components/`. Put a component that knows about a feature's rules, payload, or status in that feature's folder. Give cross-route callers a direct import from the same owner. Keep generic controls such as pagination and drawer content available under the shared component folders.

[TransferFormDialog](../../../components/transfer/transfer-form-dialog.tsx) belongs to `components/transfer/`. [Accounts](../../../components/accounts/accounts-view.tsx) and the [transaction inspector](../../../components/transactions/transaction-inspector-form.tsx) import it directly. The [Transactions view](../../../components/transactions/transactions-view.tsx) loads it dynamically. The [app shell](../../../components/app-shell.tsx) mounts [SyncToast](../../../components/sync/sync-toast.tsx), whose polling and retry behavior give it a home in `components/sync/`.

When moving a component, search for its old path and exported names across page and layout imports, type imports, dynamic loaders, and preloads. Update those paths in the same change and preserve when each caller loads the component. Open each changed entry point during verification so that a successful page load also covers the panels and dialogs loaded on demand.

## Related

- [Reuse shared page layouts and status indicators](reuse-feature-ui-composition.md)
