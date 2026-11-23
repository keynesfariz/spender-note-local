package domain

import (
	"testing"

	"portspendernote/internal/db"
)

func TestCategories_ProtectsSystemOtherCategory(t *testing.T) {
	gdb := openDomainTestDB(t)

	cats, err := ListCategories(gdb)
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	var otherID string
	for _, c := range cats {
		if c.Name == DefaultCategory {
			otherID = c.ID
			if c.IsSystem != 1 || c.Icon != "Ellipsis" {
				t.Fatalf("expected system 'Other' with Ellipsis icon, got %+v", c)
			}
		}
	}
	if otherID == "" {
		t.Fatalf("expected seeded 'Other' category")
	}

	newName := "Misc"
	newIcon := "Tag"
	if _, err := UpdateCategory(gdb, otherID, UpdateCategoryInput{Name: &newName, Icon: &newIcon}); err == nil {
		t.Fatalf("expected updating system 'Other' category to fail")
	}
	if err := DeleteCategory(gdb, otherID, "Housing"); err == nil {
		t.Fatalf("expected deleting system 'Other' category to fail")
	}

	reloaded, err := ListCategories(gdb)
	if err != nil {
		t.Fatalf("reload categories: %v", err)
	}
	found := false
	for _, c := range reloaded {
		if c.ID == otherID && c.Name == "Other" && c.Icon == "Ellipsis" && c.IsSystem == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected system 'Other' category to remain untouched")
	}
}

func TestCategories_CreateUpdateCaseInsensitiveUniquenessAndRenamePropagation(t *testing.T) {
	gdb := openDomainTestDB(t)

	if _, err := CreateCategory(gdb, CreateCategoryInput{Name: "   ", Icon: "Coffee"}); err == nil {
		t.Fatalf("expected blank category name to be rejected")
	}
	if _, err := CreateCategory(gdb, CreateCategoryInput{Name: " dining out ", Icon: "Utensils"}); err == nil {
		t.Fatalf("expected case-insensitive duplicate category name to be rejected")
	}

	created, err := CreateCategory(gdb, CreateCategoryInput{Name: "  Coffee & Tea  ", Icon: "Coffee"})
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	if created.Name != "Coffee & Tea" || created.Icon != "Coffee" || created.IsSystem != 0 {
		t.Fatalf("unexpected created category: %+v", created)
	}

	seedTestEmail(t, gdb, "email-cat-1", 1795251000000, "2026-11-21T08:50:00Z")
	txID, err := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
		EmailID:       "email-cat-1",
		EmailKind:     "receipt",
		AmountMinor:   25000,
		Currency:      "IDR",
		OccurredAtUtc: "2026-11-21T08:50:00Z",
		MerchantRaw:   "Kopi Kenangan",
		Category:      "Coffee & Tea",
	})
	if err != nil {
		t.Fatalf("SaveExtractedTransaction: %v", err)
	}

	// Renaming to existing category casing fails.
	dupName := "housing"
	if _, err := UpdateCategory(gdb, created.ID, UpdateCategoryInput{Name: &dupName}); err == nil {
		t.Fatalf("expected duplicate rename to fail")
	}

	renamed := "Cafe & Drinks"
	newIcon := "Wine"
	updated, err := UpdateCategory(gdb, created.ID, UpdateCategoryInput{Name: &renamed, Icon: &newIcon})
	if err != nil {
		t.Fatalf("UpdateCategory: %v", err)
	}
	if updated.Name != "Cafe & Drinks" || updated.Icon != "Wine" || updated.TransactionCount != 1 {
		t.Fatalf("unexpected updated category: %+v", updated)
	}

	var tx db.Transaction
	if err := gdb.First(&tx, "id = ?", txID).Error; err != nil {
		t.Fatalf("load tx: %v", err)
	}
	if tx.Category != "Cafe & Drinks" {
		t.Fatalf("expected transaction category updated to 'Cafe & Drinks', got %q", tx.Category)
	}

	var ext db.EmailExtraction
	if err := gdb.First(&ext, "email_id = ?", "email-cat-1").Error; err != nil {
		t.Fatalf("load extraction: %v", err)
	}
	if ext.Category == nil || *ext.Category != "Cafe & Drinks" {
		t.Fatalf("expected email_extraction category updated to 'Cafe & Drinks', got %v", ext.Category)
	}
}

