package syncrunner

import (
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
