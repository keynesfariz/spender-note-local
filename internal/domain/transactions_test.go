package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gorm.io/gorm"

	"portspendernote/internal/db"
)

func TestUpdateTransaction_RespectsExplicitCurrencyConversion(t *testing.T) {
	disabled, enabled := false, true
	for _, tc := range []struct {
		name     string
		convert  *bool
		currency string
	}{
		{name: "explicitly disabled", convert: &disabled, currency: "USD"},
		{name: "explicitly enabled", convert: &enabled, currency: "IDR"},
		{name: "omitted for legacy callers", currency: "IDR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gdb := openDomainTestDB(t)
			configureTestLocale(t, gdb, "Asia/Jakarta", "IDR")
			seedTestEmail(t, gdb, "foreign-email", 1795251000000, "2026-11-21T08:50:00Z")
			txID, err := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
				EmailID: "foreign-email", EmailKind: "receipt", AmountMinor: 1200, Currency: "USD",
				OccurredAtUtc: "2026-11-21T08:50:00Z", MerchantRaw: "Coffee", Category: "Food",
			})
			if err != nil {
				t.Fatal(err)
			}
			amount := int64(1235)
			item, err := UpdateTransaction(gdb, txID, UpdateTransactionInput{
				AmountMinor:            &amount,
				ConvertToLocalCurrency: tc.convert,
			})
			if err != nil {
				t.Fatal(err)
			}
			if item.Currency != tc.currency || item.AmountMinor != amount {
				t.Fatalf("updated amount = %d %s, want %d %s", item.AmountMinor, item.Currency, amount, tc.currency)
			}
		})
	}
}

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

func seedRichSourceEmail(
	t *testing.T,
	gdb *gorm.DB,
	txID, emailID, senderID, role, emailKind string,
	internalMs int64,
	receivedAtUtc, fromAddr, subject, cleanedText, rawJSON string,
) {
	t.Helper()
	var sPtr *string
	if senderID != "" {
		sPtr = &senderID
	}
	if err := gdb.Create(&db.RawEmail{
		ID:             emailID,
		GmailMessageID: "gmail-" + emailID,
		SenderID:       sPtr,
		FromAddress:    fromAddr,
		Subject:        subject,
		CleanedText:    cleanedText,
		InternalDateMs: internalMs,
		ReceivedAtUtc:  receivedAtUtc,
		Status:         "extracted",
	}).Error; err != nil {
		t.Fatalf("seed raw_email %s: %v", emailID, err)
	}
	amt := int64(50000)
	cur := "IDR"
	if err := gdb.Create(&db.EmailExtraction{
		ID:              "ext-" + emailID,
		EmailID:         emailID,
		EmailKind:       emailKind,
		AmountMinor:     &amt,
		Currency:        &cur,
		OccurredAtUtc:   &receivedAtUtc,
		ExtractionModel: "sync",
		RawLlmJSON:      rawJSON,
	}).Error; err != nil {
		t.Fatalf("seed email_extraction %s: %v", emailID, err)
	}
	if err := gdb.Create(&db.TransactionSource{
		ID:            "src-" + emailID,
		TransactionID: txID,
		EmailID:       emailID,
		Role:          role,
		MatchScore:    1.0,
		MatchState:    "confirmed",
	}).Error; err != nil {
		t.Fatalf("seed transaction_source %s: %v", emailID, err)
	}
}

func startEnrichTestServer(t *testing.T, gdb *gorm.DB, handler func(userPrompt string) map[string]any) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var userPrompt string
		for _, m := range body.Messages {
			if m.Role == "user" {
				userPrompt = m.Content
			}
		}
		payload := handler(userPrompt)
		raw, _ := json.Marshal(payload)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": string(raw)}},
			},
		})
	}))
	t.Cleanup(srv.Close)

	model := "test-enrich-model"
	if err := gdb.Model(&db.AppSetting{}).Where("id = 1").Updates(map[string]any{
		"llm_base_url":   srv.URL + "/v1",
		"llm_model":      &model,
		"llm_timeout_ms": 5000,
	}).Error; err != nil {
		t.Fatalf("update app_settings for test llm: %v", err)
	}
}

