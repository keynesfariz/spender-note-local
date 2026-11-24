package domain

import (
	"fmt"
	"testing"

	"portspendernote/internal/db"
)

func TestDuplicateMergeTransactions_RejectsMismatchedAmountAndAccount(t *testing.T) {
	gdb := openDomainTestDB(t)
	acctA := seedTestAccount(t, gdb, "acct-a", "Card A", "IDR")
	acctB := seedTestAccount(t, gdb, "acct-b", "Card B", "IDR")
	seedTestEmail(t, gdb, "e1", 1795251000000, "2026-11-21T08:50:00Z")
	seedTestEmail(t, gdb, "e2", 1795251005000, "2026-11-21T08:50:05Z")
	seedTestEmail(t, gdb, "e3", 1795251010000, "2026-11-21T08:50:10Z")

	tx1, _ := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
		EmailID: "e1", EmailKind: "card_charge", AmountMinor: 12300, Currency: "IDR",
		OccurredAtUtc: "2026-11-21T08:50:00Z", MerchantRaw: "Coffee", Category: "Food", ResolvedAccountID: &acctA,
	})
	tx2, _ := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
		EmailID: "e2", EmailKind: "receipt", AmountMinor: 32400, Currency: "IDR",
		OccurredAtUtc: "2026-11-21T08:50:05Z", MerchantRaw: "Lunch", Category: "Food", ResolvedAccountID: &acctA,
	})
	tx3, _ := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
		EmailID: "e3", EmailKind: "receipt", AmountMinor: 12300, Currency: "IDR",
		OccurredAtUtc: "2026-11-21T08:50:10Z", MerchantRaw: "Coffee", Category: "Food", ResolvedAccountID: &acctB,
	})

	if _, err := DuplicateMergeTransactions(gdb, tx1, tx2); err == nil {
		t.Fatalf("expected DuplicateMergeTransactions to reject mismatched amounts (12300 vs 32400)")
	}
	if _, err := DuplicateMergeTransactions(gdb, tx1, tx3); err == nil {
		t.Fatalf("expected DuplicateMergeTransactions to reject mismatched accounts")
	}

	var count int64
	gdb.Model(&db.Transaction{}).Count(&count)
	if count != 3 {
		t.Fatalf("expected all 3 transactions to remain untouched after failed merges, got %d", count)
	}
}

func TestDuplicateMergeTransactions_TransfersEmailsWithoutDismissingAndMarksReviewed(t *testing.T) {
	gdb := openDomainTestDB(t)
	seedTestEmail(t, gdb, "e1", 1795251000000, "2026-11-21T08:50:00Z")
	seedTestEmail(t, gdb, "e2", 1795251005000, "2026-11-21T08:50:05Z")

	txKeep, err := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
		EmailID: "e1", EmailKind: "card_charge", AmountMinor: 50000, Currency: "IDR",
		OccurredAtUtc: "2026-11-21T08:50:00Z", MerchantRaw: "GrabFood", Category: "Food",
	})
	if err != nil {
		t.Fatalf("save txKeep: %v", err)
	}
	txDrop, err := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
		EmailID: "e2", EmailKind: "receipt", AmountMinor: 50000, Currency: "IDR",
		OccurredAtUtc: "2026-11-21T08:50:05Z", MerchantRaw: "GrabFood Receipt", Category: "Food",
	})
	if err != nil {
		t.Fatalf("save txDrop: %v", err)
	}

	merged, err := DuplicateMergeTransactions(gdb, txKeep, txDrop)
	if err != nil {
		t.Fatalf("DuplicateMergeTransactions: %v", err)
	}
	if merged.ID != txKeep {
		t.Fatalf("expected kept ID %s, got %s", txKeep, merged.ID)
	}
	if merged.Reviewed != 1 {
		t.Fatalf("expected merged transaction to be reviewed=1, got %d", merged.Reviewed)
	}
	if merged.SourceCount != 2 {
		t.Fatalf("expected 2 linked sources, got %d", merged.SourceCount)
	}

	var dropCount int64
	gdb.Model(&db.Transaction{}).Where("id = ?", txDrop).Count(&dropCount)
	if dropCount != 0 {
		t.Fatalf("expected dropped transaction to be deleted")
	}

	var emails []db.RawEmail
	gdb.Where("id IN ?", []string{"e1", "e2"}).Find(&emails)
	for _, em := range emails {
		if em.Status != "extracted" {
			t.Fatalf("expected email %s status to remain 'extracted', got %q", em.ID, em.Status)
		}
	}
}

