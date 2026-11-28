---
title: Keep the same color roles in light and dark mode
date: 2026-11-24
category: design-patterns
module: UI Components
problem_type: design_pattern
component: frontend
severity: low
applies_when:
  - Changing the app palette or adding a color cue to a shared UI component
tags: [theming, dark-mode, color-tokens, contrast]
---

# Keep the same color roles in light and dark mode

## Context

The initial warm dark palette used espresso surfaces and cream primary buttons. Even with readable text, switching from light mode made Spender feel like a different theme. The accepted revision used warm charcoal and off-white for the base, with the same accents carrying the same meaning in both modes.

## Guidance

Define shared accent colors in [the global stylesheet](../../../app/globals.css) and let each theme set their intensity through semantic tokens. Cream marks the spending summary and selection, mint identifies wallets, and peach identifies credit cards. In dark mode, mix each accent into the card surface to keep the hue visible without making the surface bright. Keep primary actions near-neutral in both modes and reserve blue for links and focus.

For example, the monthly summary uses `bg-summary` in [MonthlySummary](../../../components/dashboard/monthly-summary.tsx). The stylesheet maps `--summary` to `--secondary` in both modes. That token uses cream directly in light mode and a cream tint over the dark card surface in dark mode. Account icons follow the same approach through `bg-wallet` and `bg-credit` in [AccountsView](../../../components/accounts/accounts-view.tsx).

## When to apply

When changing a shared color, compare the same screen in both modes. Check whether accents still draw attention to the same controls and information, then measure contrast against the backgrounds where those colors actually appear. Contrast checks alone did not catch the mismatch in the earlier palette.
