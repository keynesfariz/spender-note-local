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

	// Both unassigned (nil account_id) should be eligible.
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

	cat := "Food & Dining"
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

	// RecomputeTransactionFromSources must not overwrite the locked summed amount.
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

	// Manual transaction starts reviewed=1 (2026-11-21 10:00 Asia/Jakarta -> 03:00Z).
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

	// Seed 3 unreviewed Gmail transactions on 2026-11-21 (Asia/Jakarta, UTC+7).
	// Note 2026-11-20T17:30:00Z is 2026-11-21 00:30 Asia/Jakarta!
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

	// Page 1 (pageSize=2) of 2026-11-21 in Asia/Jakarta should include all 4 transactions.
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

	// Editing txIDs[0] auto-marks it reviewed=1.
	newMerchant := "Shop 0 Edited"
	updated, err := UpdateTransaction(gdb, txIDs[0], UpdateTransactionInput{Merchant: &newMerchant})
	if err != nil {
		t.Fatalf("UpdateTransaction: %v", err)
	}
	if updated.Reviewed != 1 {
		t.Fatalf("expected UpdateTransaction to set reviewed=1, got %d", updated.Reviewed)
	}

	// Bulk mark remaining 2 reviewed.
	affected, err := MarkTransactionsReviewed(gdb, []string{txIDs[1], txIDs[2]})
	if err != nil {
		t.Fatalf("MarkTransactionsReviewed: %v", err)
	}
	if affected != 2 {
		t.Fatalf("expected 2 rows marked reviewed, got %d", affected)
	}

	// UnreviewedOnly filter now returns 0 items, while TotalInRangeCount=4 and UnreviewedInRangeCount=0.
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
}