func TestCombineTransactions_SumsAmountsLocksOverrideTransfersEmailsAndMarksReviewed(t *testing.T) {
	gdb := openDomainTestDB(t)
	acctA := seedTestAccount(t, gdb, "acct-a", "GoPay", "IDR")
	acctB := seedTestAccount(t, gdb, "acct-b", "BCA", "IDR")
	seedTestEmail(t, gdb, "e-food", 1795251000000, "2026-11-21T08:50:00Z")
	seedTestEmail(t, gdb, "e-tip", 1795251060000, "2026-11-21T08:51:00Z")
	seedTestEmail(t, gdb, "e-other", 1795251120000, "2026-11-21T08:52:00Z")

	txFood, _ := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
		EmailID: "e-food", EmailKind: "receipt", AmountMinor: 50000, Currency: "IDR",
		OccurredAtUtc: "2026-11-21T08:50:00Z", MerchantRaw: "GrabFood", Category: "Food", ResolvedAccountID: &acctA,
	})
	txTip, _ := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
		EmailID: "e-tip", EmailKind: "receipt", AmountMinor: 10000, Currency: "IDR",
		OccurredAtUtc: "2026-11-21T08:51:00Z", MerchantRaw: "Grab Tip", Category: "Food", ResolvedAccountID: &acctA,
	})
	txOtherAcct, _ := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
		EmailID: "e-other", EmailKind: "receipt", AmountMinor: 5000, Currency: "IDR",
		OccurredAtUtc: "2026-11-21T08:52:00Z", MerchantRaw: "Other", Category: "Food", ResolvedAccountID: &acctB,
	})

	cat := "Dining Out"
	note := "Food + driver tip"
	if _, err := CombineTransactions(gdb, CombineTransactionsInput{
		TransactionIDs: []string{txFood, txOtherAcct},
		Merchant:       "GrabFood",
		Category:       &cat,
		OccurredAtUtc:  "2026-11-21T08:51:00Z",
	}); err == nil {
		t.Fatalf("expected CombineTransactions to reject mismatched accounts")
	}

	combined, err := CombineTransactions(gdb, CombineTransactionsInput{
		TransactionIDs: []string{txFood, txTip},
		Merchant:       "GrabFood + Tip",
		Category:       &cat,
		HasAccountID:   true,
		AccountID:      &acctA,
		OccurredAtUtc:  "2026-11-21T08:51:00Z",
		HasNote:        true,
		Note:           &note,
	})
	if err != nil {
		t.Fatalf("CombineTransactions: %v", err)
	}
	if combined.AmountMinor != 60000 {
		t.Fatalf("expected summed amount 60000, got %d", combined.AmountMinor)
	}
	if combined.ManualOverride != 1 || combined.Reviewed != 1 {
		t.Fatalf("expected manual_override=1 and reviewed=1, got %d and %d", combined.ManualOverride, combined.Reviewed)
	}
	if combined.SourceCount != 2 {
		t.Fatalf("expected 2 linked sources, got %d", combined.SourceCount)
	}

	RecomputeTransactionFromSources(gdb, combined.ID, NowUtcIsoSeconds())
	detail, err := GetTransactionDetail(gdb, combined.ID)
	if err != nil {
		t.Fatalf("reload combined detail: %v", err)
	}
	if detail.Transaction.AmountMinor != 60000 {
		t.Fatalf("expected combined amount to stay locked at 60000 after recompute, got %d", detail.Transaction.AmountMinor)
	}

	var emails []db.RawEmail
	gdb.Where("id IN ?", []string{"e-food", "e-tip"}).Find(&emails)
	for _, em := range emails {
		if em.Status != "extracted" {
			t.Fatalf("expected email %s to stay 'extracted', got %q", em.ID, em.Status)
		}
	}
}

