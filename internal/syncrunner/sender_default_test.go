package syncrunner

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"portspendernote/internal/db"
)

func TestRunExtractionWorker_IgnoresMissingSenderDefaultAccount(t *testing.T) {
	gdb := openIntegrationDB(t, "missing-sender-default.db")
	sender := seedSender(t, gdb, "sender", "sender@example.com", nil)
	// Reproduce a sender imported before the baseline had its write guards.
	for _, statement := range []string{
		"DROP TRIGGER IF EXISTS senders_validate_default_account_insert",
		"DROP TRIGGER IF EXISTS senders_validate_default_account_update",
		"PRAGMA foreign_keys = OFF",
		"UPDATE senders SET default_account_id = 'missing' WHERE id = 'sender'",
		"PRAGMA foreign_keys = ON",
	} {
		if err := gdb.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	seedRawEmail(t, gdb, "email", "gmail", sender.ID, 1795251063000, "2026-11-21T08:51:03Z")
	if err := gdb.Model(&db.RawEmail{}).Where("id = ?", "email").Update("status", "pending").Error; err != nil {
		t.Fatal(err)
	}

	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{
				"content": `{"email_kind":"receipt","amount_decimal":"120000","currency":"IDR","transaction_date":"2026-11-21","transaction_time":"15:51:03","timezone_offset":"+07:00","merchant":"Example Merchant","category":"Other"}`,
			}}},
		})
	}))
	defer llmServer.Close()
	runIntegrationExtractionWorker(t, gdb, "Asia/Jakarta", "IDR", llmServer.URL+"/v1")

	var email db.RawEmail
	if err := gdb.First(&email, "id = ?", "email").Error; err != nil {
		t.Fatal(err)
	}
	if email.Status != "extracted" {
		if email.ErrorMessage != nil {
			t.Fatalf("email status = %q: %s", email.Status, *email.ErrorMessage)
		}
		t.Fatalf("email status = %q, want extracted", email.Status)
	}
	var source db.TransactionSource
	if err := gdb.First(&source, "email_id = ?", email.ID).Error; err != nil {
		t.Fatal(err)
	}
	var transaction db.Transaction
	if err := gdb.First(&transaction, "id = ?", source.TransactionID).Error; err != nil {
		t.Fatal(err)
	}
	if transaction.AccountID != nil {
		t.Fatalf("transaction account = %q, want no account", *transaction.AccountID)
	}
}
