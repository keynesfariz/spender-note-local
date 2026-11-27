package domain

import (
	"testing"
	"time"

	"portspendernote/internal/db"
)

func TestTransactions_FilterByCategoryOrTagNamedAll(t *testing.T) {
	gdb := openDomainTestDB(t)
	category, err := CreateCategory(gdb, CreateCategoryInput{Name: "all", Icon: "Tag"})
	if err != nil {
		t.Fatal(err)
	}
	tag, err := CreateTag(gdb, CreateTagInput{Name: "all"})
	if err != nil {
		t.Fatal(err)
	}
	match, err := CreateManualTransaction(gdb, CreateManualTransactionInput{
		AmountMinor:   100,
		OccurredAtUtc: "2026-11-21T08:00:00Z",
		Merchant:      "Matching transaction",
		Category:      &category.Name,
		TagIDs:        []string{tag.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreateManualTransaction(gdb, CreateManualTransactionInput{
		AmountMinor:   200,
		OccurredAtUtc: "2026-11-21T09:00:00Z",
		Merchant:      "Other transaction",
	}); err != nil {
		t.Fatal(err)
	}
	for _, filter := range []ListTransactionsParams{{Category: "all"}, {Tag: "all"}} {
		result, err := ListTransactions(gdb, filter)
		if err != nil {
			t.Fatal(err)
		}
		if result.TotalCount != 1 || len(result.Items) != 1 || result.Items[0].ID != match.ID {
			t.Fatalf("filter %+v should return only %s, got %+v", filter, match.ID, result.Items)
		}
	}
}

func TestListTransactions_LedgerRange(t *testing.T) {
	gdb := openDomainTestDB(t)
	now := time.Date(2026, 11, 25, 18, 0, 0, 0, time.UTC)
	for _, timestamp := range []string{"2026-11-25T10:00:00Z", "2026-11-25T18:00:00Z"} {
		if _, err := CreateManualTransaction(gdb, CreateManualTransactionInput{
			AmountMinor: 100, OccurredAtUtc: timestamp, Merchant: "Test",
		}); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		name          string
		params        ListTransactionsParams
		count         int64
		wantDate      string
		wantTimestamp string
	}{
		{"legacy all dates", ListTransactionsParams{}, 2, "", ""},
		{"explicit all dates", ListTransactionsParams{LedgerRange: "all"}, 2, "", ""},
		{"local today", ListTransactionsParams{LedgerRange: "dates"}, 1, "2026-11-26", "2026-11-25T18:00:00Z"},
		{"end only", ListTransactionsParams{LedgerRange: "dates", EndDate: "2026-11-26"}, 1, "2026-11-26", "2026-11-25T18:00:00Z"},
		{"start only", ListTransactionsParams{LedgerRange: "dates", StartDate: "2026-11-25"}, 1, "2026-11-25", "2026-11-25T10:00:00Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := listTransactionsAt(gdb, tc.params, now)
			if err != nil || got.TotalCount != tc.count {
				t.Fatalf("list = %+v, %v, want count %d", got, err, tc.count)
			}
			if tc.wantDate != "" {
				if got.Ledger == nil || got.Ledger.StartDate != tc.wantDate || got.Ledger.EndDate != tc.wantDate ||
					got.Ledger.Today != "2026-11-26" || got.Ledger.NextDayUtc != "2026-11-26T17:00:00Z" {
					t.Fatalf("unexpected ledger dates: %+v", got.Ledger)
				}
				if len(got.Items) != 1 || got.Items[0].OccurredAtUtc != tc.wantTimestamp {
					t.Fatalf("unexpected ledger rows: %+v", got.Items)
				}
			}
		})
	}
}

func TestListTransactions_LedgerRangeDST(t *testing.T) {
	gdb := openDomainTestDB(t)
	if err := gdb.Model(&db.AppSetting{}).Where("id = 1").Update("timezone", "America/New_York").Error; err != nil {
		t.Fatal(err)
	}
	for _, stamp := range []string{"2026-03-08T04:59:59Z", "2026-03-08T05:00:00Z", "2026-03-09T04:00:00Z"} {
		if _, err := CreateManualTransaction(gdb, CreateManualTransactionInput{
			AmountMinor: 100, OccurredAtUtc: stamp, Merchant: "DST example",
		}); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 3, 8, 19, 0, 0, 0, time.UTC)
	list, err := listTransactionsAt(gdb, ListTransactionsParams{LedgerRange: "dates"}, now)
	if err != nil || len(list.Items) != 1 || list.Items[0].OccurredAtUtc != "2026-03-08T05:00:00Z" {
		t.Fatalf("unexpected DST ledger: %+v, %v", list, err)
	}
	if list.Ledger.Today != "2026-03-08" || list.Ledger.NextDayUtc != "2026-03-09T04:00:00Z" {
		t.Fatalf("unexpected local day boundary: %+v", list.Ledger)
	}
}

func TestListTransactions_LedgerRangeMissingMidnight(t *testing.T) {
	gdb := openDomainTestDB(t)
	if err := gdb.Model(&db.AppSetting{}).Where("id = 1").Update("timezone", "America/Santiago").Error; err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 6, 3, 30, 0, 0, time.UTC)
	if _, err := CreateManualTransaction(gdb, CreateManualTransactionInput{
		AmountMinor: 100, OccurredAtUtc: "2026-09-06T03:59:59Z", Merchant: "Before skipped midnight",
	}); err != nil {
		t.Fatal(err)
	}
	list, err := listTransactionsAt(gdb, ListTransactionsParams{LedgerRange: "dates"}, now)
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("expected date range to retain the last hour: %+v, %v", list, err)
	}
	if list.Ledger.Today != "2026-09-05" || list.Ledger.NextDayUtc != "2026-09-06T04:00:00Z" {
		t.Fatalf("unexpected local day boundary: %+v", list.Ledger)
	}
}

