---
title: Next.js experimental.goRoutes dynamic [id] directories break root go test ./... and multi-file go vet
date: 2026-11-23
category: workflow-issues
module: Next.js goRoutes & Go Toolchain
problem_type: workflow_issue
component: development_workflow
severity: medium
applies_when:
  - "Running Go tests or static analysis in a Next.js repository configured with `experimental: { goRoutes: true }`"
  - "Adding or modifying Go API route handlers under dynamic App Router segments such as `app/api/categories/[id]/route.go` or `app/api/accounts/[id]/route.go`"
tags: [nextjs-goroutes, go-test, go-vet, dynamic-routes, app-router, tooling]
---

# Next.js experimental.goRoutes dynamic [id] directories break root go test ./... and multi-file go vet

## Context

In PortSpenderNote, `next.config.ts` enables `experimental: { goRoutes: true }`, with Go API route handlers placed inside Next.js App Router directories (`app/api/**/route.go`), including dynamic `[id]` segment directories (`app/api/accounts/[id]/route.go`, `app/api/accounts/[id]/confirm/route.go`, `app/api/accounts/[id]/reconciliations/route.go`, `app/api/categories/[id]/route.go`, `app/api/senders/[id]/route.go`, `app/api/tags/[id]/route.go`, `app/api/transactions/[id]/route.go`, and `app/api/transactions/[id]/split/route.go`).

When verifying Go changes across the repository, two standard Go CLI invocation patterns fail:

1. **Root wildcard expansion (`go test ./...` or `go vet ./...`):**
   ```text
   malformed import path "portspendernote/app/api/accounts/[id]": invalid char '['
   FAIL	portspendernote/app/api/accounts/[id] [setup failed]
   ```
   The Go toolchain (`cmd/go`) expands `./...` across every directory containing `.go` files (including `app/api/` and vendored `node_modules/` packages) and validates each directory's module-relative import path. Because square brackets (`[` and `]`) are illegal in Go import paths, every bracketed route directory fails package setup (`[setup failed]`) and causes the overall command to fail.

2. **Multi-directory file list (`go vet app/api/categories/route.go 'app/api/categories/[id]/route.go'`):**
   ```text
   named files must all be in one directory; have app/api/categories and app/api/categories/[id]
   ```
   Passing `.go` file paths to `go vet` bypasses import-path validation, but `go vet` treats all positional `.go` file arguments in a single invocation as belonging to one Go package in one directory. (Passing a bracketed directory path like `./app/api/categories/[id]` fails with the same `invalid char '['` import-path error.)

## Guidance

1. **Scope `./...` to standard Go package trees (`internal/` and `cmd/`):**
   Run unit and integration tests against the non-App-Router Go package trees explicitly:
   ```bash
   go test -count=1 ./internal/... ./cmd/...
   ```

2. **Run `go vet` one route file per invocation:**
   Pass one `route.go` file path per `go vet` call across `app/api/`:
   ```bash
   while IFS= read -r f; do
     go vet "$f" || exit 1
   done < <(find app/api -name 'route.go' | sort)
   ```

3. **Run `bun run build` as the full `goRoutes` compilation gate:**
   Run `bun run build` (`bun --bun next build` in `package.json`) to compile all Go API route handlers alongside TypeScript and Next.js App Router pages.

## Why This Matters

Every `route.go` handler under `app/api/` declares `package main` and serves as its own standalone entry point for Next.js `goRoutes`, rather than a shared Go library package. Because `go.mod` (`module portspendernote`) sits at the repository root, `cmd/go` discovers all `app/api/**/[id]/**/route.go` directories whenever `./...` is expanded from root. Scoping `go test` to `./internal/... ./cmd/...` and vetting each `route.go` file individually avoids bracketed-import-path failures while covering every package and handler.

## When to Apply

- Running Go tests locally or in automated verification loops after editing `internal/**`, `cmd/**`, or `app/api/**/route.go`.
- Adding new dynamic Next.js Go API routes under `app/api/**/[param]/route.go`.
- Writing verification instructions in implementation plans or subagent prompts so workers do not run bare `go test ./...`.

## Examples

### Before (fails during `cmd/go` package resolution)

```bash
# Fails on bracketed [id] directories: malformed import path "...": invalid char '['
go test ./...

# Fails with: named files must all be in one directory
go vet app/api/categories/route.go 'app/api/categories/[id]/route.go'
```

### After (passes all Go package tests, all route checks, and Next.js build)

```bash
while IFS= read -r f; do
  go vet "$f" || exit 1
done < <(find app/api -name 'route.go' | sort) && go test -count=1 ./internal/... ./cmd/... && bun run build
```

## Related

- `next.config.ts` (`experimental: { goRoutes: true }`)
- `docs/solutions/database-issues/sqlite-table-rebuild-begin-immediate-goroutes.md` — related Next.js `goRoutes` multi-process execution behavior on startup
