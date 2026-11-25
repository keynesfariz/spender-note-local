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

func TestListTransactions_GhostRowFilteringAndDateOnlyInRangeCount(t *testing.T) {
	gdb := openDomainTestDB(t)
	sender := seedSenderWithCursor(t, gdb, "s-filt", "wallet@pay.test", 1000, "auto")
	acctA := seedTestAccount(t, gdb, "acct-filt-a", "Main Wallet", "IDR")
	if err := gdb.Model(&db.Sender{}).Where("id = ?", sender.ID).Update("default_account_id", acctA).Error; err != nil {
		t.Fatal(err)
	}

	tag, err := CreateTag(gdb, CreateTagInput{Name: "Weekend"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = CreateManualTransaction(gdb, CreateManualTransactionInput{
		AmountMinor: 50000, OccurredAtUtc: "2026-11-24T05:00:00Z", Merchant: "Tagged Tx", TagIDs: []string{tag.ID}, AccountID: &acctA,
	})
	if err != nil {
		t.Fatal(err)
	}

	seedSkippedEmail(
		t, gdb, "g-match", &sender.ID, "wallet@pay.test", "Coffee Transfer", "Body", 2000, "2026-11-24T04:00:00Z", "qwen",
		`{"email_kind":"non_transaction","amount_decimal":"25000","currency":"IDR","transaction_date":"2026-11-24","transaction_time":"11:00:00","timezone_offset":"+07:00","merchant":"Kopi Kenangan","category":"Dining Out","payment_method":"Main Wallet"}`,
	)
	seedSkippedEmail(
		t, gdb, "g-other-date", &sender.ID, "wallet@pay.test", "Older Transfer", "Body", 1500, "2026-11-20T04:00:00Z", "qwen",
		`{"email_kind":"non_transaction","amount_decimal":"30000","currency":"IDR","transaction_date":"2026-11-20","transaction_time":"11:00:00","timezone_offset":"+07:00","merchant":"Bakery","category":"Dining Out"}`,
	)

	searchMiss, err := ListTransactions(gdb, ListTransactionsParams{
		StartDate: "2026-11-24", EndDate: "2026-11-24", Search: "nonexistent-term",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(searchMiss.GhostRows) != 0 || searchMiss.GhostInRangeCount != 1 || searchMiss.TotalGhostCount != 2 {
		t.Fatalf("expected date-only ghostInRangeCount=1 totalGhostCount=2 with 0 search hits, got len=%d inRange=%d total=%d", len(searchMiss.GhostRows), searchMiss.GhostInRangeCount, searchMiss.TotalGhostCount)
	}

	// Tag filter hides Ghost Rows cleanly.
	withTag, err := ListTransactions(gdb, ListTransactionsParams{
		StartDate: "2026-11-24", EndDate: "2026-11-24", Tag: tag.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(withTag.GhostRows) != 0 {
		t.Fatalf("expected tag filter to hide Ghost Rows, got %d", len(withTag.GhostRows))
	}

	// showSkipped=false hides Ghost Rows while unreviewedOnly=true keeps them.
	hideSkipped := false
	hiddenRes, err := ListTransactions(gdb, ListTransactionsParams{
		StartDate: "2026-11-24", EndDate: "2026-11-24", ShowSkipped: &hideSkipped,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hiddenRes.GhostRows) != 0 || hiddenRes.GhostInRangeCount != 1 {
		t.Fatalf("expected showSkipped=false to hide ghost rows while keeping range count=1, got %+v", hiddenRes)
	}

	unrevRes, err := ListTransactions(gdb, ListTransactionsParams{
		StartDate: "2026-11-24", EndDate: "2026-11-24", UnreviewedOnly: true, Search: "kopi", Category: "Dining Out", AccountID: acctA,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(unrevRes.GhostRows) != 1 || unrevRes.GhostRows[0].EmailID != "g-match" {
		t.Fatalf("expected g-match across search/category/account/unreviewed filters, got %+v", unrevRes.GhostRows)
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
