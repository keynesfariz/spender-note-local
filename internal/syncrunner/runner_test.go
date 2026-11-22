package syncrunner

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"portspendernote/internal/db"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gdb, err := db.OpenDatabase(":memory:", false)
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	return gdb
}

func seedSender(t *testing.T, gdb *gorm.DB, id, email string, lastCursor *int64) db.Sender {
	t.Helper()
	s := db.Sender{
		ID:                 id,
		Email:              email,
		RoleHint:           "auto",
		Enabled:            1,
		BackfillFromUtc:    "2026-10-31T17:00:00Z",
		LastInternalDateMs: lastCursor,
	}
	if err := gdb.Create(&s).Error; err != nil {
		t.Fatalf("create sender: %v", err)
	}
	return s
}

func seedRawEmail(t *testing.T, gdb *gorm.DB, id, gmailID, senderID string, internalDateMs int64, receivedAtUtc string) {
	t.Helper()
	row := db.RawEmail{
		ID:             id,
		GmailMessageID: gmailID,
		SenderID:       &senderID,
		FromAddress:    "bca@bca.co.id",
		Subject:        "Test",
		CleanedText:    "Test body",
		InternalDateMs: internalDateMs,
		ReceivedAtUtc:  receivedAtUtc,
		Status:         "extracted",
	}
	if err := gdb.Create(&row).Error; err != nil {
		t.Fatalf("create raw_email: %v", err)
	}
}

func TestUpdateSenderSyncState_PartialBatchUpdatesSyncTimestampWithoutAdvancingCursor(t *testing.T) {
	gdb := openTestDB(t)
	sender := seedSender(t, gdb, "sender-1", "bca@bca.co.id", nil)

	newestMs := int64(1795251063000)
	updateSenderSyncState(gdb, sender, &newestMs, true)

	var got db.Sender
	if err := gdb.First(&got, "id = ?", sender.ID).Error; err != nil {
		t.Fatalf("load sender: %v", err)
	}
	if got.LastSyncAtUtc == nil || *got.LastSyncAtUtc == "" {
		t.Fatalf("expected last_sync_at_utc to be populated on partial batch (hasMore=true), got nil")
	}
	if got.LastInternalDateMs != nil {
		t.Fatalf("expected last_internal_date_ms to remain NULL while hasMore=true so older emails are not skipped, got %d", *got.LastInternalDateMs)
	}
}

func TestUpdateSenderSyncState_FinalBatchAdvancesCursorToHighWaterMarkAcrossPriorBatches(t *testing.T) {
	gdb := openTestDB(t)
	sender := seedSender(t, gdb, "sender-1", "bca@bca.co.id", nil)

	// Earlier batch-capped run fetched newer emails (Gmail returns newest-first).
	earlierNewestMs := int64(1795251063000)
	seedRawEmail(t, gdb, "email-new", "gmail-new", sender.ID, earlierNewestMs, "2026-11-21T08:51:03Z")

	// Final batch fetches older emails and completes with hasMore=false.
	finalBatchOlderMs := int64(1794273898000)
	seedRawEmail(t, gdb, "email-old", "gmail-old", sender.ID, finalBatchOlderMs, "2026-11-10T01:24:58Z")

	updateSenderSyncState(gdb, sender, &finalBatchOlderMs, false)

	var got db.Sender
	if err := gdb.First(&got, "id = ?", sender.ID).Error; err != nil {
		t.Fatalf("load sender: %v", err)
	}
	if got.LastSyncAtUtc == nil || *got.LastSyncAtUtc == "" {
		t.Fatalf("expected last_sync_at_utc to be populated, got nil")
	}
	if got.LastInternalDateMs == nil {
		t.Fatalf("expected last_internal_date_ms to be populated after final batch, got nil")
	}
	if *got.LastInternalDateMs != earlierNewestMs {
		t.Fatalf("expected last_internal_date_ms=%d (high-water mark from earlier batch), got %d", earlierNewestMs, *got.LastInternalDateMs)
	}
}

func TestUpdateSenderSyncState_FinalBatchWithZeroNewEmailsUsesStoredHighWaterMark(t *testing.T) {
	gdb := openTestDB(t)
	sender := seedSender(t, gdb, "sender-1", "bca@bca.co.id", nil)

	earlierNewestMs := int64(1795251063000)
	seedRawEmail(t, gdb, "email-new", "gmail-new", sender.ID, earlierNewestMs, "2026-11-21T08:51:03Z")

	// Final run finds 0 remaining unseen messages (maxInternalDateMs == nil) and hasMore=false.
	updateSenderSyncState(gdb, sender, nil, false)

	var got db.Sender
	if err := gdb.First(&got, "id = ?", sender.ID).Error; err != nil {
		t.Fatalf("load sender: %v", err)
	}
	if got.LastSyncAtUtc == nil || *got.LastSyncAtUtc == "" {
		t.Fatalf("expected last_sync_at_utc to be populated, got nil")
	}
	if got.LastInternalDateMs == nil || *got.LastInternalDateMs != earlierNewestMs {
		t.Fatalf("expected last_internal_date_ms=%d, got %v", earlierNewestMs, got.LastInternalDateMs)
	}
}

func TestUpdateSenderSyncState_NoEmailsAtAllLeavesCursorNullAndSetsSyncTimestamp(t *testing.T) {
	gdb := openTestDB(t)
	sender := seedSender(t, gdb, "sender-empty", "empty@example.com", nil)

	updateSenderSyncState(gdb, sender, nil, false)

	var got db.Sender
	if err := gdb.First(&got, "id = ?", sender.ID).Error; err != nil {
		t.Fatalf("load sender: %v", err)
	}
	if got.LastSyncAtUtc == nil || *got.LastSyncAtUtc == "" {
		t.Fatalf("expected last_sync_at_utc to be populated, got nil")
	}
	if got.LastInternalDateMs != nil {
		t.Fatalf("expected last_internal_date_ms to stay NULL when no emails exist, got %d", *got.LastInternalDateMs)
	}
}

