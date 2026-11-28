---
title: Base UI Button render prop breaks link semantics and logs nativeButton error
date: 2026-11-21
category: ui-bugs
module: UI Components
problem_type: ui_bug
component: frontend
symptoms:
  - "Base UI: A component that acts as a button expected a native <button> because the `nativeButton` prop is true."
  - "Setting `nativeButton={false}` on `<Button render={<Link />} />` overrides semantic link roles by applying `role=\"button\"` to the rendered `<a>` element."
root_cause: wrong_api
resolution_type: code_fix
severity: medium
framework_version: "@base-ui/react 1.9.0 / next 16.5.0-canary.59"
tags: [base-ui, shadcn, button-variants, accessibility, next-link]
---

# Base UI Button render prop breaks link semantics and logs nativeButton error

## Problem
Rendering navigation links via `<Button render={<Link href="..." />} />` or `<Button render={<a href="..." />} />` triggers a dev-mode Base UI console error in Next.js, and following Base UI's error message by adding `nativeButton={false}` silently applies `role="button"` onto `<a>` elements, breaking native link accessibility semantics.

## Symptoms
- Dev console error on mount in `AppShell` (and any view rendering a link via `<Button render={...}>`):
  `Base UI: A component that acts as a button expected a native <button> because the \`nativeButton\` prop is true. Rendering a non-<button> removes native button semantics, which can impact forms and accessibility. Use a real <button> in the \`render\` prop, or set \`nativeButton\` to \`false\`.`
- If silenced with `nativeButton={false}`, rendered `<a>` elements receive `role="button"` instead of preserving native link semantics.

## What Didn't Work
- **Following Base UI's error message (`nativeButton={false}`):** Adding `nativeButton={false}` to `<Button render={<Link ... />} />` silences the console error, but `useButton` in `@base-ui/react` unconditionally merges `{ role: 'button' }` when `isNativeButton` is `false`, overriding the `<a>` element's native link role.

## Solution
Use the `buttonVariants` helper exported by `components/ui/button.tsx` directly on `<Link>` or `<a>` instead of wrapping links in `<Button render={...}>`.

**Before:**
```tsx
import { Button } from '@/components/ui/button';

<Button
  key={item.href}
  variant={active ? 'secondary' : 'ghost'}
  size="sm"
  render={<Link href={item.href} />}
>
  {item.label}
</Button>
```

**After:**
```tsx
import { Button, buttonVariants } from '@/components/ui/button';

<Link
  key={item.href}
  href={item.href}
  className={buttonVariants({
    variant: active ? 'secondary' : 'ghost',
    size: 'sm',
  })}
>
  {item.label}
</Link>
```

The same pattern applies to plain `<a>` elements (such as full-page OAuth redirects to `/api/gmail/auth`):
```tsx
<a href="/api/gmail/auth" className={buttonVariants({ size: 'sm' })}>
  Reconnect Gmail
</a>
```

Updated all 6 affected link-button call sites across:
- `components/app-shell.tsx` (`AppShell` primary nav links and expired-Gmail reconnect link)
- `components/accounts/accounts-view.tsx` (`AccountsView` setup link to `/settings`)
- `components/dashboard/dashboard-view.tsx` (`DashboardView` setup link to `/settings`)
- `components/settings/gmail-connection-card.tsx` (`GmailConnectionCard` Gmail OAuth connect/reconnect link)
- `components/sync/sync-toast.tsx` (`SyncToast` link to `/settings`)

## Why This Works
- `Button` in `components/ui/button.tsx` wraps `ButtonPrimitive` from `@base-ui/react/button`.
- `Button` in `@base-ui/react` (`node_modules/@base-ui/react/button/Button.mjs`) defaults `nativeButton` to `true` and forwards `native: nativeButton` to `useButton`.
- `useButton` (`node_modules/@base-ui/react/internals/use-button/useButton.mjs`) runs a dev-only `useEffect` that errors when `isNativeButton` is `true` and `elementRef.current.tagName !== 'BUTTON'`. When `isNativeButton` is `false`, `getButtonProps` merges `{ role: 'button' }` onto the element.
- Styling `<Link>` or `<a>` directly with `buttonVariants({ variant, size })` bypasses `ButtonPrimitive` and `useButton` entirely: it applies the exact same visual button classes while keeping native `<a>` link semantics and avoiding Base UI's dev-mode error.
- Reserve `<Button render={...}>` (or trigger `render={<Button ... />}`) strictly for components that actually render a native `<button>` (such as `DialogClose`, `AlertDialogTrigger`, and `CollapsibleTrigger`).

## Prevention
- **Standing rule for button-styled links:** Never pass `<Link>` or `<a>` to `<Button render={...}>` (with or without `nativeButton={false}`). Always render `<Link>` or `<a>` directly with `className={buttonVariants({ variant, size })}`.
- **Recommended automated check:** Once the repo's ESLint setup supports the installed TypeScript version (or once a frontend component test runner is added), add either an ESLint `no-restricted-syntax` rule rejecting `Button` JSX elements whose `render` prop contains `<Link>` or `<a>`, or a component smoke test mounting `AppShell` and asserting zero `console.error` calls and `role="link"` on nav items.

## Related Issues
- None.