func TestManualReviewTransitionsAndDateRangePagination(t *testing.T) {
	gdb := openDomainTestDB(t)
	acctA := seedTestAccount(t, gdb, "acct-a", "BCA", "IDR")

	manualTx, err := CreateManualTransaction(gdb, CreateManualTransactionInput{
		AmountMinor:   15000,
		OccurredAtUtc: "2026-11-21T03:00:00Z",
		Merchant:      "Manual Snack",
		AccountID:     &acctA,
	})
	if err != nil {
		t.Fatalf("CreateManualTransaction: %v", err)
	}
	if manualTx.Reviewed != 1 {
		t.Fatalf("expected manual transaction to start reviewed=1, got %d", manualTx.Reviewed)
	}

	txIDs := make([]string, 0, 3)
	times := []string{"2026-11-20T17:30:00Z", "2026-11-21T05:00:00Z", "2026-11-21T16:30:00Z"}
	for i, ts := range times {
		emailID := fmt.Sprintf("em-%d", i)
		seedTestEmail(t, gdb, emailID, 1795200000000+int64(i*1000), ts)
		id, err := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
			EmailID: emailID, EmailKind: "receipt", AmountMinor: int64((i + 1) * 10000), Currency: "IDR",
			OccurredAtUtc: ts, MerchantRaw: fmt.Sprintf("Shop %d", i), Category: "Food", ResolvedAccountID: &acctA,
		})
		if err != nil {
			t.Fatalf("save gmail tx %d: %v", i, err)
		}
		txIDs = append(txIDs, id)
	}

	page1, err := ListTransactions(gdb, ListTransactionsParams{
		StartDate: "2026-11-21",
		EndDate:   "2026-11-21",
		Page:      1,
		PageSize:  2,
	})
	if err != nil {
		t.Fatalf("ListTransactions page 1: %v", err)
	}
	if page1.TotalCount != 4 || len(page1.Items) != 2 {
		t.Fatalf("expected totalCount=4 and 2 items on page 1, got totalCount=%d len=%d", page1.TotalCount, len(page1.Items))
	}
	if page1.TotalInRangeCount != 4 || page1.UnreviewedInRangeCount != 3 {
		t.Fatalf("expected totalInRange=4 unreviewedInRange=3, got %d and %d", page1.TotalInRangeCount, page1.UnreviewedInRangeCount)
	}

	newMerchant := "Shop 0 Edited"
	updated, err := UpdateTransaction(gdb, txIDs[0], UpdateTransactionInput{Merchant: &newMerchant})
	if err != nil {
		t.Fatalf("UpdateTransaction: %v", err)
	}
	if updated.Reviewed != 1 {
		t.Fatalf("expected UpdateTransaction to set reviewed=1, got %d", updated.Reviewed)
	}

	affected, err := MarkTransactionsReviewed(gdb, []string{txIDs[1], txIDs[2]})
	if err != nil {
		t.Fatalf("MarkTransactionsReviewed: %v", err)
	}
	if affected != 2 {
		t.Fatalf("expected 2 rows marked reviewed, got %d", affected)
	}

	afterReview, err := ListTransactions(gdb, ListTransactionsParams{
		StartDate:      "2026-11-21",
		EndDate:        "2026-11-21",
		UnreviewedOnly: true,
	})
	if err != nil {
		t.Fatalf("ListTransactions after review: %v", err)
	}
	if afterReview.TotalCount != 0 || afterReview.TotalInRangeCount != 4 || afterReview.UnreviewedInRangeCount != 0 {
		t.Fatalf("expected totalCount=0 totalInRange=4 unreviewedInRange=0, got %+v", afterReview)
	}

	_, _ = CreateManualTransaction(gdb, CreateManualTransactionInput{
		AmountMinor: 1000, OccurredAtUtc: "2026-11-20T16:59:59Z", Merchant: "Prev Day", AccountID: &acctA,
	})
	_, _ = CreateManualTransaction(gdb, CreateManualTransactionInput{
		AmountMinor: 1000, OccurredAtUtc: "2026-11-21T17:00:00Z", Merchant: "Next Day", AccountID: &acctA,
	})
	page2, err := ListTransactions(gdb, ListTransactionsParams{
		StartDate: "2026-11-21",
		EndDate:   "2026-11-21",
		Page:      2,
		PageSize:  2,
	})
	if err != nil {
		t.Fatalf("ListTransactions page 2: %v", err)
	}
	if page2.TotalCount != 4 || len(page2.Items) != 2 {
		t.Fatalf("expected page 2 to return 2 of 4 in-range items (excluding adjacent days), got total=%d len=%d", page2.TotalCount, len(page2.Items))
	}
}