func TestIsBillingStatementEmail_DetectsStatementsWithoutFalsePositives(t *testing.T) {
	positives := []struct {
		name    string
		subject string
		body    string
	}{
		{
			name:    "Jenius credit card billing statement",
			subject: "Billing Statement of Jenius Credit Card Platinum d-Card",
			body:    "The following is the summary of your Jenius Credit Card Platinum d-Card billing statement:\nTotal Billing (IDR): 9,139,549.26\nMinimum Payment (IDR): 456,977.00\nDue Date: 08 Dec 2026",
		},
		{
			name:    "Indonesian e-Statement subject",
			subject: "e-Statement Kartu Kredit BCA November 2026",
			body:    "Terlampir lembar tagihan kartu kredit Anda.",
		},
		{
			name:    "Generic subject with statement body cues",
			subject: "Your November Summary",
			body:    "Total Tagihan: Rp 5.200.000\nPembayaran Minimum: Rp 520.000\nTanggal Jatuh Tempo: 10 Des 2026",
		},
	}

	for _, tc := range positives {
		if !IsBillingStatementEmail(tc.subject, tc.body) {
			t.Errorf("%s: expected IsBillingStatementEmail=true, got false", tc.name)
		}
	}

	negatives := []struct {
		name    string
		subject string
		body    string
	}{
		{
			name:    "Legitimate Indonesian card charge alert",
			subject: "Notifikasi Transaksi Kartu Kredit",
			body:    "Transaksi Kartu Kredit berhasil di Tokopedia sebesar Rp 150.000 pada 22 Nov 2026.",
		},
		{
			name:    "Merchant receipt mentioning billing address and payment due",
			subject: "Your order receipt #12345",
			body:    "Thank you for your purchase. Billing address: Jakarta. Payment due upon receipt. Total: IDR 250,000.",
		},
	}

	for _, tc := range negatives {
		if IsBillingStatementEmail(tc.subject, tc.body) {
			t.Errorf("%s: expected IsBillingStatementEmail=false, got true", tc.name)
		}
	}
}

func TestRunExtractionPhase_SkipsBillingStatementWithoutCallingLLM(t *testing.T) {
	gdb := openTestDB(t)
	sender := seedSender(t, gdb, "sender-jenius", "jenius_noreply@smbci.com", nil)

	stmtEmail := db.RawEmail{
		ID:             "email-stmt-1",
		GmailMessageID: "gmail-stmt-1",
		SenderID:       &sender.ID,
		FromAddress:    "jenius_noreply@smbci.com",
		Subject:        "Billing Statement of Jenius Credit Card Platinum d-Card",
		CleanedText:    "Total Billing (IDR): 9,139,549.26\nMinimum Payment (IDR): 456,977.00\nDue Date: 08 Dec 2026",
		InternalDateMs: 1795278003000,
		ReceivedAtUtc:  "2026-11-21T16:20:03Z",
		Status:         "pending",
	}
	if err := gdb.Create(&stmtEmail).Error; err != nil {
		t.Fatalf("create pending statement email: %v", err)
	}

	run := db.SyncRun{
		ID:           "run-stmt-1",
		Status:       "running",
		Phase:        "extracting",
		LlmProvider:  "gguf",
		LlmModel:     "test.gguf",
		StartedAtUtc: "2026-11-22T00:00:00Z",
	}
	if err := gdb.Create(&run).Error; err != nil {
		t.Fatalf("create sync_run: %v", err)
	}

	// Pass an unreachable LLM URL to prove the deterministic filter skips the LLM call.
	cfg := &validatedConfig{
		Timezone:     "Asia/Jakarta",
		Currency:     "IDR",
		LlmProvider:  "gguf",
		LlmModelPath: "test.gguf",
		LlmTimeoutMs: 1000,
	}
	runExtractionPhase(context.Background(), gdb, run.ID, cfg, "http://127.0.0.1:1")

	var updatedEmail db.RawEmail
	if err := gdb.First(&updatedEmail, "id = ?", stmtEmail.ID).Error; err != nil {
		t.Fatalf("load email: %v", err)
	}
	if updatedEmail.Status != "non_transaction" {
		t.Fatalf("expected raw_email status='non_transaction', got %q", updatedEmail.Status)
	}

	var ext db.EmailExtraction
	if err := gdb.First(&ext, "email_id = ?", stmtEmail.ID).Error; err != nil {
		t.Fatalf("load extraction: %v", err)
	}
	if ext.EmailKind != "non_transaction" {
		t.Fatalf("expected extraction email_kind='non_transaction', got %q", ext.EmailKind)
	}

	var updatedRun db.SyncRun
	if err := gdb.First(&updatedRun, "id = ?", run.ID).Error; err != nil {
		t.Fatalf("load sync_run: %v", err)
	}
	if updatedRun.SkippedCount != 1 || updatedRun.FailedCount != 0 || updatedRun.Status != "completed" {
		t.Fatalf("unexpected sync_run after statement skip: status=%s skipped=%d failed=%d", updatedRun.Status, updatedRun.SkippedCount, updatedRun.FailedCount)
	}

	var txCount int64
	gdb.Model(&db.Transaction{}).Count(&txCount)
	if txCount != 0 {
		t.Fatalf("expected 0 transactions created for billing statement, got %d", txCount)
	}
}
