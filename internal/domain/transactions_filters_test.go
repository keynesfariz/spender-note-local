package domain

import (
	"testing"

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

	seedLegacySkippedEmail(
		t, gdb, "g-match", &sender.ID, "wallet@pay.test", "Coffee Transfer", "Body", 2000, "2026-11-24T04:00:00Z", "qwen",
		`{"email_kind":"non_transaction","amount_decimal":"25000","currency":"IDR","transaction_date":"2026-11-24","transaction_time":"11:00:00","timezone_offset":"+07:00","merchant":"Kopi Kenangan","category":"Dining Out","payment_method":"Main Wallet"}`,
	)
	seedLegacySkippedEmail(
		t, gdb, "g-other-date", &sender.ID, "wallet@pay.test", "Older Transfer", "Body", 1500, "2026-11-20T04:00:00Z", "qwen",
		`{"email_kind":"non_transaction","amount_decimal":"30000","currency":"IDR","transaction_date":"2026-11-20","transaction_time":"11:00:00","timezone_offset":"+07:00","merchant":"Bakery","category":"Dining Out"}`,
	)
	if err := BackfillSkippedEmailTriageData(gdb); err != nil {
		t.Fatal(err)
	}

	// Search mismatch does NOT collapse date-bounded ghostInRangeCount.
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