func TestMergeEligibilityEdgeCases_CurrencyNullAccountAndManualWithGmail(t *testing.T) {
	gdb := openDomainTestDB(t)
	acctA := seedTestAccount(t, gdb, "acct-a", "BCA", "IDR")
	seedTestEmail(t, gdb, "e-idr", 1795251000000, "2026-11-21T08:50:00Z")
	seedTestEmail(t, gdb, "e-usd", 1795251005000, "2026-11-21T08:50:05Z")
	seedTestEmail(t, gdb, "e-nil", 1795251010000, "2026-11-21T08:50:10Z")

	txIDR, _ := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
		EmailID: "e-idr", EmailKind: "card_charge", AmountMinor: 25000, Currency: "IDR",
		OccurredAtUtc: "2026-11-21T08:50:00Z", MerchantRaw: "Store", Category: "Food", ResolvedAccountID: &acctA,
	})
	txUSD, _ := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
		EmailID: "e-usd", EmailKind: "card_charge", AmountMinor: 25000, Currency: "USD",
		OccurredAtUtc: "2026-11-21T08:50:05Z", MerchantRaw: "Store", Category: "Food", ResolvedAccountID: &acctA,
	})
	txNilAcct, _ := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
		EmailID: "e-nil", EmailKind: "receipt", AmountMinor: 25000, Currency: "IDR",
		OccurredAtUtc: "2026-11-21T08:50:10Z", MerchantRaw: "Store", Category: "Food",
	})

	if _, err := DuplicateMergeTransactions(gdb, txIDR, txUSD); err == nil {
		t.Fatalf("expected mismatched currency (IDR vs USD) to be rejected")
	}
	if _, err := DuplicateMergeTransactions(gdb, txIDR, txNilAcct); err == nil {
		t.Fatalf("expected NULL account vs concrete account to be rejected")
	}

	manualSame, err := CreateManualTransaction(gdb, CreateManualTransactionInput{
		AmountMinor: 25000, OccurredAtUtc: "2026-11-21T08:50:00Z", Merchant: "Manual Store", AccountID: &acctA,
	})
	if err != nil {
		t.Fatalf("CreateManualTransaction: %v", err)
	}
	merged, err := DuplicateMergeTransactions(gdb, manualSame.ID, txIDR)
	if err != nil {
		t.Fatalf("expected manual + gmail duplicate merge to succeed: %v", err)
	}
	if merged.SourceCount != 1 {
		t.Fatalf("expected 1 linked email preserved on manual+gmail merge, got %d", merged.SourceCount)
	}
}

