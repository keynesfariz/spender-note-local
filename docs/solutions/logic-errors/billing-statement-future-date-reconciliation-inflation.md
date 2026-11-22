---
title: Billing statement emails extracted with future due dates inflate reconciled credit card balances
date: 2026-11-22
category: logic-errors
module: Account Reconciliation & Gmail Sync
problem_type: logic_error
component: service_layer
related_components:
  - background_job
symptoms:
  - "Reconciling a credit card to its actual balance immediately snaps back to a higher balance on the next load (e.g., IDR 9,882,004 jumping to IDR 19,021,553)"
  - "Monthly credit card billing statement emails are imported as card_charge transactions with Unknown Merchant and an occurred_at_utc set to the future payment due date"
  - "Repeated reconciliations at the current timestamp record identical negative adjustments without fixing the displayed balance owed"
root_cause: logic_error
resolution_type: code_fix
severity: high
tags: [reconciliation, credit-card, billing-statement, future-date, llm-extraction, sqlite, go]
---

# Billing statement emails extracted with future due dates inflate reconciled credit card balances

## Problem
When a monthly credit card billing statement email is misclassified as a `card_charge` with its future payment due date as `occurred_at_utc`, reconciling the credit card account to its true balance immediately snaps back to the inflated balance on the next load.

## Symptoms
- Reconciling a credit card (e.g., `Smbci ••6039`) to `IDR 9,882,004` immediately displays `IDR 19,021,553` (`+IDR 9,139,549`), and reconciling again records `-IDR 9,139,549` repeatedly without changing the computed balance.
- A single `gmail` transaction row (`Unknown Merchant`, `Other`) has `occurred_at_utc` weeks in the future (the statement's `Due Date`, e.g., `2026-12-08`) while its `raw_emails` source is a monthly billing statement (`Billing Statement of Jenius Credit Card Platinum d-Card`).

## What Didn't Work
- **Relying solely on a single `non_transaction` prompt bullet in `lib/llm/types.ts`:** Small local GGUF models still classified a billing statement email containing `Total Billing (IDR): 9,139,549.26` and `Due Date: 08 Dec 2026` as `card_charge` when prompted only with `"bank statements... are non_transaction"`.
- **Re-reconciling the account at "now":** Because `computeAccountBalance` summed `WHERE account_id = ? AND occurred_at_utc > ?` with no upper time bound, any transaction dated after `now` stayed permanently above every new reconciliation anchor.

## Solution
Pending merge on branch `feat/credit-card-billing-cycle-reconciliation`:

1. **Bound live balance and net flow queries to `occurred_at_utc <= asOfUtc` and surface future-dated rows (`internal/domain/accounts.go`):**
   - Refactored `computeAccountBalance` to `computeAccountBalanceAsOf(gdb, account, targetCurrency, asOfUtc, nowUtc)` so both anchored (`occurred_at_utc > anchorTime AND occurred_at_utc <= asOfUtc`) and unanchored (`anchorBalance == nil`, `occurred_at_utc <= asOfUtc`) branches exclude future-dated transactions.
   - Added `future_transaction_count` (`WHERE account_id = ? AND occurred_at_utc > ?`) to `AccountSummary` and rendered an exclusion badge in `components/accounts-view.tsx` so mis-dated rows remain visible for manual cleanup.
   - Evaluated `ReconcileAccount` (`reconcileAccountAt`) as of `recorded_at_utc` and rejected future `recorded_at_utc > nowUtc`.

2. **Filter billing statement emails deterministically before LLM extraction (`internal/syncrunner/statement_filter.go`, `internal/syncrunner/runner.go`, `lib/llm/types.ts`):**
   - Added `IsBillingStatementEmail(subject, cleanedText)` to catch unmistakable statement subjects (`billing statement`, `e-statement`, `credit card statement`, `tagihan kartu kredit`, `lembar tagihan`, `ringkasan tagihan`) and paired statement-body cues (`total billing` / `total tagihan` / `statement balance` + `minimum payment` / `due date` / `tanggal jatuh tempo`).
   - In `runExtractionPhase`, matching emails are recorded as `non_transaction` (`extraction_model = 'rule:billing_statement'`) before calling `callNodeLlamaExtract`, and the `lib/llm/types.ts` prompt explicitly reinforces never classifying billing statements or their due dates as `card_charge`.

## Why This Works
- **Upper-bounded anchor queries prevent time-travel corruption:** An anchor-based ledger (`anchor + SUM(tx after anchor)`) only reflects the balance "as of T" when the sum is bounded by `occurred_at_utc <= T`. Without that upper bound, any row with `occurred_at_utc > T` escapes every reconciliation until wall-clock time catches up.
- **Deterministic pre-filtering removes prompt-compliance risk for known non-transactions:** Monthly billing statements have high-signal structural phrases and paired fields (`Total Billing` + `Minimum Payment` / `Due Date`). Filtering them in Go before invoking the local GGUF model prevents mis-extraction and saves inference time, while keeping legitimate receipts and card charge alerts untouched.

## Prevention
- **Regression tests:**
  - `internal/domain/accounts_test.go` (`TestFutureDatedTransactions_ExcludedFromBalanceAndCounted`, `TestCreditCardStatementReconciliation_PreservesUnbilledSpending`, `TestBackdatedReconciliation_PreservesNewerAnchor`) verifies that future-dated transactions are excluded from both confirmed and provisional account balances, counted in `future_transaction_count`, and that future reconciliation timestamps are rejected.
  - `internal/syncrunner/runner_test.go` (`TestIsBillingStatementEmail_DetectsStatementsWithoutFalsePositives`, `TestRunExtractionPhase_SkipsBillingStatementWithoutCallingLLM`) verifies billing statements are skipped without calling the LLM while legitimate card charges and receipts pass through.
- **Ledger query invariant:** Any balance or net-flow query anchored to a timestamp `T` must apply both a lower bound (when anchored) and an upper bound (`occurred_at_utc <= T`) across every branch, including unanchored/provisional accounts.

## Related Issues
- `docs/solutions/logic-errors/gmail-sync-sender-cursor-and-timestamp-null.md` — sibling Gmail sync runner learning in `internal/syncrunner/runner.go`.
