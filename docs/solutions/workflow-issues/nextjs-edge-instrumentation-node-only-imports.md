---
title: Load Node.js setup conditionally from Next.js instrumentation
date: 2026-11-24
category: workflow-issues
module: Next.js instrumentation and goRoutes
problem_type: workflow_issue
component: development_workflow
severity: low
applies_when:
  - Adding Node.js-only APIs or dependencies to the root Next.js instrumentation hook
  - Diagnosing Edge Runtime warnings about process.stdout during a Turbopack build
tags: [nextjs, instrumentation, edge-runtime, turbopack, nextjs-goroutes]
---

# Load Node.js setup conditionally from Next.js instrumentation

## Context

Next.js `experimental.goRoutes` pipes Go route workers' output into `process.stdout`. The server startup hook raises the stream's listener limit to avoid `MaxListenersExceededWarning` as workers start. With Next.js 16.5.0-canary.61, `bun run build` succeeded but reported two Edge Runtime warnings for `process.stdout` in the shared `instrumentation.ts`.

## Guidance

Keep Node.js API calls in a separate module. In [`instrumentation.ts`](../../../instrumentation.ts), await a dynamic import inside the `process.env.NEXT_RUNTIME === "nodejs"` branch of `register()`. This lets the startup hook finish the setup before Next.js serves requests. [Next.js guide to runtime-specific imports](https://nextjs.org/docs/app/guides/instrumentation#importing-runtime-specific-code)

## Why This Matters

The original early return kept Edge from executing the two `process.stdout` calls. Turbopack still flagged both references when it analyzed the shared file for Edge. The conditional import moves the Node.js-only references out of that file and gives the compiler an import it can exclude for Edge.

## When to Apply

Use this pattern when shared server instrumentation needs a Node.js API or dependency. Run `bun run build` and inspect the output for Edge Runtime warnings. A successful exit can still include those diagnostics.

## Example

```ts
export async function register() {
  if (process.env.NEXT_RUNTIME === "nodejs") {
    const { configureGoRouteStdout } = await import("./instrumentation-node")
    configureGoRouteStdout()
  }
}
```

[`configureGoRouteStdout()`](../../../instrumentation-node.ts) raises a finite limit below 64 to 64 and preserves higher limits and the unlimited setting of 0. The production build emitted neither stdout warning after the split. A separate runtime check verified that Edge leaves the limit alone and that Node.js preserves the listener-limit behavior.

## Related

- [Go route build and test workflow](nextjs-goroutes-bracket-dirs-break-go-test-and-vet.md)