func TestDeleteTransactions_DeletesMixedBatchDismissesEmailsAndLeavesOthersIntact(t *testing.T) {
	gdb := openDomainTestDB(t)
	acctA := seedTestAccount(t, gdb, "acct-a", "BCA", "IDR")
	acctB := seedTestAccount(t, gdb, "acct-b", "Visa", "IDR")
	seedTestEmail(t, gdb, "e1", 1795251000000, "2026-11-21T08:50:00Z")
	seedTestEmail(t, gdb, "e2", 1795251005000, "2026-11-21T08:50:05Z")
	seedTestEmail(t, gdb, "e-keep", 1795251010000, "2026-11-21T08:50:10Z")

	tx1, err := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
		EmailID: "e1", EmailKind: "card_charge", AmountMinor: 25000, Currency: "IDR",
		OccurredAtUtc: "2026-11-21T08:50:00Z", MerchantRaw: "Coffee", Category: "Food", ResolvedAccountID: &acctA,
	})
	if err != nil {
		t.Fatalf("save tx1: %v", err)
	}
	tx2, err := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
		EmailID: "e2", EmailKind: "receipt", AmountMinor: 1500, Currency: "USD",
		OccurredAtUtc: "2026-11-21T08:50:05Z", MerchantRaw: "App Store", Category: "Subscriptions", ResolvedAccountID: &acctB,
	})
	if err != nil {
		t.Fatalf("save tx2: %v", err)
	}
	txKeep, err := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
		EmailID: "e-keep", EmailKind: "receipt", AmountMinor: 50000, Currency: "IDR",
		OccurredAtUtc: "2026-11-21T08:50:10Z", MerchantRaw: "Groceries", Category: "Food", ResolvedAccountID: &acctA,
	})
	if err != nil {
		t.Fatalf("save txKeep: %v", err)
	}
	txManual, err := CreateManualTransaction(gdb, CreateManualTransactionInput{
		AmountMinor: 10000, OccurredAtUtc: "2026-11-21T08:55:00Z", Merchant: "Cash Snack",
	})
	if err != nil {
		t.Fatalf("create manual tx: %v", err)
	}

	deleted, err := DeleteTransactions(gdb, []string{tx1, tx2, txManual.ID, " " + tx1 + " "})
	if err != nil {
		t.Fatalf("DeleteTransactions: %v", err)
	}
	if deleted != 3 {
		t.Fatalf("expected 3 deleted transactions, got %d", deleted)
	}

	var remaining []db.Transaction
	gdb.Find(&remaining)
	if len(remaining) != 1 || remaining[0].ID != txKeep {
		t.Fatalf("expected only %s to remain, got %+v", txKeep, remaining)
	}

	var dismissedEmails []db.RawEmail
	gdb.Where("id IN ?", []string{"e1", "e2"}).Find(&dismissedEmails)
	if len(dismissedEmails) != 2 {
		t.Fatalf("expected 2 emails e1/e2, got %d", len(dismissedEmails))
	}
	for _, em := range dismissedEmails {
		if em.Status != "dismissed" {
			t.Fatalf("expected email %s status to be 'dismissed', got %q", em.ID, em.Status)
		}
	}

	var keptEmail db.RawEmail
	if err := gdb.Where("id = ?", "e-keep").First(&keptEmail).Error; err != nil {
		t.Fatalf("load e-keep: %v", err)
	}
	if keptEmail.Status != "extracted" {
		t.Fatalf("expected unselected email e-keep to remain 'extracted', got %q", keptEmail.Status)
	}
}

func TestDeleteTransactions_RejectsEmptyAndRollsBackOnMissingID(t *testing.T) {
	gdb := openDomainTestDB(t)
	seedTestEmail(t, gdb, "e1", 1795251000000, "2026-11-21T08:50:00Z")
	tx1, err := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
		EmailID: "e1", EmailKind: "card_charge", AmountMinor: 25000, Currency: "IDR",
		OccurredAtUtc: "2026-11-21T08:50:00Z", MerchantRaw: "Coffee", Category: "Food",
	})
	if err != nil {
		t.Fatalf("save tx1: %v", err)
	}

	if _, err := DeleteTransactions(gdb, nil); err == nil {
		t.Fatalf("expected error for nil transaction IDs")
	}
	if _, err := DeleteTransactions(gdb, []string{"", "   "}); err == nil {
		t.Fatalf("expected error for blank transaction IDs")
	}
	if _, err := DeleteTransactions(gdb, []string{tx1, "non-existent-id"}); err == nil {
		t.Fatalf("expected error when batch contains a non-existent transaction ID")
	}

	var count int64
	gdb.Model(&db.Transaction{}).Where("id = ?", tx1).Count(&count)
	if count != 1 {
		t.Fatalf("expected %s to remain after rolled-back batch delete, got count=%d", tx1, count)
	}
	var em db.RawEmail
	if err := gdb.Where("id = ?", "e1").First(&em).Error; err != nil {
		t.Fatalf("load e1: %v", err)
	}
	if em.Status != "extracted" {
		t.Fatalf("expected e1 status to remain 'extracted' after rollback, got %q", em.Status)
	}
}