func TestCategories_DeleteReassignsTransactionsAndExtractions(t *testing.T) {
	gdb := openDomainTestDB(t)

	catA, err := CreateCategory(gdb, CreateCategoryInput{Name: "Pets", Icon: "PawPrint"})
	if err != nil {
		t.Fatalf("create Pets: %v", err)
	}
	catB, err := CreateCategory(gdb, CreateCategoryInput{Name: "Hobbies", Icon: "Gamepad2"})
	if err != nil {
		t.Fatalf("create Hobbies: %v", err)
	}

	seedTestEmail(t, gdb, "email-del-1", 1795251000000, "2026-11-21T08:50:00Z")
	tx1, err := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
		EmailID: "email-del-1", EmailKind: "receipt", AmountMinor: 10000, Currency: "IDR",
		OccurredAtUtc: "2026-11-21T08:50:00Z", MerchantRaw: "Pet Store", Category: "Pets",
	})
	if err != nil {
		t.Fatalf("save tx1: %v", err)
	}
	tx2, err := CreateManualTransaction(gdb, CreateManualTransactionInput{
		AmountMinor: 20000, OccurredAtUtc: "2026-11-21T09:00:00Z", Merchant: "Board Games", Category: &catB.Name,
	})
	if err != nil {
		t.Fatalf("create tx2: %v", err)
	}

	if err := DeleteCategory(gdb, catA.ID, "NonExistent"); err == nil {
		t.Fatalf("expected DeleteCategory to reject nonexistent replacement category")
	}
	if err := DeleteCategory(gdb, catA.ID, catA.Name); err == nil {
		t.Fatalf("expected DeleteCategory to reject self as replacement category")
	}

	// Empty replacement defaults to "Other".
	if err := DeleteCategory(gdb, catA.ID, ""); err != nil {
		t.Fatalf("DeleteCategory default replacement: %v", err)
	}
	var gotTx1 db.Transaction
	if err := gdb.First(&gotTx1, "id = ?", tx1).Error; err != nil {
		t.Fatalf("load tx1: %v", err)
	}
	if gotTx1.Category != "Other" {
		t.Fatalf("expected tx1 reassigned to 'Other', got %q", gotTx1.Category)
	}
	var gotExt1 db.EmailExtraction
	if err := gdb.First(&gotExt1, "email_id = ?", "email-del-1").Error; err != nil {
		t.Fatalf("load ext1: %v", err)
	}
	if gotExt1.Category == nil || *gotExt1.Category != "Other" {
		t.Fatalf("expected extraction reassigned to 'Other', got %v", gotExt1.Category)
	}

	// Explicit replacement category.
	if err := DeleteCategory(gdb, catB.ID, "Shopping"); err != nil {
		t.Fatalf("DeleteCategory explicit replacement: %v", err)
	}
	var gotTx2 db.Transaction
	if err := gdb.First(&gotTx2, "id = ?", tx2.ID).Error; err != nil {
		t.Fatalf("load tx2: %v", err)
	}
	if gotTx2.Category != "Shopping" {
		t.Fatalf("expected tx2 reassigned to 'Shopping', got %q", gotTx2.Category)
	}
}

func TestTags_CRUDCaseInsensitiveUniquenessAndCascadeDelete(t *testing.T) {
	gdb := openDomainTestDB(t)

	if _, err := CreateTag(gdb, CreateTagInput{Name: "   "}); err == nil {
		t.Fatalf("expected blank tag name to fail")
	}

	tag, err := CreateTag(gdb, CreateTagInput{Name: "  Work Trip  "})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if tag.Name != "Work Trip" {
		t.Fatalf("expected trimmed tag name 'Work Trip', got %q", tag.Name)
	}
	if _, err := CreateTag(gdb, CreateTagInput{Name: "work trip"}); err == nil {
		t.Fatalf("expected case-insensitive duplicate tag to fail")
	}

	otherTag, err := CreateTag(gdb, CreateTagInput{Name: "Reimbursable"})
	if err != nil {
		t.Fatalf("CreateTag other: %v", err)
	}
	if _, err := UpdateTag(gdb, otherTag.ID, UpdateTagInput{Name: "WORK TRIP"}); err == nil {
		t.Fatalf("expected duplicate tag rename to fail")
	}

	tx, err := CreateManualTransaction(gdb, CreateManualTransactionInput{
		AmountMinor:   50000,
		OccurredAtUtc: "2026-11-21T10:00:00Z",
		Merchant:      "Flight",
	})
	if err != nil {
		t.Fatalf("create tx: %v", err)
	}
	if err := gdb.Create(&db.TransactionTag{
		TransactionID: tx.ID,
		TagID:         tag.ID,
		CreatedAtUtc:  NowUtcIsoSeconds(),
	}).Error; err != nil {
		t.Fatalf("attach tag: %v", err)
	}

	renamed, err := UpdateTag(gdb, tag.ID, UpdateTagInput{Name: "Japan Trip"})
	if err != nil {
		t.Fatalf("UpdateTag: %v", err)
	}
	if renamed.Name != "Japan Trip" || renamed.TransactionCount != 1 {
		t.Fatalf("unexpected renamed tag summary: %+v", renamed)
	}

	if err := DeleteTag(gdb, tag.ID); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
	var linkCnt int64
	gdb.Model(&db.TransactionTag{}).Where("tag_id = ?", tag.ID).Count(&linkCnt)
	if linkCnt != 0 {
		t.Fatalf("expected transaction_tags rows removed on tag delete, got %d", linkCnt)
	}
	var txCnt int64
	gdb.Model(&db.Transaction{}).Where("id = ?", tx.ID).Count(&txCnt)
	if txCnt != 1 {
		t.Fatalf("expected transaction to survive tag deletion, got count=%d", txCnt)
	}
}
