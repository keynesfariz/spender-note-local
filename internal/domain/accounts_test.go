package domain

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"portspendernote/internal/db"
)

func intPtr(v int) *int       { return &v }
func strPtr(v string) *string { return &v }

func configureTestLocale(t *testing.T, gdb *gorm.DB, tz, currency string) {
	t.Helper()
	if err := gdb.Model(&db.AppSetting{}).Where("id = 1").Updates(map[string]any{
		"country_code": "ID",
		"timezone":     tz,
		"currency":     currency,
	}).Error; err != nil {
		t.Fatalf("configure test locale: %v", err)
	}
}

func TestListAccountOptions_PreservesSummaryOrderAndCurrentNames(t *testing.T) {
	gdb := openDomainTestDB(t)
	options, err := ListAccountOptions(gdb)
	if err != nil || options == nil || len(options) != 0 {
		t.Fatalf("empty options = %#v, %v; want an empty slice", options, err)
	}

	firstID := seedTestAccount(t, gdb, "first", "Second alphabetically", "IDR")
	secondID := seedTestAccount(t, gdb, "second", "First alphabetically", "IDR")
	newestID := seedTestAccount(t, gdb, "newest", "A more recent card", "IDR")
	if err := gdb.Model(&db.Account{}).Where("id = ?", newestID).
		Update("created_at_utc", "2026-02-01T00:00:00Z").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateAccount(gdb, firstID, UpdateAccountInput{Name: strPtr("Renamed card")}); err != nil {
		t.Fatal(err)
	}

	options, err = ListAccountOptions(gdb)
	if err != nil {
		t.Fatal(err)
	}
	want := []AccountOption{
		{ID: secondID, Name: "First alphabetically"},
		{ID: firstID, Name: "Renamed card"},
		{ID: newestID, Name: "A more recent card"},
	}
	if !reflect.DeepEqual(options, want) {
		t.Fatalf("options = %#v, want %#v", options, want)
	}
}

func TestAccountStatementDay_CRUDAndValidation(t *testing.T) {
	gdb := openDomainTestDB(t)
	configureTestLocale(t, gdb, "Asia/Jakarta", "IDR")

	// Create credit card with statement day 22
	cc, err := CreateAccount(gdb, CreateAccountInput{
		Name:                "Jenius CC",
		Type:                "credit_card",
		InitialBalanceMinor: 0,
		StatementDay:        intPtr(22),
	})
	if err != nil {
		t.Fatalf("create credit card with statement_day=22: %v", err)
	}
	if cc.StatementDay == nil || *cc.StatementDay != 22 {
		t.Fatalf("expected statement_day=22, got %v", cc.StatementDay)
	}

	// Create credit card without statement day
	ccNil, err := CreateAccount(gdb, CreateAccountInput{
		Name:                "No Cycle CC",
		Type:                "credit_card",
		InitialBalanceMinor: 0,
	})
	if err != nil {
		t.Fatalf("create credit card without statement_day: %v", err)
	}
	if ccNil.StatementDay != nil {
		t.Fatalf("expected nil statement_day, got %v", *ccNil.StatementDay)
	}

	// Update: set to 15
	updated, err := UpdateAccount(gdb, cc.ID, UpdateAccountInput{
		HasStatementDay: true,
		StatementDay:    intPtr(15),
	})
	if err != nil {
		t.Fatalf("update statement_day=15: %v", err)
	}
	if updated.StatementDay == nil || *updated.StatementDay != 15 {
		t.Fatalf("expected statement_day=15, got %v", updated.StatementDay)
	}

	// Update: omit HasStatementDay keeps 15 intact
	renamed, err := UpdateAccount(gdb, cc.ID, UpdateAccountInput{
		Name: strPtr("Jenius Platinum"),
	})
	if err != nil {
		t.Fatalf("rename account without statement_day patch: %v", err)
	}
	if renamed.StatementDay == nil || *renamed.StatementDay != 15 {
		t.Fatalf("expected statement_day=15 to remain unchanged, got %v", renamed.StatementDay)
	}

	// Update: explicitly clear to nil
	cleared, err := UpdateAccount(gdb, cc.ID, UpdateAccountInput{
		HasStatementDay: true,
		StatementDay:    nil,
	})
	if err != nil {
		t.Fatalf("clear statement_day: %v", err)
	}
	if cleared.StatementDay != nil {
		t.Fatalf("expected cleared statement_day=nil, got %v", *cleared.StatementDay)
	}

	// Validation: reject 0 and 32
	for _, bad := range []int{0, -1, 32} {
		if _, err := CreateAccount(gdb, CreateAccountInput{
			Name:                "Bad CC",
			Type:                "credit_card",
			InitialBalanceMinor: 0,
			StatementDay:        intPtr(bad),
		}); err == nil {
			t.Fatalf("expected error for statement_day=%d", bad)
		}
	}

	// Validation: reject non-credit-card with statement_day
	for _, acctType := range []string{"debit_card", "digital_wallet"} {
		if _, err := CreateAccount(gdb, CreateAccountInput{
			Name:                "Non CC",
			Type:                acctType,
			InitialBalanceMinor: 0,
			StatementDay:        intPtr(22),
		}); err == nil {
			t.Fatalf("expected error for %s with statement_day", acctType)
		}
	}
}