func TestDeleteTransaction_DeletesSingleAndRejectsMissing(t *testing.T) {
	gdb := openDomainTestDB(t)
	seedTestEmail(t, gdb, "e-single", 1795251000000, "2026-11-21T08:50:00Z")
	txID, err := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
		EmailID: "e-single", EmailKind: "card_charge", AmountMinor: 15000, Currency: "IDR",
		OccurredAtUtc: "2026-11-21T08:50:00Z", MerchantRaw: "Bakery", Category: "Food",
	})
	if err != nil {
		t.Fatalf("save tx: %v", err)
	}

	if err := DeleteTransaction(gdb, "missing-id"); err == nil {
		t.Fatalf("expected DeleteTransaction to return error for missing ID")
	}

	if err := DeleteTransaction(gdb, txID); err != nil {
		t.Fatalf("DeleteTransaction: %v", err)
	}

	var count int64
	gdb.Model(&db.Transaction{}).Where("id = ?", txID).Count(&count)
	if count != 0 {
		t.Fatalf("expected transaction %s to be deleted, got count=%d", txID, count)
	}

	var em db.RawEmail
	if err := gdb.Where("id = ?", "e-single").First(&em).Error; err != nil {
		t.Fatalf("load e-single: %v", err)
	}
	if em.Status != "dismissed" {
		t.Fatalf("expected e-single status to be 'dismissed', got %q", em.Status)
	}
}

func TestTransactions_MultiTagCreateUpdateClearAndFilter(t *testing.T) {
	gdb := openDomainTestDB(t)

	tagJapan, err := CreateTag(gdb, CreateTagInput{Name: "Japan"})
	if err != nil {
		t.Fatalf("create tag Japan: %v", err)
	}
	tagWork, err := CreateTag(gdb, CreateTagInput{Name: "Reimbursable"})
	if err != nil {
		t.Fatalf("create tag Reimbursable: %v", err)
	}
	tagSnack, err := CreateTag(gdb, CreateTagInput{Name: "Snacks"})
	if err != nil {
		t.Fatalf("create tag Snacks: %v", err)
	}

	catFood := "Food"
	tx1, err := CreateManualTransaction(gdb, CreateManualTransactionInput{
		AmountMinor:   25000,
		OccurredAtUtc: "2026-11-21T08:00:00Z",
		Merchant:      "Tokyo Ramen",
		Category:      &catFood,
		TagIDs:        []string{tagWork.ID, tagJapan.ID},
	})
	if err != nil {
		t.Fatalf("CreateManualTransaction: %v", err)
	}
	if len(tx1.Tags) != 2 || tx1.Tags[0].Name != "Japan" || tx1.Tags[1].Name != "Reimbursable" {
		t.Fatalf("expected sorted tags [Japan, Reimbursable], got %+v", tx1.Tags)
	}

	tx2, err := CreateManualTransaction(gdb, CreateManualTransactionInput{
		AmountMinor:   15000,
		OccurredAtUtc: "2026-11-21T09:00:00Z",
		Merchant:      "Local Cafe",
		Category:      &catFood,
		TagIDs:        []string{tagSnack.ID},
	})
	if err != nil {
		t.Fatalf("CreateManualTransaction 2: %v", err)
	}

	filtered, err := ListTransactions(gdb, ListTransactionsParams{
		Tag: "Japan",
	})
	if err != nil {
		t.Fatalf("ListTransactions by tag: %v", err)
	}
	if filtered.TotalCount != 1 || filtered.TotalInRangeCount != 2 || len(filtered.Items) != 1 || filtered.Items[0].ID != tx1.ID {
		t.Fatalf("unexpected tag-filtered result: %+v", filtered)
	}
	if len(filtered.Items[0].Tags) != 2 {
		t.Fatalf("expected 2 hydrated tags on list item, got %+v", filtered.Items[0].Tags)
	}

	newMerchant := "Tokyo Ramen Updated"
	unchangedTags, err := UpdateTransaction(gdb, tx1.ID, UpdateTransactionInput{
		Merchant: &newMerchant,
	})
	if err != nil {
		t.Fatalf("UpdateTransaction without HasTagIDs: %v", err)
	}
	if len(unchangedTags.Tags) != 2 {
		t.Fatalf("expected tags unchanged when HasTagIDs=false, got %+v", unchangedTags.Tags)
	}

	replaced, err := UpdateTransaction(gdb, tx1.ID, UpdateTransactionInput{
		HasTagIDs: true,
		TagIDs:    []string{tagSnack.ID},
	})
	if err != nil {
		t.Fatalf("UpdateTransaction replacing tags: %v", err)
	}
	if len(replaced.Tags) != 1 || replaced.Tags[0].ID != tagSnack.ID {
		t.Fatalf("expected tags replaced with [Snacks], got %+v", replaced.Tags)
	}

	cleared, err := UpdateTransaction(gdb, tx2.ID, UpdateTransactionInput{
		HasTagIDs: true,
		TagIDs:    []string{},
	})
	if err != nil {
		t.Fatalf("UpdateTransaction clearing tags: %v", err)
	}
	if len(cleared.Tags) != 0 {
		t.Fatalf("expected empty tags after clear, got %+v", cleared.Tags)
	}
}

