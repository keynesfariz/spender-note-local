---
title: Reuse shared page layouts and status indicators
date: 2026-11-26
category: design-patterns
module: UI Components
problem_type: design_pattern
component: frontend
severity: low
applies_when:
  - Adding a page heading with a subtitle or primary actions
  - Showing an existing status in another view
tags: [react, composition, reuse, page-layout, status-indicator]
---

# Reuse shared page layouts and status indicators

## Context

Several views repeated the page title, subtitle, and responsive action layout. The transaction list and inspector also rendered the same Reviewed state differently. Both patterns sit above the individual UI controls, so checking only `components/ui/` can miss a component that already owns the full presentation.

## Guidance

1. Search feature components and neighboring views for the same UI role. Compare callers before choosing props. Check headings, optional content, action placement on narrow screens, and accessible status text.
2. Put shared structure and presentation in one component. Pass page-specific content through props and pass existing controls through a slot so each caller keeps its event handlers and workflow. For a status indicator, let the caller decide when to render it. Keep the icon, tooltip, and accessible label together.
3. Review each caller for behavior at the boundary. For page actions, check visibility during loading, focus and click handlers, and placement relative to filters. For status displays, check the state condition and any adjacent action that changes it.

## Examples

[PageLayout](../../../components/PageLayout.tsx) renders the heading, optional subtitle, optional wrapping action row, and page content. [AccountsView](../../../components/accounts-view.tsx) supplies the Transfer and Add account controls through `actions`, while [SettingsView](../../../components/settings-view.tsx) omits the slot.

[ReviewedIcon](../../../components/reviewed.tsx) keeps the Reviewed tooltip and screen-reader text with the icon. The [transaction list](../../../components/transaction-list.tsx) and [inspector](../../../components/transaction-inspector-drawer.tsx) decide when to render it. The list also owns the separate Mark reviewed button for transactions that need review.

## Related

- [Reuse shared shadcn controls in feature forms](reuse-shadcn-controls-in-feature-forms.md)