func TestListAccountsAt_MatchesSingleAccountSummary(t *testing.T) {
	gdb, err := db.OpenDatabase(":memory:", false)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	configureTestLocale(t, gdb, "Asia/Jakarta", "IDR")

	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	asOf := "2026-10-01T00:00:00Z"

	cc, err := CreateAccount(gdb, CreateAccountInput{
		Name:                  "BCA CC",
		Type:                  "credit_card",
		InitialBalanceMinor:   100000,
		InitialBalanceAsOfUtc: &asOf,
		StatementDay:          intPtr(20),
	})
	if err != nil {
		t.Fatalf("create cc: %v", err)
	}
	debit, err := CreateAccount(gdb, CreateAccountInput{
		Name:                  "Mandiri Debit",
		Type:                  "debit_card",
		InitialBalanceMinor:   500000,
		InitialBalanceAsOfUtc: &asOf,
	})
	if err != nil {
		t.Fatalf("create debit: %v", err)
	}
	_, err = CreateAccount(gdb, CreateAccountInput{
		Name:                  "Empty Wallet",
		Type:                  "digital_wallet",
		InitialBalanceMinor:   50000,
		InitialBalanceAsOfUtc: &asOf,
	})
	if err != nil {
		t.Fatalf("create wallet: %v", err)
	}

	seedAccountTx(t, gdb, "tx-cc-last", cc.ID, 40000, "IDR", "2026-11-10T05:00:00Z")
	seedAccountTx(t, gdb, "tx-cc-cur", cc.ID, 25000, "IDR", "2026-11-22T05:00:00Z")
	seedAccountTx(t, gdb, "tx-cc-fx", cc.ID, 1000, "USD", "2026-11-22T06:00:00Z")
	seedAccountTx(t, gdb, "tx-cc-fut", cc.ID, 15000, "IDR", "2026-11-25T05:00:00Z")
	seedAccountTx(t, gdb, "tx-deb-1", debit.ID, 75000, "IDR", "2026-11-15T05:00:00Z")

	batched, err := listAccountsAt(gdb, now)
	if err != nil {
		t.Fatalf("listAccountsAt: %v", err)
	}
	if len(batched) != 3 {
		t.Fatalf("expected 3 accounts, got %d", len(batched))
	}
	for _, got := range batched {
		single, err := buildAccountSummaryAt(gdb, got.Account, "Asia/Jakarta", got.Identifiers, now)
		if err != nil {
			t.Fatalf("buildAccountSummaryAt(%s): %v", got.Name, err)
		}
		if !reflect.DeepEqual(got, single) {
			t.Fatalf("account %s mismatch:\nbatched=%+v\nsingle =%+v", got.Name, got, single)
		}
	}
}