func TestMonthlySpending_IncomeFiltersAndMonthlyCardPayments(t *testing.T) {
	gdb := openDomainTestDB(t)
	category, err := CreateCategory(gdb, CreateCategoryInput{Name: "Dinner", Icon: "Tag"})
	if err != nil {
		t.Fatal(err)
	}
	dinner, err := CreateTag(gdb, CreateTagInput{Name: "Dinner"})
	if err != nil {
		t.Fatal(err)
	}
	extra, err := CreateTag(gdb, CreateTagInput{Name: "Extra"})
	if err != nil {
		t.Fatal(err)
	}
	funding := seedTestAccount(t, gdb, "funding", "Wallet", "IDR")
	if err := gdb.Model(&db.Account{}).Where("id = ?", funding).Update("type", "digital_wallet").Error; err != nil {
		t.Fatal(err)
	}
	card := seedTestAccount(t, gdb, "card", "Local card", "IDR")
	foreignCard := seedTestAccount(t, gdb, "foreign-card", "Foreign card", "USD")
	seed := func(tx db.Transaction, tags ...string) {
		t.Helper()
		tx.SourceType = "manual"
		tx.AccountID = &funding
		tx.CreatedAtUtc = tx.OccurredAtUtc
		tx.UpdatedAtUtc = tx.OccurredAtUtc
		if tx.CategoryID == "" {
			tx.CategoryID = category.ID
		}
		if tx.Currency == "" {
			tx.Currency = "IDR"
		}
		if err := gdb.Create(&tx).Error; err != nil {
			t.Fatal(err)
		}
		for _, tag := range tags {
			if err := gdb.Create(&db.TransactionTag{TransactionID: tx.ID, TagID: tag, CreatedAtUtc: tx.CreatedAtUtc}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}

	seed(db.Transaction{ID: "expense", AmountMinor: 1_000_000, Merchant: "Shared dinner", OccurredAtUtc: "2026-11-30T16:59:59Z"}, dinner.ID, extra.ID)
	seed(db.Transaction{ID: "income", Kind: "income", AmountMinor: 750_000, OccurredAtUtc: "2026-11-30T16:59:59Z"}, dinner.ID)
	cardAmount, foreignAmount := int64(900_000), int64(1000)
	localCurrency, foreignCurrency := "IDR", "USD"
	seed(db.Transaction{ID: "local-paid", Kind: "transfer", AmountMinor: cardAmount, DestinationAccountID: &card, DestinationAmountMinor: &cardAmount, DestinationCurrency: &localCurrency, OccurredAtUtc: "2026-11-25T10:00:00Z"}, dinner.ID)
	seed(db.Transaction{ID: "foreign-paid", Kind: "transfer", AmountMinor: 200_000, DestinationAccountID: &foreignCard, DestinationAmountMinor: &foreignAmount, DestinationCurrency: &foreignCurrency, OccurredAtUtc: "2026-11-25T10:00:00Z"})
	seed(db.Transaction{ID: "untagged-income", Kind: "income", AmountMinor: 500_000, OccurredAtUtc: "2026-11-30T16:59:59Z"})
	seed(db.Transaction{ID: "next-month-income", Kind: "income", AmountMinor: 150_000, OccurredAtUtc: "2026-11-30T17:00:00Z"}, dinner.ID)
	seed(db.Transaction{ID: "foreign-expense", AmountMinor: 100, Currency: "USD", OccurredAtUtc: "2026-11-27T10:00:00Z"}, dinner.ID)
	for _, tc := range []struct {
		name, month, category, tag                                              string
		expenses, income, net, payments, foreignEntries, foreignPayments, count int64
	}{
		{"unfiltered with additional income", "2026-11", "", "", 1_000_000, 1_250_000, -250_000, 900_000, 1, 1, 4},
		{"intersect category and tag", "2026-11", category.Name, dinner.ID, 1_000_000, 750_000, 250_000, 900_000, 1, 1, 3},
		{"uppercase tag excludes untagged income", "2026-11", "", "DINNER", 1_000_000, 750_000, 250_000, 900_000, 1, 1, 3},
		{"excluded category retains all card payments", "2026-11", "Different category", dinner.ID, 0, 0, 0, 900_000, 0, 1, 0},
		{"next calendar month can be negative", "2026-12", "", dinner.ID, 0, 150_000, -150_000, 0, 0, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := GetFilteredMonthlySpendingSummary(gdb, tc.month, tc.category, tc.tag)
			if err != nil {
				t.Fatal(err)
			}
			if s.TotalSpentMinor != tc.expenses || s.IncomeMinor != tc.income || s.NetSpentMinor != tc.net ||
				s.CardPaymentsMinor != tc.payments || s.ForeignCurrencyCount != tc.foreignEntries || s.ForeignCardPaymentCount != tc.foreignPayments ||
				s.TransactionCount != tc.count {
				t.Fatalf("unexpected summary: %+v", s)
			}
			if tc.expenses > 0 && (len(s.ByCategory) != 1 || s.ByCategory[0].TotalMinor != tc.expenses || s.ByCategory[0].Count != 1) {
				t.Fatalf("expense breakdown must exclude income and count tagged expense once: %+v", s.ByCategory)
			}
		})
	}
}

func TestListTransactions_TransferKindMatchesEitherAccount(t *testing.T) {
	gdb := openDomainTestDB(t)
	from := seedTestAccount(t, gdb, "from-account", "Source card", "IDR")
	to := seedTestAccount(t, gdb, "to-account", "Destination card", "USD")
	unrelated := seedTestAccount(t, gdb, "unrelated-account", "Unrelated account", "IDR")
	timestamp, toPosting := "2026-11-22T10:00:00Z", "2026-11-22T11:00:00Z"
	row, err := CreateManualTransaction(gdb, CreateManualTransactionInput{
		AmountMinor: 100_000, OccurredAtUtc: timestamp, Merchant: "Transfer", AccountID: &from,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := gdb.Model(&db.Transaction{}).Where("id = ?", row.ID).Updates(map[string]any{
		"kind": "transfer", "destination_account_id": to, "destination_amount_minor": 500,
		"destination_currency": "USD", "source_posted_at_utc": timestamp, "destination_posted_at_utc": toPosting,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := CreateManualTransaction(gdb, CreateManualTransactionInput{
		AmountMinor: 100_000, OccurredAtUtc: timestamp, Merchant: "Expense", AccountID: &from,
	}); err != nil {
		t.Fatal(err)
	}

	for _, scope := range []struct {
		account string
		count   int64
	}{{from, 1}, {to, 1}, {"", 1}, {unrelated, 0}} {
		result, err := ListTransactions(gdb, ListTransactionsParams{AccountID: scope.account, Kind: "transfer"})
		if err != nil {
			t.Fatal(err)
		}
		if result.TotalCount != scope.count || int64(len(result.Items)) != scope.count {
			t.Fatalf("account scope %q should contain %d transfer(s): %+v", scope.account, scope.count, result)
		}
		for _, item := range result.Items {
			if item.ID != row.ID || item.Kind != "transfer" || item.DestinationAccountName == nil || *item.DestinationAccountName != "Destination card" || item.DestinationPostedAtUtc == nil || *item.DestinationPostedAtUtc != toPosting {
				t.Fatalf("unexpected transfer details: %+v", item)
			}
		}
	}
	detail, err := GetTransactionInspector(gdb, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if item := detail.Transaction; item.AccountID == nil || *item.AccountID != from || item.DestinationAccountID == nil || *item.DestinationAccountID != to ||
		item.DestinationAmountMinor == nil || *item.DestinationAmountMinor != 500 || item.DestinationCurrency == nil || *item.DestinationCurrency != "USD" ||
		item.SourcePostedAtUtc == nil || *item.SourcePostedAtUtc != timestamp {
		t.Fatalf("missing transfer sides in detail: %+v", item)
	}
}

func TestListTransactions_SourceTypeFilter(t *testing.T) {
	gdb := openDomainTestDB(t)
	nowUtc := NowUtcIsoSeconds()

	manualReviewed, err := CreateManualTransaction(gdb, CreateManualTransactionInput{
		AmountMinor:   15000,
		OccurredAtUtc: "2026-11-20T05:00:00Z",
		Merchant:      "Manual Cafe",
	})
	if err != nil {
		t.Fatal(err)
	}

	catID, err := validateTransactionCategoryID(gdb, DefaultCategory)
	if err != nil {
		t.Fatal(err)
	}
	gmailTx := db.Transaction{
		ID:            "tx-gmail-1",
		SourceType:    "gmail",
		Kind:          "expense",
		AmountMinor:   25000,
		Currency:      "IDR",
		OccurredAtUtc: "2026-11-20T06:00:00Z",
		Merchant:      "Gmail Store",
		CategoryID:    catID,
		Reviewed:      0,
		CreatedAtUtc:  nowUtc,
		UpdatedAtUtc:  nowUtc,
	}
	manualUnreviewed := db.Transaction{
		ID:            "tx-manual-unreviewed",
		SourceType:    "manual",
		Kind:          "expense",
		AmountMinor:   35000,
		Currency:      "IDR",
		OccurredAtUtc: "2026-11-20T07:00:00Z",
		Merchant:      "Imported Cafe",
		CategoryID:    catID,
		Reviewed:      0,
		CreatedAtUtc:  nowUtc,
		UpdatedAtUtc:  nowUtc,
	}
	if err := gdb.Create(&gmailTx).Error; err != nil {
		t.Fatal(err)
	}
	if err := gdb.Create(&manualUnreviewed).Error; err != nil {
		t.Fatal(err)
	}

	allRes, err := ListTransactions(gdb, ListTransactionsParams{SourceType: "all"})
	if err != nil || allRes.TotalCount != 3 {
		t.Fatalf("all source filter = %+v, %v; want 3", allRes, err)
	}

	gmailRes, err := ListTransactions(gdb, ListTransactionsParams{SourceType: "gmail"})
	if err != nil || gmailRes.TotalCount != 1 || gmailRes.Items[0].ID != gmailTx.ID {
		t.Fatalf("gmail source filter = %+v, %v; want %s", gmailRes, err, gmailTx.ID)
	}

	manualRes, err := ListTransactions(gdb, ListTransactionsParams{SourceType: "manual"})
	if err != nil || manualRes.TotalCount != 2 {
		t.Fatalf("manual source filter = %+v, %v; want 2 (including %s)", manualRes, err, manualReviewed.ID)
	}

	unreviewedManualRes, err := ListTransactions(gdb, ListTransactionsParams{SourceType: "manual", UnreviewedOnly: true})
	if err != nil || unreviewedManualRes.TotalCount != 1 || unreviewedManualRes.Items[0].ID != manualUnreviewed.ID {
		t.Fatalf("manual+unreviewed filter = %+v, %v; want %s", unreviewedManualRes, err, manualUnreviewed.ID)
	}

	if _, err := ListTransactions(gdb, ListTransactionsParams{SourceType: "csv"}); err == nil {
		t.Fatalf("expected error for invalid sourceType")
	}
}
