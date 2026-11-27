package domain

import (
	"testing"

	"portspendernote/internal/db"
)

func TestImportManualTransactions_HappyPathAndMatching(t *testing.T) {
	gdb := openDomainTestDB(t)
	if err := gdb.Model(&db.AppSetting{}).Where("id = 1").Update("timezone", "Asia/Jakarta").Error; err != nil {
		t.Fatal(err)
	}

	acct, err := CreateAccount(gdb, CreateAccountInput{
		Name: "BCA Debit",
		Type: "debit_card",
	})
	if err != nil {
		t.Fatal(err)
	}
	foodCat, err := CreateCategory(gdb, CreateCategoryInput{Name: "Snacks", Icon: "Utensils"})
	if err != nil {
		t.Fatal(err)
	}
	workTag, err := CreateTag(gdb, CreateTagInput{Name: "work"})
	if err != nil {
		t.Fatal(err)
	}

	unmatchedCat := "Totally Unknown Category"
	note := "Team lunch"
	res, err := ImportManualTransactions(gdb, ImportManualTransactionsInput{
		AccountID: acct.ID,
		Transactions: []ImportManualTransactionRow{
			{
				Kind:          "expense",
				AmountMinor:   45000,
				OccurredAtUtc: "2026-11-15",
				Merchant:      "Kopi Kenangan",
				Category:      &unmatchedCat,
				TagNames:      []string{"Work", "invented-tag"},
				Note:          &note,
			},
			{
				Kind:          "income",
				AmountMinor:   500000,
				OccurredAtUtc: "2026-11-16T03:30:00Z",
				Merchant:      "Client Refund",
				Category:      &foodCat.Name,
			},
		},
	})
	if err != nil {
		t.Fatalf("ImportManualTransactions failed: %v", err)
	}
	if res.ImportedCount != 2 || len(res.IDs) != 2 {
		t.Fatalf("unexpected result: %+v", res)
	}

	list, err := ListTransactions(gdb, ListTransactionsParams{SourceType: "manual", UnreviewedOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if list.TotalCount != 2 {
		t.Fatalf("expected 2 unreviewed manual transactions, got %d", list.TotalCount)
	}

	byID := map[string]TransactionListItem{}
	for _, item := range list.Items {
		byID[item.ID] = item
	}

	first := byID[res.IDs[0]]
	if first.SourceType != "manual" || first.Reviewed != 0 {
		t.Fatalf("expected manual unreviewed tx, got source=%s reviewed=%d", first.SourceType, first.Reviewed)
	}
	if first.Category != DefaultCategory {
		t.Fatalf("expected unmatched category to fall back to %q, got %q", DefaultCategory, first.Category)
	}
	if first.OccurredAtUtc != "2026-11-15T05:00:00Z" {
		t.Fatalf("expected Asia/Jakarta noon -> 2026-11-15T05:00:00Z, got %s", first.OccurredAtUtc)
	}
	if len(first.Tags) != 1 || first.Tags[0].ID != workTag.ID {
		t.Fatalf("expected only existing work tag, got %+v", first.Tags)
	}

	var tagCount int64
	if err := gdb.Model(&db.Tag{}).Where("normalized_name = ?", "invented-tag").Count(&tagCount).Error; err != nil {
		t.Fatal(err)
	}
	if tagCount != 0 {
		t.Fatalf("expected unmatched tag not to be created")
	}

	second := byID[res.IDs[1]]
	if second.Kind != "income" || second.Category != foodCat.Name || second.Reviewed != 0 {
		t.Fatalf("unexpected second tx: %+v", second)
	}
}

func TestImportManualTransactions_AtomicRollbackOnInvalidRow(t *testing.T) {
	gdb := openDomainTestDB(t)
	acct, err := CreateAccount(gdb, CreateAccountInput{
		Name: "BCA Debit",
		Type: "debit_card",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = ImportManualTransactions(gdb, ImportManualTransactionsInput{
		AccountID: acct.ID,
		Transactions: []ImportManualTransactionRow{
			{
				Kind:          "expense",
				AmountMinor:   10000,
				OccurredAtUtc: "2026-11-15T05:00:00Z",
				Merchant:      "Valid Merchant",
			},
			{
				Kind:          "transfer",
				AmountMinor:   20000,
				OccurredAtUtc: "2026-11-15T06:00:00Z",
				Merchant:      "Invalid Transfer",
			},
		},
	})
	if err == nil {
		t.Fatalf("expected error for transfer kind")
	}

	var count int64
	if err := gdb.Model(&db.Transaction{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected atomic rollback leaving 0 rows, got %d", count)
	}
}
