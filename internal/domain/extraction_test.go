package domain

import (
	"testing"

	"gorm.io/gorm"

	"portspendernote/internal/db"
)

func openDomainTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gdb, err := db.OpenDatabase(":memory:", false)
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	tz := "Asia/Jakarta"
	cur := "IDR"
	if err := gdb.Model(&db.AppSetting{}).Where("id = 1").Updates(map[string]any{
		"country_code": "ID",
		"timezone":     tz,
		"currency":     cur,
	}).Error; err != nil {
		t.Fatalf("configure app_settings: %v", err)
	}
	return gdb
}

func seedTestEmail(t *testing.T, gdb *gorm.DB, id string, internalDateMs int64, receivedAtUtc string) {
	t.Helper()
	if err := gdb.Create(&db.RawEmail{
		ID:             id,
		GmailMessageID: "gmail-" + id,
		FromAddress:    "alerts@bank.test",
		Subject:        "Transaction alert " + id,
		CleanedText:    "Paid merchant",
		InternalDateMs: internalDateMs,
		ReceivedAtUtc:  receivedAtUtc,
		Status:         "extracted",
	}).Error; err != nil {
		t.Fatalf("seed raw_email %s: %v", id, err)
	}
}

func seedTestAccount(t *testing.T, gdb *gorm.DB, id, name, currency string) string {
	t.Helper()
	if err := gdb.Create(&db.Account{
		ID:                    id,
		Name:                  name,
		Type:                  "credit_card",
		Currency:              currency,
		InitialBalanceAsOfUtc: "2026-01-01T00:00:00Z",
		Status:                "confirmed",
		CreatedAtUtc:          "2026-01-01T00:00:00Z",
	}).Error; err != nil {
		t.Fatalf("seed account %s: %v", id, err)
	}
	return id
}

func TestSaveExtractedTransaction_NoAutoDedupAndStartsUnreviewed(t *testing.T) {
	gdb := openDomainTestDB(t)
	acctID := seedTestAccount(t, gdb, "acct-1", "BCA Card", "IDR")
	seedTestEmail(t, gdb, "email-1", 1795251000000, "2026-11-21T08:50:00Z")
	seedTestEmail(t, gdb, "email-2", 1795251005000, "2026-11-21T08:50:05Z")

	ref := "REF-123"
	tx1ID, err := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
		EmailID:           "email-1",
		EmailKind:         "card_charge",
		AmountMinor:       50000,
		Currency:          "IDR",
		OccurredAtUtc:     "2026-11-21T08:50:00Z",
		MerchantRaw:       "GrabFood",
		Category:          "Food",
		ReferenceCode:     &ref,
		ResolvedAccountID: &acctID,
	})
	if err != nil {
		t.Fatalf("SaveExtractedTransaction 1: %v", err)
	}

	tx2ID, err := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
		EmailID:           "email-2",
		EmailKind:         "receipt",
		AmountMinor:       50000,
		Currency:          "IDR",
		OccurredAtUtc:     "2026-11-21T08:50:05Z",
		MerchantRaw:       "GrabFood",
		Category:          "Food",
		ReferenceCode:     &ref,
		ResolvedAccountID: &acctID,
	})
	if err != nil {
		t.Fatalf("SaveExtractedTransaction 2: %v", err)
	}

	if tx1ID == tx2ID {
		t.Fatalf("expected two distinct transactions with auto-dedup removed, got identical ID %s", tx1ID)
	}

	var txs []db.Transaction
	if err := gdb.Find(&txs).Error; err != nil {
		t.Fatalf("list transactions: %v", err)
	}
	if len(txs) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(txs))
	}
	for _, tx := range txs {
		if tx.Reviewed != 0 {
			t.Fatalf("expected newly synced transaction %s to have reviewed=0, got %d", tx.ID, tx.Reviewed)
		}
	}
}

func TestSplitTransactionSource_DetachedTransactionStartsUnreviewedWithoutRejectedMerges(t *testing.T) {
	gdb := openDomainTestDB(t)
	acctID := seedTestAccount(t, gdb, "acct-1", "BCA Card", "IDR")
	seedTestEmail(t, gdb, "email-1", 1795251000000, "2026-11-21T08:50:00Z")
	seedTestEmail(t, gdb, "email-2", 1795251060000, "2026-11-21T08:51:00Z")

	txID, err := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
		EmailID:           "email-1",
		EmailKind:         "card_charge",
		AmountMinor:       50000,
		Currency:          "IDR",
		OccurredAtUtc:     "2026-11-21T08:50:00Z",
		MerchantRaw:       "GrabFood",
		Category:          "Food",
		ResolvedAccountID: &acctID,
	})
	if err != nil {
		t.Fatalf("save first email: %v", err)
	}
	if err := gdb.Exec(`
		INSERT INTO email_extractions (id, email_id, email_kind, amount_minor, currency, occurred_at_utc, merchant_raw, merchant_norm, category, extraction_model, raw_llm_json)
		VALUES ('ext-2', 'email-2', 'receipt', 10000, 'IDR', '2026-11-21T08:51:00Z', 'GrabFood Tip', 'grabfood tip', 'Food', 'sync', '{}')
	`).Error; err != nil {
		t.Fatalf("insert extraction 2: %v", err)
	}
	if err := gdb.Create(&db.TransactionSource{
		ID:            "src-2",
		TransactionID: txID,
		EmailID:       "email-2",
		Role:          "receipt",
		MatchScore:    1.0,
		MatchState:    "confirmed",
	}).Error; err != nil {
		t.Fatalf("attach second source: %v", err)
	}

	splitTxID, err := SplitTransactionSource(gdb, txID, "email-2")
	if err != nil {
		t.Fatalf("SplitTransactionSource: %v", err)
	}
	var splitTx db.Transaction
	if err := gdb.Where("id = ?", splitTxID).First(&splitTx).Error; err != nil {
		t.Fatalf("load split tx: %v", err)
	}
	if splitTx.Reviewed != 0 {
		t.Fatalf("expected split transaction to start with reviewed=0, got %d", splitTx.Reviewed)
	}
	if splitTx.AmountMinor != 10000 {
		t.Fatalf("expected split transaction amount 10000, got %d", splitTx.AmountMinor)
	}
}