func TestTransactions_DuplicateMergeAndCombinePreserveTagUnion(t *testing.T) {
	gdb := openDomainTestDB(t)
	acctA := seedTestAccount(t, gdb, "acct-a", "Card A", "IDR")

	tagJapan, _ := CreateTag(gdb, CreateTagInput{Name: "Japan"})
	tagFood, _ := CreateTag(gdb, CreateTagInput{Name: "Food"})
	tagReimb, _ := CreateTag(gdb, CreateTagInput{Name: "Reimbursable"})

	txA, err := CreateManualTransaction(gdb, CreateManualTransactionInput{
		AmountMinor:   50000,
		OccurredAtUtc: "2026-11-21T08:00:00Z",
		Merchant:      "Izakaya A",
		AccountID:     &acctA,
		TagIDs:        []string{tagJapan.ID, tagFood.ID},
	})
	if err != nil {
		t.Fatalf("create txA: %v", err)
	}
	txB, err := CreateManualTransaction(gdb, CreateManualTransactionInput{
		AmountMinor:   50000,
		OccurredAtUtc: "2026-11-21T08:05:00Z",
		Merchant:      "Izakaya B",
		AccountID:     &acctA,
		TagIDs:        []string{tagJapan.ID, tagReimb.ID},
	})
	if err != nil {
		t.Fatalf("create txB: %v", err)
	}

	dupMerged, err := DuplicateMergeTransactions(gdb, txA.ID, txB.ID)
	if err != nil {
		t.Fatalf("DuplicateMergeTransactions: %v", err)
	}
	if len(dupMerged.Tags) != 3 || dupMerged.Tags[0].Name != "Food" || dupMerged.Tags[1].Name != "Japan" || dupMerged.Tags[2].Name != "Reimbursable" {
		t.Fatalf("expected union tags [Food, Japan, Reimbursable] on duplicate merge, got %+v", dupMerged.Tags)
	}

	txC, err := CreateManualTransaction(gdb, CreateManualTransactionInput{
		AmountMinor:   15000,
		OccurredAtUtc: "2026-11-21T09:00:00Z",
		Merchant:      "Dessert",
		AccountID:     &acctA,
		TagIDs:        []string{tagFood.ID},
	})
	txD, err2 := CreateManualTransaction(gdb, CreateManualTransactionInput{
		AmountMinor:   20000,
		OccurredAtUtc: "2026-11-21T09:05:00Z",
		Merchant:      "Tea",
		AccountID:     &acctA,
		TagIDs:        []string{tagJapan.ID, tagReimb.ID},
	})
	if err != nil || err2 != nil {
		t.Fatalf("create txC/txD: %v %v", err, err2)
	}
	cat := "Dining Out"
	combined, err := CombineTransactions(gdb, CombineTransactionsInput{
		TransactionIDs: []string{txC.ID, txD.ID},
		Merchant:       "Izakaya Evening",
		Category:       &cat,
		OccurredAtUtc:  "2026-11-21T09:05:00Z",
	})
	if err != nil {
		t.Fatalf("CombineTransactions: %v", err)
	}
	if len(combined.Tags) != 3 || combined.Tags[0].Name != "Food" || combined.Tags[1].Name != "Japan" || combined.Tags[2].Name != "Reimbursable" {
		t.Fatalf("expected union tags [Food, Japan, Reimbursable] on combine, got %+v", combined.Tags)
	}
}