func TestForeignAccountUsesItsCurrencyForBalancesBillingAndReconciliation(t *testing.T) {
	gdb := openDomainTestDB(t)
	configureTestLocale(t, gdb, "Asia/Jakarta", "IDR")
	accountID := seedTestAccount(t, gdb, "foreign-card", "USD card", "USD")
	if err := gdb.Model(&db.Account{}).Where("id = ?", accountID).Updates(map[string]any{
		"initial_balance_minor":     10000,
		"initial_balance_as_of_utc": "2026-11-01T00:00:00Z",
		"statement_day":             20,
	}).Error; err != nil {
		t.Fatal(err)
	}
	seedAccountTx(t, gdb, "usd-charge", accountID, 1250, "USD", "2026-11-22T05:00:00Z")
	seedAccountTx(t, gdb, "idr-charge", accountID, 20000, "IDR", "2026-11-22T06:00:00Z")
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

	accounts, err := listAccountsAt(gdb, now)
	if err != nil {
		t.Fatal(err)
	}
	account := accounts[0]
	if account.ComputedBalanceMinor == nil || *account.ComputedBalanceMinor != 11250 ||
		account.NetFlowMinor != 1250 || account.UncountedForeignCount != 1 {
		t.Fatalf("unexpected USD account totals: %+v", account)
	}
	if account.BillingCycle == nil || account.BillingCycle.CurrentCycle.SpentMinor != 1250 ||
		account.BillingCycle.CurrentCycle.TransactionCount != 1 {
		t.Fatalf("unexpected USD billing cycle: %+v", account.BillingCycle)
	}

	result, err := reconcileAccountAt(gdb, ReconcileAccountInput{
		AccountID: accountID, ActualBalanceMinor: 11000,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	rec := result["reconciliation"].(db.AccountReconciliation)
	if rec.Currency != "USD" || rec.ComputedBalanceMinor != 11250 ||
		rec.ActualBalanceMinor != 11000 || rec.AdjustmentMinor != -250 {
		t.Fatalf("unexpected USD reconciliation: %+v", rec)
	}
	summary := result["account"].(AccountSummary)
	if summary.ComputedBalanceMinor == nil || *summary.ComputedBalanceMinor != 11000 ||
		summary.BillingCycle == nil || summary.BillingCycle.CurrentCycle.SpentMinor != 1250 {
		t.Fatalf("unexpected USD reconciled account: %+v", summary)
	}
}

func seedAccountTx(t *testing.T, gdb *gorm.DB, id, accountID string, amountMinor int64, currency, occurredAtUtc string) {
	t.Helper()
	tx := db.Transaction{
		ID:            id,
		SourceType:    "manual",
		AccountID:     &accountID,
		AmountMinor:   amountMinor,
		Currency:      currency,
		OccurredAtUtc: occurredAtUtc,
		Merchant:      "Test Merchant",
		CategoryID:    "cat-other",
		Reviewed:      1,
		CreatedAtUtc:  "2026-11-01T00:00:00Z",
		UpdatedAtUtc:  "2026-11-01T00:00:00Z",
	}
	if err := gdb.Create(&tx).Error; err != nil {
		t.Fatalf("seed tx %s: %v", id, err)
	}
}

func TestAccountMovements_TransfersAndIncomeRespectEachPostingAndAnchor(t *testing.T) {
	gdb := openDomainTestDB(t)
	configureTestLocale(t, gdb, "UTC", "IDR")
	bank := seedTestAccount(t, gdb, "bank", "Bank", "IDR")
	wallet := seedTestAccount(t, gdb, "wallet", "Wallet", "IDR")
	card := seedTestAccount(t, gdb, "card", "Card", "USD")
	initial := "2026-11-01T00:00:00Z"
	for _, account := range []struct {
		id, kind string
		initial  int64
	}{{bank, "debit_card", 1000}, {wallet, "digital_wallet", 0}, {card, "credit_card", 500}} {
		if err := gdb.Model(&db.Account{}).Where("id = ?", account.id).Updates(map[string]any{
			"type": account.kind, "initial_balance_minor": account.initial, "initial_balance_as_of_utc": initial,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := gdb.Model(&db.Account{}).Where("id = ?", card).Update("statement_day", 20).Error; err != nil {
		t.Fatal(err)
	}

	noon := "2026-11-23T12:00:00Z"
	now := time.Date(2026, 11, 23, 16, 0, 0, 0, time.UTC)
	for _, anchor := range []struct {
		id     string
		actual int64
	}{{bank, 900}, {card, 500}} {
		if _, err := reconcileAccountAt(gdb, ReconcileAccountInput{
			AccountID: anchor.id, ActualBalanceMinor: anchor.actual, RecordedAtUtc: &noon,
		}, now); err != nil {
			t.Fatal(err)
		}
	}
	seedAccountMovement(t, gdb, "payment", bank, "IDR", 100, noon, map[string]any{
		"kind": "transfer", "destination_account_id": card, "destination_amount_minor": 30,
		"destination_currency": "USD", "source_posted_at_utc": "2026-11-23T09:00:00Z",
		"destination_posted_at_utc": "2026-11-23T15:00:00Z",
	})
	seedAccountMovement(t, gdb, "same-as-reconciliation", bank, "IDR", 1, noon, map[string]any{
		"kind": "transfer", "destination_account_id": card, "destination_amount_minor": 1, "destination_currency": "USD",
	})
	seedAccountMovement(t, gdb, "card-top-up", card, "USD", 10, "2026-11-23T14:00:00Z", map[string]any{
		"kind": "transfer", "destination_account_id": wallet, "destination_amount_minor": 50, "destination_currency": "IDR",
	})
	seedAccountMovement(t, gdb, "future", wallet, "IDR", 20, "2026-11-23T18:00:00Z", map[string]any{
		"kind": "transfer", "destination_account_id": bank, "destination_amount_minor": 20, "destination_currency": "IDR",
	})
	seedAccountMovement(t, gdb, "bank-income", bank, "IDR", 40, "2026-11-23T13:00:00Z", map[string]any{"kind": "income"})
	seedAccountMovement(t, gdb, "card-income", card, "USD", 5, "2026-11-23T13:00:00Z", map[string]any{"kind": "income"})
	seedAccountTx(t, gdb, "card-expense", card, 7, "USD", "2026-11-23T13:00:00Z")
	seedAccountTx(t, gdb, "wallet-expense", wallet, 10, "IDR", initial)

	want := map[string]struct {
		balance, flow, future int64
	}{
		bank: {940, 40, 1}, wallet: {40, 40, 1}, card: {482, -18, 0},
	}
	list, err := listAccountsAt(gdb, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range list {
		single, err := buildAccountSummaryAt(gdb, account.Account, "UTC", account.Identifiers, now)
		if err != nil || !reflect.DeepEqual(account, single) {
			t.Fatalf("account %s: batched = %+v, single = %+v, err = %v", account.ID, account, single, err)
		}
		expected, ok := want[account.ID]
		if !ok {
			t.Fatalf("unexpected or duplicate account %s", account.ID)
		}
		delete(want, account.ID)
		if account.ComputedBalanceMinor == nil || *account.ComputedBalanceMinor != expected.balance ||
			account.NetFlowMinor != expected.flow || account.UncountedForeignCount != 0 ||
			account.FutureTransactionCount != expected.future {
			t.Fatalf("account %s: got %+v, want %+v", account.ID, account, expected)
		}
		if account.ID == card && (account.BillingCycle == nil || account.BillingCycle.CurrentCycle.SpentMinor != 7 ||
			account.BillingCycle.CurrentCycle.TransactionCount != 1) {
			t.Fatalf("card billing cycle includes a non-expense: %+v", account.BillingCycle)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing expected accounts: %v", want)
	}
}

func TestAccountMovements_ProtectHistoryOnDeleteAndMerge(t *testing.T) {
	gdb := openDomainTestDB(t)
	configureTestLocale(t, gdb, "UTC", "IDR")
	source := seedTestAccount(t, gdb, "source", "Source", "IDR")
	destination := seedTestAccount(t, gdb, "destination", "Destination", "IDR")
	target := seedTestAccount(t, gdb, "target", "Merge target", "IDR")
	income := seedTestAccount(t, gdb, "income", "Income", "IDR")
	if err := gdb.Model(&db.Account{}).Where("id = ?", destination).Update("status", "provisional").Error; err != nil {
		t.Fatal(err)
	}
	at := "2026-11-23T12:00:00Z"
	seedAccountMovement(t, gdb, "linked-transfer", source, "IDR", 50, at, map[string]any{
		"kind": "transfer", "destination_account_id": destination, "destination_amount_minor": 50, "destination_currency": "IDR",
	})
	seedAccountMovement(t, gdb, "linked-income", income, "IDR", 50, at, map[string]any{"kind": "income"})
	for _, id := range []string{source, destination, income} {
		if err := DeleteAccount(gdb, id); err == nil || !strings.Contains(err.Error(), "This account still has income or transfers.") {
			t.Fatalf("deletion of account %s should explain its movement history, got %v", id, err)
		}
	}
	nextType := "debit_card"
	if _, err := UpdateAccount(gdb, destination, UpdateAccountInput{Type: &nextType}); err == nil {
		t.Fatal("changed type on provisional transfer destination")
	}
	if _, err := MergeAccounts(gdb, source, destination); err == nil || !strings.Contains(err.Error(), "linked-transfer") {
		t.Fatalf("expected merge rejection naming the collapsed transfer, got %v", err)
	}
	var result struct {
		AccountID            string
		DestinationAccountID string
	}
	if err := gdb.Table("transactions").Where("id = ?", "linked-transfer").
		Select("account_id, destination_account_id").Take(&result).Error; err != nil {
		t.Fatal(err)
	}
	if result.AccountID != source || result.DestinationAccountID != destination {
		t.Fatalf("rejected merge modified transfer: %+v", result)
	}
	merged, err := MergeAccounts(gdb, destination, target)
	if err != nil {
		t.Fatal(err)
	}
	if merged["movedTransactions"] != int64(1) {
		t.Fatalf("destination move was not counted: %v", merged["movedTransactions"])
	}
	if err := gdb.Table("transactions").Where("id = ?", "linked-transfer").
		Select("account_id, destination_account_id").Take(&result).Error; err != nil {
		t.Fatal(err)
	}
	if result.AccountID != source || result.DestinationAccountID != target {
		t.Fatalf("merge failed to reassign transfer destination: %+v", result)
	}
}

func seedAccountMovement(t *testing.T, gdb *gorm.DB, id, accountID, currency string, amountMinor int64, occurredAtUtc string, fields map[string]any) {
	t.Helper()
	seedAccountTx(t, gdb, id, accountID, amountMinor, currency, occurredAtUtc)
	if err := gdb.Model(&db.Transaction{}).Where("id = ?", id).Updates(fields).Error; err != nil {
		t.Fatalf("seed movement %s: %v", id, err)
	}
}

func TestComputeCreditCardCycleBounds_ClampingAndBoundaries(t *testing.T) {
	// Non-leap February 2026 with statement_day=31 clamps to Feb 28
	nowNonLeap := time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC)
	bNonLeap, err := ComputeCreditCardCycleBounds(31, "Asia/Jakarta", nowNonLeap)
	if err != nil {
		t.Fatalf("ComputeCreditCardCycleBounds non-leap: %v", err)
	}
	if bNonLeap.LastStatementDate != "2026-02-28" {
		t.Fatalf("expected LastStatementDate=2026-02-28, got %s", bNonLeap.LastStatementDate)
	}
	if bNonLeap.CurrentCycle.StartDate != "2026-03-01" || bNonLeap.CurrentCycle.EndDate != "2026-03-31" {
		t.Fatalf("unexpected non-leap current cycle: %+v", bNonLeap.CurrentCycle)
	}
	if bNonLeap.LastCycle.StartDate != "2026-02-01" || bNonLeap.LastCycle.EndDate != "2026-02-28" {
		t.Fatalf("unexpected non-leap last cycle: %+v", bNonLeap.LastCycle)
	}

	// Leap February 2028 with statement_day=31 clamps to Feb 29
	nowLeap := time.Date(2028, 3, 5, 10, 0, 0, 0, time.UTC)
	bLeap, err := ComputeCreditCardCycleBounds(31, "Asia/Jakarta", nowLeap)
	if err != nil {
		t.Fatalf("ComputeCreditCardCycleBounds leap: %v", err)
	}
	if bLeap.LastStatementDate != "2028-02-29" {
		t.Fatalf("expected LastStatementDate=2028-02-29, got %s", bLeap.LastStatementDate)
	}
	if bLeap.LastCycle.EndDate != "2028-02-29" {
		t.Fatalf("unexpected leap last cycle: %+v", bLeap.LastCycle)
	}

	// On statement day itself (Nov 22 WIB), current cycle is still Oct 23 - Nov 22 and last closed statement is Oct 22
	nowOnStmtDay := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC) // 19:00 WIB on Nov 22
	bOnDay, err := ComputeCreditCardCycleBounds(22, "Asia/Jakarta", nowOnStmtDay)
	if err != nil {
		t.Fatalf("ComputeCreditCardCycleBounds on statement day: %v", err)
	}
	if bOnDay.CurrentCycle.StartDate != "2026-10-23" || bOnDay.CurrentCycle.EndDate != "2026-11-22" {
		t.Fatalf("unexpected on-day current cycle: %+v", bOnDay.CurrentCycle)
	}
	if bOnDay.LastStatementDate != "2026-10-22" {
		t.Fatalf("expected LastStatementDate=2026-10-22 on Nov 22, got %s", bOnDay.LastStatementDate)
	}
}

func TestCreditCardStatementReconciliation_PreservesUnbilledSpending(t *testing.T) {
	gdb := openDomainTestDB(t)
	configureTestLocale(t, gdb, "Asia/Jakarta", "IDR")

	acct, err := CreateAccount(gdb, CreateAccountInput{
		Name:                  "Smbci 6039",
		Type:                  "credit_card",
		InitialBalanceMinor:   0,
		InitialBalanceAsOfUtc: strPtr("2026-10-01T00:00:00Z"),
		StatementDay:          intPtr(22),
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	// Last cycle charge on Nov 22 at 23:30 WIB (2026-11-22T16:30:00Z)
	seedAccountTx(t, gdb, "tx-last-cycle", acct.ID, 500000, "IDR", "2026-11-22T16:30:00Z")
	// Current unbilled cycle charges on Nov 23 at 00:05 WIB (2026-11-22T17:05:00Z) and Nov 24
	seedAccountTx(t, gdb, "tx-unbilled-1", acct.ID, 242455, "IDR", "2026-11-22T17:05:00Z")
	seedAccountTx(t, gdb, "tx-unbilled-2", acct.ID, 500000, "IDR", "2026-11-24T05:00:00Z")

	// Today is Nov 25 12:00 WIB (2026-11-25T05:00:00Z)
	now := time.Date(2026, 11, 25, 5, 0, 0, 0, time.UTC)

	// Reconcile as of Nov 22 23:59:59 WIB (2026-11-22T16:59:59Z) with IDR 9,139,549
	res, err := reconcileAccountAt(gdb, ReconcileAccountInput{
		AccountID:          acct.ID,
		ActualBalanceMinor: 9139549,
		RecordedAtUtc:      strPtr("2026-11-22T16:59:59Z"),
	}, now)
	if err != nil {
		t.Fatalf("reconcileAccountAt: %v", err)
	}

	rec := res["reconciliation"].(db.AccountReconciliation)
	// Computed as of Nov 22 23:59:59 WIB should only include tx-last-cycle (500,000), not Nov 23-24 unbilled tx
	if rec.ComputedBalanceMinor != 500000 {
		t.Fatalf("expected historical computed_balance_minor=500000, got %d", rec.ComputedBalanceMinor)
	}
	if rec.AdjustmentMinor != 9139549-500000 {
		t.Fatalf("expected adjustment_minor=%d, got %d", 9139549-500000, rec.AdjustmentMinor)
	}

	summary := res["account"].(AccountSummary)
	if summary.ComputedBalanceMinor == nil || *summary.ComputedBalanceMinor != 9882004 {
		t.Fatalf("expected live balance owed=9882004, got %v", summary.ComputedBalanceMinor)
	}
	if summary.BillingCycle == nil {
		t.Fatalf("expected non-nil billing_cycle")
	}
	if summary.BillingCycle.LastStatementDate != "2026-11-22" {
		t.Fatalf("expected last_statement_date=2026-11-22, got %s", summary.BillingCycle.LastStatementDate)
	}
	if summary.BillingCycle.CurrentCycle.SpentMinor != 742455 {
		t.Fatalf("expected current cycle unbilled spend=742455, got %d", summary.BillingCycle.CurrentCycle.SpentMinor)
	}
	if summary.BillingCycle.LastCycle.SpentMinor != 500000 {
		t.Fatalf("expected last cycle spend=500000, got %d", summary.BillingCycle.LastCycle.SpentMinor)
	}
}

func TestFutureDatedTransactions_ExcludedFromBalanceAndCounted(t *testing.T) {
	gdb := openDomainTestDB(t)
	configureTestLocale(t, gdb, "Asia/Jakarta", "IDR")

	acct, err := CreateAccount(gdb, CreateAccountInput{
		Name:                  "Smbci 6039",
		Type:                  "credit_card",
		InitialBalanceMinor:   0,
		InitialBalanceAsOfUtc: strPtr("2026-11-01T00:00:00Z"),
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	now := time.Date(2026, 11, 22, 13, 10, 0, 0, time.UTC)
	if _, err := reconcileAccountAt(gdb, ReconcileAccountInput{
		AccountID:          acct.ID,
		ActualBalanceMinor: 9882004,
		RecordedAtUtc:      strPtr("2026-11-22T13:08:50Z"),
	}, now); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	// Future-dated misclassified billing statement on Dec 8
	seedAccountTx(t, gdb, "tx-future-stmt", acct.ID, 9139549, "IDR", "2026-12-08T09:20:03Z")

	accounts, err := listAccountsAt(gdb, now)
	if err != nil {
		t.Fatalf("listAccountsAt: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(accounts))
	}
	got := accounts[0]
	if got.ComputedBalanceMinor == nil || *got.ComputedBalanceMinor != 9882004 {
		t.Fatalf("expected balance=9882004 (excluding future tx), got %v", got.ComputedBalanceMinor)
	}
	if got.FutureTransactionCount != 1 {
		t.Fatalf("expected future_transaction_count=1, got %d", got.FutureTransactionCount)
	}
	if got.BillingCycle != nil {
		t.Fatalf("expected nil billing_cycle when statement_day is unset")
	}

	// Also verify unanchored provisional account excludes future-dated tx from net_flow_minor
	prov := db.Account{
		ID:                    "prov-cc",
		Name:                  "Provisional CC",
		Type:                  "credit_card",
		Currency:              "IDR",
		InitialBalanceMinor:   nil,
		InitialBalanceAsOfUtc: "2026-11-01T00:00:00Z",
		Status:                "provisional",
		CreatedAtUtc:          "2026-11-01T00:00:00Z",
	}
	if err := gdb.Create(&prov).Error; err != nil {
		t.Fatalf("seed provisional account: %v", err)
	}
	seedAccountTx(t, gdb, "tx-prov-past", prov.ID, 100000, "IDR", "2026-11-20T10:00:00Z")
	seedAccountTx(t, gdb, "tx-prov-future", prov.ID, 999999, "IDR", "2026-12-08T10:00:00Z")

	bal, err := computeAccountBalanceAsOf(gdb, prov, ToUtcIsoSeconds(now), ToUtcIsoSeconds(now))
	if err != nil {
		t.Fatalf("computeAccountBalanceAsOf: %v", err)
	}
	if bal.NetFlowMinor != 100000 {
		t.Fatalf("expected provisional net_flow_minor=100000 (excluding future tx), got %d", bal.NetFlowMinor)
	}
	if bal.FutureTransactionCount != 1 {
		t.Fatalf("expected provisional future_transaction_count=1, got %d", bal.FutureTransactionCount)
	}
}

func TestDomainDBErrorPropagation(t *testing.T) {
	gdb := openDomainTestDB(t)
	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close sql.DB: %v", err)
	}

	if _, err := computeAccountBalanceAsOf(gdb, db.Account{ID: "acct-1", Currency: "USD"}, "2026-11-20T00:00:00Z", "2026-11-20T00:00:00Z"); err == nil {
		t.Fatalf("expected error from computeAccountBalanceAsOf on closed DB")
	}
	if err := RecomputeTransactionFromSources(gdb, "tx-1", "2026-11-20T00:00:00Z"); err == nil {
		t.Fatalf("expected error from RecomputeTransactionFromSources on closed DB")
	}
	if _, err := GetMonthlySpendingSummary(gdb, "2026-11"); err == nil {
		t.Fatalf("expected error from GetMonthlySpendingSummary on closed DB")
	}
}

func TestBackdatedReconciliation_PreservesNewerAnchor(t *testing.T) {
	gdb := openDomainTestDB(t)
	configureTestLocale(t, gdb, "Asia/Jakarta", "IDR")

	acct, err := CreateAccount(gdb, CreateAccountInput{
		Name:                  "Smbci 6039",
		Type:                  "credit_card",
		InitialBalanceMinor:   0,
		InitialBalanceAsOfUtc: strPtr("2026-11-01T00:00:00Z"),
		StatementDay:          intPtr(22),
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	now := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)

	// Newer reconciliation on Nov 25
	if _, err := reconcileAccountAt(gdb, ReconcileAccountInput{
		AccountID:          acct.ID,
		ActualBalanceMinor: 10000000,
		RecordedAtUtc:      strPtr("2026-11-25T09:00:00Z"),
	}, now); err != nil {
		t.Fatalf("reconcile Nov 25: %v", err)
	}

	// Older backdated reconciliation on Nov 22
	res, err := reconcileAccountAt(gdb, ReconcileAccountInput{
		AccountID:          acct.ID,
		ActualBalanceMinor: 9139549,
		RecordedAtUtc:      strPtr("2026-11-22T16:59:59Z"),
	}, now)
	if err != nil {
		t.Fatalf("reconcile Nov 22: %v", err)
	}

	summary := res["account"].(AccountSummary)
	if summary.LatestReconciliation == nil || summary.LatestReconciliation.RecordedAtUtc != "2026-11-25T09:00:00Z" {
		t.Fatalf("expected Nov 25 reconciliation to remain active anchor, got %+v", summary.LatestReconciliation)
	}
	if summary.ComputedBalanceMinor == nil || *summary.ComputedBalanceMinor != 10000000 {
		t.Fatalf("expected balance=10000000 anchored to Nov 25, got %v", summary.ComputedBalanceMinor)
	}

	// Reject future-dated reconciliation timestamp
	if _, err := reconcileAccountAt(gdb, ReconcileAccountInput{
		AccountID:          acct.ID,
		ActualBalanceMinor: 10000000,
		RecordedAtUtc:      strPtr("2026-11-26T00:00:00Z"),
	}, now); err == nil {
		t.Fatalf("expected error for future-dated reconciliation timestamp")
	}
}
