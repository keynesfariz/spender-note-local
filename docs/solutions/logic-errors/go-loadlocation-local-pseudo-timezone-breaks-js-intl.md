---
title: Go time.LoadLocation accepts Local pseudo-timezone that throws RangeError in JS Intl.DateTimeFormat
date: 2026-11-23
category: logic-errors
module: Country Configuration & Timezone Validation
problem_type: logic_error
component: service_layer
related_components:
  - frontend
symptoms:
  - "Country configuration startup validation accepts `\"Local\"` as a valid timezone when delegating solely to Go's `time.LoadLocation`"
  - "Passing `\"Local\"` to JavaScript `Intl.DateTimeFormat` throws a `RangeError` (unguarded in `lib/format.ts` `dateTimeInputToUtcIso` -> `getTimeZoneOffsetMs`, and caught as `null` in `lib/llm/country-rules.ts` `resolveTimezoneOffsetString`)"
root_cause: wrong_api
resolution_type: code_fix
severity: medium
tags: [go, timezone, iana, intl-datetimeformat, cross-runtime, validation]
---

# Go time.LoadLocation accepts Local pseudo-timezone that throws RangeError in JS Intl.DateTimeFormat

## Problem

Go's `time.LoadLocation` (wrapped by `LoadTimezone` in `internal/domain/time.go`) succeeds for the Go-specific pseudo-timezone `"Local"`, even though `"Local"` is not an IANA timezone identifier. When a timezone validated in Go is persisted or exposed to the Next.js / TypeScript runtime, `Intl.DateTimeFormat` rejects `"Local"` with a `RangeError`.

## Symptoms

- Validating `defaultTimezone` or `timezones` entries with `LoadTimezone` alone allowed `"Local"` to pass startup validation in `internal/domain/countries.go`.
- In TypeScript, `getTimeZoneOffsetMs` (`lib/format.ts`) passes the configured timezone directly to `Intl.DateTimeFormat` without a `try/catch`, so `dateTimeInputToUtcIso` throws an unhandled `RangeError` for `"Local"`.
- In `lib/llm/country-rules.ts`, `resolveTimezoneOffsetString` catches the `RangeError` and returns `null`, silently dropping the UTC offset hint from the LLM extraction prompt.

## What Didn't Work

- **Relying on `LoadTimezone` (`time.LoadLocation`) alone to validate IANA timezones:** Go treats `"Local"` (and `""`, which `LoadTimezone` maps to `time.UTC`) as valid special names rather than strictly checking the IANA tzdata database.

## Solution

1. Explicitly reject `"Local"` (case-insensitive via `strings.EqualFold(tz, "Local")`) alongside empty-string checks before calling `LoadTimezone` in `validateCountryPack` (`internal/domain/countries.go`):

```go
if strings.EqualFold(tz, "Local") {
	return fmt.Errorf("%s: field \"timezones[%d]\": \"Local\" is not a valid IANA timezone", fullPath, i)
}
if _, err := LoadTimezone(tz); err != nil {
	return fmt.Errorf("%s: field \"timezones[%d]\": %w", fullPath, i, err)
}
```

2. Added a regression test case (`Local pseudo-timezone rejected`) in `TestLoadCountryPacksFromDir_FailsFastOnInvalidFiles` (`internal/domain/countries_test.go`) asserting that `"Local"` fails fast at startup with `"not a valid IANA timezone"`.

## Why This Works

Go's `time.LoadLocation` documents `"Local"` and `""` as special built-in names that bypass the IANA timezone database lookup. JavaScript's `Intl.DateTimeFormat` (ECMA-402) only accepts canonical IANA timezone identifiers (and `UTC`), so any Go validation boundary that feeds a timezone to the TypeScript frontend or LLM layer must reject `"Local"` explicitly.

## Prevention

- Whenever a Go endpoint or config loader validates an IANA timezone that will be stored in SQLite or consumed by TypeScript `Intl.DateTimeFormat`, explicitly reject `"Local"` (case-insensitive) in addition to calling `LoadTimezone` / `time.LoadLocation`.
- **Recommended follow-up (owner consolidation):** Move the `strings.EqualFold(tz, "Local")` rejection directly into `LoadTimezone` in `internal/domain/time.go` so callers like `UpdateLocaleSettings` (`internal/domain/settings.go`) also reject `"Local"` automatically.
- Keep `TestLoadCountryPacksFromDir_FailsFastOnInvalidFiles/Local_pseudo-timezone_rejected` (`internal/domain/countries_test.go`) covering the startup boundary.

## Related Issues

- `docs/plans/2026-11-22-2346-refactor-single-file-country-packs-plan.md` — single-file country configuration plan where this cross-runtime validation gap was caught during code review.