func TestEnrichTransactionsFromSources_SynthesizesMultipleEmailsWithoutMutatingDB(t *testing.T) {
	gdb := openDomainTestDB(t)
	tagWeekend, err := CreateTag(gdb, CreateTagInput{Name: "Weekend"})
	if err != nil {
		t.Fatalf("create Weekend tag: %v", err)
	}
	if _, err := CreateTag(gdb, CreateTagInput{Name: "Work"}); err != nil {
		t.Fatalf("create Work tag: %v", err)
	}

	txID := "tx-food-4"
	if err := gdb.Create(&db.Transaction{
		ID:            txID,
		SourceType:    "gmail",
		AmountMinor:   75000,
		Currency:      "IDR",
		OccurredAtUtc: "2026-11-23T05:14:00Z",
		Merchant:      "GOPAY*GOFOOD",
		CategoryID:    "cat-food",
		CreatedAtUtc:  "2026-11-23T05:14:00Z",
		UpdatedAtUtc:  "2026-11-23T05:14:00Z",
	}).Error; err != nil {
		t.Fatalf("seed transaction: %v", err)
	}

	seedRichSourceEmail(t, gdb, txID, "em-1", "", "card_charge", "card_charge", 1000, "2026-11-23T05:14:00Z", "alerts@bank.id", "Card charge IDR 75,000", "GOPAY*GOFOOD charge", `{"email_kind":"card_charge","transaction_date":"2026-11-23","transaction_time":"12:14"}`)
	seedRichSourceEmail(t, gdb, txID, "em-2", "", "receipt", "receipt", 2000, "2026-11-23T05:15:00Z", "wallet@gopay.co.id", "GoPay payment", "Paid Rp75.000", `{}`)
	seedRichSourceEmail(t, gdb, txID, "em-3", "", "receipt", "receipt", 3000, "2026-11-23T05:16:00Z", "orders@gofood.co.id", "GoFood order confirmed", "Order confirmed for Sate Khas Senayan", `{}`)
	seedRichSourceEmail(t, gdb, txID, "em-4", "", "receipt", "receipt", 4000, "2026-11-23T05:48:00Z", "receipts@gofood.co.id", "Order Receipt", "Sate Khas Senayan: 10x Sate Ayam, 1x Nasi Putih, 1x Es Teh Manis", `{"email_kind":"receipt","transaction_date":"2026-11-23","transaction_time":"12:48"}`)

	for _, name := range []string{"historical-user", "historical-magic", "historical-category-edit"} {
		if err := gdb.Create(&db.Transaction{
			ID: name, SourceType: "gmail", AmountMinor: 1000, Currency: "IDR",
			Merchant: "GoFood", CategoryID: "cat-food", ManualOverride: 1,
			OccurredAtUtc: "2026-11-22T05:00:00Z", CreatedAtUtc: "2026-11-22T05:00:00Z", UpdatedAtUtc: "2026-11-22T05:00:00Z",
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	oldRemark := "GoFood: Previous Restaurant"
	saved, err := UpdateTransaction(gdb, "historical-user", UpdateTransactionInput{Remark: &oldRemark})
	if err != nil || !saved.RemarkUserEdited || saved.Merchant != "GoFood" {
		t.Fatalf("saving a remark should record its provenance and preserve the merchant: %+v, %v", saved, err)
	}
	fallbackTitle := "GoFood"
	unchanged, err := UpdateTransaction(gdb, "historical-category-edit", UpdateTransactionInput{Remark: &fallbackTitle})
	if err != nil || unchanged.Remark != nil || unchanged.RemarkUserEdited {
		t.Fatalf("saving the unchanged imported title should not create a remark example: %+v, %v", unchanged, err)
	}
	normalizedMerchant, generated := "Actual Restaurant", false
	normalized, err := UpdateTransaction(gdb, "historical-category-edit", UpdateTransactionInput{
		Merchant: &normalizedMerchant, Remark: &fallbackTitle, RemarkUserEdited: &generated,
	})
	if err != nil || normalized.Remark == nil || *normalized.Remark != fallbackTitle ||
		normalized.RemarkUserEdited || normalized.Merchant != normalizedMerchant {
		t.Fatalf("normalizing the merchant should retain the explicit displayed title: %+v, %v", normalized, err)
	}
	magicRemark, magicEdited := "Magic title", false
	if _, err := UpdateTransaction(gdb, "historical-magic", UpdateTransactionInput{Remark: &magicRemark, RemarkUserEdited: &magicEdited}); err != nil {
		t.Fatal(err)
	}

	var capturedPrompt string
	startEnrichTestServer(t, gdb, func(userPrompt string) map[string]any {
		capturedPrompt = userPrompt
		return map[string]any{
			"merchant":         "Sate Khas Senayan",
			"remark":           "GoFood: [vendor]",
			"category":         "Street Food",
			"tags":             []string{"weekend", "GoFood"},
			"note":             "10x Sate Ayam, Nasi Putih, Es Teh Manis via GoFood",
			"transaction_date": nil,
			"transaction_time": nil,
			"timezone_offset":  nil,
		}
	})

	preview, err := EnrichTransactionsFromSources(gdb, context.Background(), []string{txID})
	if err != nil {
		t.Fatalf("EnrichTransactionsFromSources failed: %v", err)
	}
	for _, want := range []string{"GOPAY*GOFOOD charge", "GoPay payment", "GoFood order confirmed", "10x Sate Ayam", "Weekend, Work", "Dining Out", `Saved-Remark-Examples: ["GoFood: [vendor]"]`} {
		if !strings.Contains(capturedPrompt, want) {
			t.Fatalf("expected enrichment prompt to include %q, got:\n%s", want, capturedPrompt)
		}
	}
	if strings.Contains(capturedPrompt, "Previous Restaurant") || strings.Contains(capturedPrompt, "Magic title") {
		t.Fatalf("prompt contains a historical restaurant or an unedited Magic suggestion: %s", capturedPrompt)
	}
	if preview.Remark != "GoFood: Sate Khas Senayan" {
		t.Fatalf("expected the suggested remark, got %q", preview.Remark)
	}
	if preview.Merchant != "Sate Khas Senayan" {
		t.Fatalf("expected merchant Sate Khas Senayan, got %q", preview.Merchant)
	}
	if preview.Category != "Other" {
		t.Fatalf("expected unknown category coerced to Other, got %q", preview.Category)
	}
	if preview.Note == nil || *preview.Note != "10x Sate Ayam, Nasi Putih, Es Teh Manis via GoFood" {
		t.Fatalf("unexpected note: %v", preview.Note)
	}
	if preview.OccurredAtUtc == nil || *preview.OccurredAtUtc != "2026-11-23T05:14:00Z" {
		t.Fatalf("expected card_charge timestamp 2026-11-23T05:14:00Z, got %v", preview.OccurredAtUtc)
	}
	if len(preview.ExistingTags) != 1 || preview.ExistingTags[0].ID != tagWeekend.ID || preview.ExistingTags[0].Name != "Weekend" {
		t.Fatalf("expected matched existing tag [Weekend], got %+v", preview.ExistingTags)
	}
	if len(preview.DraftTagNames) != 1 || preview.DraftTagNames[0] != "GoFood" {
		t.Fatalf("expected draft tag [GoFood], got %v", preview.DraftTagNames)
	}

	tagsAfter, _ := ListTags(gdb)
	if len(tagsAfter) != 2 {
		t.Fatalf("expected preview not to insert tags in DB, got %d tags", len(tagsAfter))
	}
	var persisted db.Transaction
	gdb.Where("id = ?", txID).First(&persisted)
	if persisted.Merchant != "GOPAY*GOFOOD" || persisted.CategoryID != "cat-food" || persisted.Note != nil || persisted.Remark != nil {
		t.Fatalf("expected transaction row untouched by preview, got %+v", persisted)
	}
}

func TestEnrichTransactionsFromSources_UsesOnlyServiceWhenVendorIsMissing(t *testing.T) {
	gdb := openDomainTestDB(t)
	stamp := "2026-11-23T05:14:00Z"
	for _, id := range []string{"previous-order", "service-only-order"} {
		if err := gdb.Create(&db.Transaction{
			ID: id, SourceType: "gmail", AmountMinor: 45000, Currency: "IDR",
			Merchant: "GrabFood", CategoryID: "cat-food", OccurredAtUtc: stamp,
			CreatedAtUtc: stamp, UpdatedAtUtc: stamp,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	correction := "GrabFood: Previous Restaurant"
	if _, err := UpdateTransaction(gdb, "previous-order", UpdateTransactionInput{Remark: &correction}); err != nil {
		t.Fatal(err)
	}
	seedRichSourceEmail(t, gdb, "service-only-order", "payment-email", "", "receipt", "receipt",
		1000, stamp, "payments@grabfood.example", "GrabFood payment", "Your GrabFood payment was successful.", `{}`)

	for _, tc := range []struct {
		name     string
		merchant string
		remark   string
	}{
		{"copied placeholder", "GrabFood", "GrabFood: [vendor]"},
		{"unsupported vendor", "Previous Restaurant", "GrabFood: Previous Restaurant"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			startEnrichTestServer(t, gdb, func(prompt string) map[string]any {
				return map[string]any{"merchant": tc.merchant, "remark": tc.remark, "category": "Food"}
			})

			preview, err := EnrichTransactionsFromSources(gdb, context.Background(), []string{"service-only-order"})
			if err != nil {
				t.Fatal(err)
			}
			if preview.Remark != "GrabFood" {
				t.Errorf("remark = %q, want service alone when the linked email omits the vendor", preview.Remark)
			}
		})
	}
}

func TestEnrichTransactionsFromSources_CardChargeTimestampAndSenderRoleHintPrecedence(t *testing.T) {
	gdb := openDomainTestDB(t)
	s1 := db.Sender{ID: "snd-1", Email: "bank@test.id", RoleHint: "auto", Enabled: 1, BackfillFromUtc: "2026-11-01T00:00:00Z"}
	s2 := db.Sender{ID: "snd-2", Email: "shop@test.id", RoleHint: "auto", Enabled: 1, BackfillFromUtc: "2026-11-01T00:00:00Z"}
	if err := gdb.Create(&s1).Error; err != nil {
		t.Fatalf("create s1: %v", err)
	}
	if err := gdb.Create(&s2).Error; err != nil {
		t.Fatalf("create s2: %v", err)
	}

	txID := "tx-role-hint"
	if err := gdb.Create(&db.Transaction{
		ID: txID, SourceType: "gmail", AmountMinor: 50000, Currency: "IDR",
		OccurredAtUtc: "2026-11-23T05:14:00Z", Merchant: "MERCHANT", CategoryID: "cat-food",
		CreatedAtUtc: "2026-11-23T05:14:00Z", UpdatedAtUtc: "2026-11-23T05:14:00Z",
	}).Error; err != nil {
		t.Fatalf("create tx: %v", err)
	}

	seedRichSourceEmail(t, gdb, txID, "er-1", s1.ID, "card_charge", "card_charge", 1000, "2026-11-23T05:14:00Z", s1.Email, "Alert", "Charge at 12:14", `{"transaction_date":"2026-11-23","transaction_time":"12:14","timezone_offset":null}`)
	seedRichSourceEmail(t, gdb, txID, "er-2", s2.ID, "receipt", "receipt", 2000, "2026-11-23T05:48:00Z", s2.Email, "Receipt", "Delivered at 12:48", `{"transaction_date":"2026-11-23","transaction_time":"12:48","timezone_offset":null}`)

	startEnrichTestServer(t, gdb, func(userPrompt string) map[string]any {
		return map[string]any{
			"merchant":         "Bakery",
			"category":         "Food",
			"tags":             []string{},
			"note":             nil,
			"transaction_date": "2026-11-23",
			"transaction_time": "13:30",
			"timezone_offset":  nil,
		}
	})

	p1, err := EnrichTransactionsFromSources(gdb, context.Background(), []string{txID})
	if err != nil {
		t.Fatalf("first enrich: %v", err)
	}
	if p1.OccurredAtUtc == nil || *p1.OccurredAtUtc != "2026-11-23T05:14:00Z" {
		t.Fatalf("expected 12:14 Jakarta (05:14Z) from card_charge role, got %v", p1.OccurredAtUtc)
	}

	gdb.Model(&db.Sender{}).Where("id = ?", s1.ID).Update("role_hint", "receipt")
	gdb.Model(&db.Sender{}).Where("id = ?", s2.ID).Update("role_hint", "card_charge")

	p2, err := EnrichTransactionsFromSources(gdb, context.Background(), []string{txID})
	if err != nil {
		t.Fatalf("second enrich: %v", err)
	}
	if p2.OccurredAtUtc == nil || *p2.OccurredAtUtc != "2026-11-23T05:48:00Z" {
		t.Fatalf("expected sender role_hint to swap priority to 12:48 Jakarta (05:48Z), got %v", p2.OccurredAtUtc)
	}
}

func TestEnrichTransactionsFromSources_MultipleTransactionsEarliestCardCharge(t *testing.T) {
	gdb := openDomainTestDB(t)
	// Include source-free transactions to check that the whole selection
	// remains eligible even when only a few transactions contribute emails.
	ids := []string{"tx-a", "tx-b"}
	for i := 0; i < 49; i++ {
		ids = append(ids, fmt.Sprintf("tx-other-%d", i))
	}
	for _, id := range ids {
		if err := gdb.Create(&db.Transaction{
			ID: id, SourceType: "gmail", AmountMinor: 25000, Currency: "IDR",
			OccurredAtUtc: "2026-11-23T08:00:00Z", Merchant: id, CategoryID: "cat-food",
			CreatedAtUtc: "2026-11-23T08:00:00Z", UpdatedAtUtc: "2026-11-23T08:00:00Z",
		}).Error; err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	seedRichSourceEmail(t, gdb, "tx-a", "em-a", "", "card_charge", "card_charge", 1000, "2026-11-23T07:30:00Z", "bank@test.id", "Later Charge", "Body A", `{"transaction_date":"2026-11-23","transaction_time":"14:30"}`)
	seedRichSourceEmail(t, gdb, "tx-b", "em-b", "", "card_charge", "card_charge", 2000, "2026-11-23T04:05:00Z", "bank@test.id", "Earlier Charge", "Body B", `{"transaction_date":"2026-11-23","transaction_time":"11:05"}`)

	var capturedPrompt string
	startEnrichTestServer(t, gdb, func(userPrompt string) map[string]any {
		capturedPrompt = userPrompt
		return map[string]any{
			"merchant":         "Merged Kitchen",
			"category":         "Dining Out",
			"tags":             []string{},
			"note":             "Order + tip",
			"transaction_date": "2026-11-23",
			"transaction_time": "14:45",
			"timezone_offset":  nil,
		}
	})

	preview, err := EnrichTransactionsFromSources(gdb, context.Background(), ids)
	if err != nil {
		t.Fatalf("multi-tx enrich failed: %v", err)
	}
	if !strings.Contains(capturedPrompt, "Body A") || !strings.Contains(capturedPrompt, "Body B") {
		t.Fatalf("expected both transactions' emails in prompt, got:\n%s", capturedPrompt)
	}
	if preview.OccurredAtUtc == nil || *preview.OccurredAtUtc != "2026-11-23T04:05:00Z" {
		t.Fatalf("expected earliest card_charge timestamp 04:05:00Z, got %v", preview.OccurredAtUtc)
	}
}

func TestEnrichTransactionsFromSources_MissingCardChargeTimeFallsBackToLlmOrNil(t *testing.T) {
	gdb := openDomainTestDB(t)
	txID := "tx-fallback-ts"
	if err := gdb.Create(&db.Transaction{
		ID: txID, SourceType: "gmail", AmountMinor: 40000, Currency: "IDR",
		OccurredAtUtc: "2026-11-23T05:00:00Z", Merchant: "Cafe", CategoryID: "cat-food",
		CreatedAtUtc: "2026-11-23T05:00:00Z", UpdatedAtUtc: "2026-11-23T05:00:00Z",
	}).Error; err != nil {
		t.Fatalf("create tx: %v", err)
	}
	seedRichSourceEmail(t, gdb, txID, "em-notime", "", "card_charge", "card_charge", 1000, "2026-11-23T05:00:00Z", "bank@test.id", "Charge", "No clock time in raw json", `{"transaction_date":"2026-11-23","transaction_time":null}`)
	seedRichSourceEmail(t, gdb, txID, "em-rcpt", "", "receipt", "receipt", 2000, "2026-11-23T12:20:00Z", "shop@test.id", "Receipt", "Paid at 19:15", `{"transaction_date":"2026-11-23","transaction_time":"19:15"}`)

	var llmDate, llmTime any = "2026-11-23", "19:15"
	startEnrichTestServer(t, gdb, func(string) map[string]any {
		return map[string]any{
			"merchant":         "Cafe Cozy",
			"category":         "Dining Out",
			"tags":             []string{},
			"note":             nil,
			"transaction_date": llmDate,
			"transaction_time": llmTime,
			"timezone_offset":  nil,
		}
	})

	withLlmTime, err := EnrichTransactionsFromSources(gdb, context.Background(), []string{txID})
	if err != nil {
		t.Fatalf("fallback enrich: %v", err)
	}
	if withLlmTime.OccurredAtUtc == nil || *withLlmTime.OccurredAtUtc != "2026-11-23T12:15:00Z" {
		t.Fatalf("expected fallback to LLM time 12:15:00Z rather than card_charge sync fallback 05:00:00Z, got %v", withLlmTime.OccurredAtUtc)
	}

	llmDate, llmTime = "2026-11-23", nil
	withNilTime, err := EnrichTransactionsFromSources(gdb, context.Background(), []string{txID})
	if err != nil {
		t.Fatalf("nil-time enrich: %v", err)
	}
	if withNilTime.OccurredAtUtc != nil {
		t.Fatalf("expected nil OccurredAtUtc when neither card_charge nor LLM has clock time, got %q", *withNilTime.OccurredAtUtc)
	}
}

func TestEnrichTransactionsFromSources_ValidationErrors(t *testing.T) {
	gdb := openDomainTestDB(t)
	manualTx, err := CreateManualTransaction(gdb, CreateManualTransactionInput{
		AmountMinor: 10000, OccurredAtUtc: "2026-11-23T05:00:00Z", Merchant: "Cash",
	})
	if err != nil {
		t.Fatalf("CreateManualTransaction: %v", err)
	}

	assertValidationErr := func(err error, desc string) {
		t.Helper()
		var ve *ValidationError
		if err == nil || !errors.As(err, &ve) {
			t.Fatalf("%s: expected ValidationError, got %T: %v", desc, err, err)
		}
	}

	_, err = EnrichTransactionsFromSources(gdb, context.Background(), []string{})
	assertValidationErr(err, "empty transaction IDs")

	_, err = EnrichTransactionsFromSources(gdb, context.Background(), []string{"does-not-exist"})
	assertValidationErr(err, "missing transaction")

	_, err = EnrichTransactionsFromSources(gdb, context.Background(), []string{manualTx.ID})
	assertValidationErr(err, "no model configured / zero emails")

	model := "qwen2.5"
	gdb.Model(&db.AppSetting{}).Where("id = 1").Update("llm_model", &model)
	_, err = EnrichTransactionsFromSources(gdb, context.Background(), []string{manualTx.ID})
	assertValidationErr(err, "manual tx with zero emails")
}

func TestUpdateAndCombineTransactions_AtomicTagNamesPersistence(t *testing.T) {
	gdb := openDomainTestDB(t)
	acctA := seedTestAccount(t, gdb, "acct-a", "GoPay", "IDR")
	tagWeekend, _ := CreateTag(gdb, CreateTagInput{Name: "Weekend"})
	tagWork, _ := CreateTag(gdb, CreateTagInput{Name: "Work"})

	tx1, err := CreateManualTransaction(gdb, CreateManualTransactionInput{
		AmountMinor: 20000, OccurredAtUtc: "2026-11-23T05:00:00Z", Merchant: "Lunch", AccountID: &acctA, TagIDs: []string{tagWork.ID},
	})
	if err != nil {
		t.Fatalf("create tx1: %v", err)
	}

	updated, err := UpdateTransaction(gdb, tx1.ID, UpdateTransactionInput{
		HasTagIDs: true,
		TagIDs:    []string{tagWork.ID},
		TagNames:  []string{"GoFood", "weekend", " GoFood "},
	})
	if err != nil {
		t.Fatalf("UpdateTransaction with TagNames: %v", err)
	}
	if len(updated.Tags) != 3 || updated.Tags[0].Name != "GoFood" || updated.Tags[1].Name != "Weekend" || updated.Tags[2].Name != "Work" {
		t.Fatalf("expected [GoFood, Weekend, Work], got %+v", updated.Tags)
	}
	allTags, _ := ListTags(gdb)
	if len(allTags) != 3 {
		t.Fatalf("expected 3 total tags without duplicates, got %d", len(allTags))
	}

	badCat := "NoSuchCategory"
	if _, err := UpdateTransaction(gdb, tx1.ID, UpdateTransactionInput{
		Category:  &badCat,
		HasTagIDs: true,
		TagNames:  []string{"RollbackTag"},
	}); err == nil {
		t.Fatalf("expected UpdateTransaction to fail for invalid category")
	}
	allTagsAfterFail, _ := ListTags(gdb)
	if len(allTagsAfterFail) != 3 {
		t.Fatalf("expected RollbackTag not to persist on transaction failure, got %d tags", len(allTagsAfterFail))
	}

	changedMerchant := "Changed Merchant"
	if _, err := UpdateTransaction(gdb, tx1.ID, UpdateTransactionInput{
		Merchant:  &changedMerchant,
		HasTagIDs: true,
		TagIDs:    []string{"missing-tag-id"},
		TagNames:  []string{"RollbackTag"},
	}); err == nil {
		t.Fatalf("expected UpdateTransaction to fail for unknown tag id")
	}
	allTagsAfterTagFail, _ := ListTags(gdb)
	if len(allTagsAfterTagFail) != 3 {
		t.Fatalf("expected created RollbackTag to roll back after tag id failure, got %d tags", len(allTagsAfterTagFail))
	}
	var afterTagFail db.Transaction
	if err := gdb.Where("id = ?", tx1.ID).First(&afterTagFail).Error; err != nil {
		t.Fatalf("reload tx1: %v", err)
	}
	if afterTagFail.Merchant != "Lunch" {
		t.Fatalf("expected merchant update to roll back, got %q", afterTagFail.Merchant)
	}

	namesOnly, err := UpdateTransaction(gdb, tx1.ID, UpdateTransactionInput{TagNames: []string{"NamesOnly"}})
	if err != nil {
		t.Fatalf("UpdateTransaction with tag_names only: %v", err)
	}
	hasWork := false
	for _, tg := range namesOnly.Tags {
		if tg.ID == tagWork.ID {
			hasWork = true
		}
	}
	if !hasWork || len(namesOnly.Tags) < 4 {
		t.Fatalf("expected tag_names-only update to preserve existing tags and add NamesOnly, got %+v", namesOnly.Tags)
	}

	tx2, _ := CreateManualTransaction(gdb, CreateManualTransactionInput{
		AmountMinor: 10000, OccurredAtUtc: "2026-11-23T06:00:00Z", Merchant: "Drink", AccountID: &acctA, TagIDs: []string{tagWeekend.ID},
	})
	combined, err := CombineTransactions(gdb, CombineTransactionsInput{
		TransactionIDs: []string{tx1.ID, tx2.ID},
		Merchant:       "Lunch + Drink",
		OccurredAtUtc:  "2026-11-23T05:00:00Z",
		HasTagIDs:      true,
		TagIDs:         []string{tagWork.ID},
		TagNames:       []string{"SplitBill", "weekend"},
	})
	if err != nil {
		t.Fatalf("CombineTransactions with TagNames: %v", err)
	}
	if len(combined.Tags) != 3 || combined.Tags[0].Name != "SplitBill" || combined.Tags[1].Name != "Weekend" || combined.Tags[2].Name != "Work" {
		t.Fatalf("expected combined tags [SplitBill, Weekend, Work], got %+v", combined.Tags)
	}
}
