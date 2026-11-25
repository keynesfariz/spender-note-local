package syncrunner

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"portspendernote/internal/db"
	"portspendernote/internal/domain"
	"portspendernote/internal/gmail"
	"portspendernote/internal/llm"
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
	seedRawEmail(t, gdb, "email-new", "gmail-new", sender.ID, 1795251063000, "2026-11-21T08:51:03Z")

	if err := updateSenderSyncState(gdb, sender, true); err != nil {
		t.Fatal(err)
	}

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

func TestUpdateSenderSyncState_CompletedBatch(t *testing.T) {
	newest := int64(1795251063000)
	older := int64(1794273898000)
	for _, tc := range []struct {
		name   string
		stored []int64
		want   *int64
	}{
		{name: "high-water mark across batches", stored: []int64{newest, older}, want: &newest},
		{name: "only prior batch messages", stored: []int64{newest}, want: &newest},
		{name: "no messages"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gdb := openTestDB(t)
			sender := seedSender(t, gdb, "sender-1", "bca@bca.co.id", nil)
			for _, date := range tc.stored {
				receivedAt := time.UnixMilli(date).UTC().Format(time.RFC3339)
				seedRawEmail(t, gdb, receivedAt, "gmail-"+receivedAt, sender.ID, date, receivedAt)
			}

			if err := updateSenderSyncState(gdb, sender, false); err != nil {
				t.Fatal(err)
			}

			var got db.Sender
			if err := gdb.First(&got, "id = ?", sender.ID).Error; err != nil {
				t.Fatalf("load sender: %v", err)
			}
			if got.LastSyncAtUtc == nil || *got.LastSyncAtUtc == "" {
				t.Fatal("expected last_sync_at_utc to be populated")
			}
			if tc.want == nil {
				if got.LastInternalDateMs != nil {
					t.Fatalf("expected an empty cursor, got %d", *got.LastInternalDateMs)
				}
			} else if got.LastInternalDateMs == nil {
				t.Fatalf("expected cursor %d, got nil", *tc.want)
			} else if *got.LastInternalDateMs != *tc.want {
				t.Fatalf("expected cursor %d, got %d", *tc.want, *got.LastInternalDateMs)
			}
		})
	}
}

func TestSenderAddressChangeDiscardsOldCursorAndRejectsStaleUpdates(t *testing.T) {
	gdb := openTestDB(t)
	oldCursor := int64(1795251063000)
	original := seedSender(t, gdb, "sender-1", "bca@bca.co.id", &oldCursor)
	seedRawEmail(t, gdb, "email-old-address", "gmail-old-address", original.ID, oldCursor, "2026-11-21T08:51:03Z")
	if err := updateSenderSyncState(gdb, original, false); err != nil {
		t.Fatal(err)
	}

	newAddress := "receipts@example.com"
	current, err := domain.UpdateSender(gdb, original.ID, domain.UpdateSenderInput{Email: &newAddress})
	if err != nil {
		t.Fatal(err)
	}
	if current.LastInternalDateMs != nil || current.LastSyncAtUtc != nil {
		t.Fatalf("expected address edit to clear cursor and last sync: %+v", current)
	}

	if err := updateSenderSyncState(gdb, original, false); err != nil {
		t.Fatal(err)
	}
	var got db.Sender
	if err := gdb.First(&got, "id = ?", original.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.LastInternalDateMs != nil || got.LastSyncAtUtc != nil {
		t.Fatalf("stale fetch updated sender state: %+v", got)
	}

	if err := updateSenderSyncState(gdb, *current, false); err != nil {
		t.Fatal(err)
	}
	if err := gdb.First(&got, "id = ?", original.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.LastInternalDateMs != nil || got.LastSyncAtUtc == nil {
		t.Fatalf("current fetch included old address's mail or did not update timestamp: %+v", got)
	}
}

func TestFetchWriteFailureLeavesEmailAndCursorRetryable(t *testing.T) {
	gdb := openTestDB(t)
	sender := seedSender(t, gdb, "sender-1", "bca@bca.co.id", nil)
	run := db.SyncRun{
		ID: "run", Status: "running", Phase: "syncing", LlmProvider: "openai_compatible",
		LlmModel: "qwen2.5", StartedAtUtc: "2026-11-22T00:00:00Z",
	}
	if err := gdb.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	if err := gdb.Exec(`
		CREATE TRIGGER reject_fetch_counter BEFORE UPDATE OF fetched_count ON sync_runs
		BEGIN SELECT RAISE(ABORT, 'test counter write rejected'); END`).Error; err != nil {
		t.Fatal(err)
	}

	originalToken, originalFetch := getValidAccessTokenFn, fetchSenderMessagesFn
	t.Cleanup(func() {
		getValidAccessTokenFn, fetchSenderMessagesFn = originalToken, originalFetch
	})
	getValidAccessTokenFn = func(context.Context, *gorm.DB, bool) (string, error) {
		return "test-token", nil
	}
	msg := gmail.FetchedMessage{
		GmailMessageID: "gmail-test", FromAddress: sender.Email, Subject: "Receipt",
		InternalDateMs: 1795251063000, ReceivedAtUtc: "2026-11-21T08:51:03Z",
	}
	fetchSenderMessagesFn = func(_ context.Context, params gmail.FetchSenderMessagesParams) (*gmail.FetchSenderMessagesResult, error) {
		if _, exists := params.KnownMessageIDs[msg.GmailMessageID]; exists {
			t.Fatal("rolled-back Gmail message was marked as known")
		}
		params.OnMessage(msg)
		return &gmail.FetchSenderMessagesResult{Messages: []gmail.FetchedMessage{msg}}, nil
	}

	config := &validatedConfig{SyncBatchLimit: 1}
	err := runFetchWorker(context.Background(), gdb, run.ID, config, nil)
	if err == nil || !strings.Contains(err.Error(), "test counter write rejected") {
		t.Fatalf("expected persistence error, got %v", err)
	}
	var got db.Sender
	if err := gdb.First(&got, "id = ?", sender.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.LastSyncAtUtc != nil || got.LastInternalDateMs != nil {
		t.Fatalf("failed write advanced sender state: %+v", got)
	}
	if err := gdb.Exec("DROP TRIGGER reject_fetch_counter").Error; err != nil {
		t.Fatal(err)
	}
	if err := runFetchWorker(context.Background(), gdb, run.ID, config, nil); err != nil {
		t.Fatal(err)
	}
	if err := gdb.First(&got, "id = ?", sender.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.LastInternalDateMs == nil || *got.LastInternalDateMs != msg.InternalDateMs {
		t.Fatalf("successful retry did not advance cursor: %+v", got)
	}
}

func TestSyncLockPreventsOtherHandlesFromTakingOverLiveRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spender.db")
	open := func(recoverStaleRuns bool) *gorm.DB {
		t.Helper()
		gdb, err := db.OpenDatabase(path, recoverStaleRuns)
		if err != nil {
			t.Fatal(err)
		}
		sqlDB, err := gdb.DB()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = sqlDB.Close() })
		return gdb
	}
	owner := open(false)
	release, err := db.TryLockSync(owner)
	if err != nil || release == nil {
		t.Fatalf("acquire sync lock: %v", err)
	}
	t.Cleanup(release)

	other := open(false)
	if _, err := RetryPendingAndFailedExtractions(other); !errors.Is(err, errAlreadySyncing) {
		t.Fatalf("expected concurrent retry to be rejected, got %v", err)
	}
	if _, err := DismissPendingQueue(other); !errors.Is(err, errAlreadySyncing) {
		t.Fatalf("expected dismiss during sync to be rejected, got %v", err)
	}

	run := db.SyncRun{
		ID: "owned-run", Status: "running", Phase: "syncing", LlmProvider: "openai_compatible",
		LlmModel: "qwen2.5", StartedAtUtc: "2026-11-22T00:00:00Z",
	}
	if err := owner.Create(&run).Error; err != nil {
		t.Fatal(err)
	}

	other = open(true)
	if CancelSync(other) {
		t.Fatal("cancel must not clear a run this worker does not own")
	}
	if err := other.First(&run, "id = ?", run.ID).Error; err != nil || run.Status != "running" {
		t.Fatalf("live run changed during startup recovery: %s, %v", run.Status, err)
	}

	release()
	recovered := open(true)
	if err := recovered.First(&run, "id = ?", run.ID).Error; err != nil || run.Status != "interrupted" {
		t.Fatalf("abandoned run was not recovered: %s, %v", run.Status, err)
	}
}

func TestIsBillingStatementEmail_DetectsStatementsWithoutFalsePositives(t *testing.T) {
	idCountry := "ID"
	usCountry := "US"

	positives := []struct {
		name        string
		countryCode *string
		subject     string
		body        string
	}{
		{
			name:        "Jenius credit card billing statement (universal English)",
			countryCode: nil,
			subject:     "Billing Statement of Jenius Credit Card Platinum d-Card",
			body:        "The following is the summary of your Jenius Credit Card Platinum d-Card billing statement:\nTotal Billing (IDR): 9,139,549.26\nMinimum Payment (IDR): 456,977.00\nDue Date: 08 Dec 2026",
		},
		{
			name:        "Indonesian e-Statement subject",
			countryCode: &idCountry,
			subject:     "e-Statement Kartu Kredit BCA November 2026",
			body:        "Terlampir lembar tagihan kartu kredit Anda.",
		},
		{
			name:        "Indonesian subject phrase only with ID active",
			countryCode: &idCountry,
			subject:     "Tagihan Kartu Kredit BCA November 2026",
			body:        "Silakan cek lampiran.",
		},
		{
			name:        "Generic subject with Indonesian statement body cues (ID active)",
			countryCode: &idCountry,
			subject:     "Your November Summary",
			body:        "Total Tagihan: Rp 5.200.000\nPembayaran Minimum: Rp 520.000\nTanggal Jatuh Tempo: 10 Des 2026",
		},
	}

	for _, tc := range positives {
		if !IsBillingStatementEmail(tc.subject, tc.body, tc.countryCode) {
			t.Errorf("%s: expected IsBillingStatementEmail=true, got false", tc.name)
		}
	}

	negatives := []struct {
		name        string
		countryCode *string
		subject     string
		body        string
	}{
		{
			name:        "Legitimate Indonesian card charge alert",
			countryCode: &idCountry,
			subject:     "Notifikasi Transaksi Kartu Kredit",
			body:        "Transaksi Kartu Kredit berhasil di Tokopedia sebesar Rp 150.000 pada 22 Nov 2026.",
		},
		{
			name:        "Merchant receipt mentioning billing address and payment due",
			countryCode: &idCountry,
			subject:     "Your order receipt #12345",
			body:        "Thank you for your purchase. Billing address: Jakarta. Payment due upon receipt. Total: IDR 250,000.",
		},
		{
			name:        "Indonesian-only cues do not trigger when non-ID country is active",
			countryCode: &usCountry,
			subject:     "Your November Summary",
			body:        "Total Tagihan: Rp 5.200.000\nPembayaran Minimum: Rp 520.000\nTanggal Jatuh Tempo: 10 Des 2026",
		},
		{
			name:        "Indonesian-only subject does not trigger when country is nil",
			countryCode: nil,
			subject:     "Tagihan Kartu Kredit BCA November 2026",
			body:        "Silakan cek lampiran.",
		},
	}

	for _, tc := range negatives {
		if IsBillingStatementEmail(tc.subject, tc.body, tc.countryCode) {
			t.Errorf("%s: expected IsBillingStatementEmail=false, got true", tc.name)
		}
	}
}

func TestLoadValidatedSettings_RejectsUnsupportedSavedCountryOnFullSync(t *testing.T) {
	gdb := openTestDB(t)
	if err := gdb.Exec(`
		UPDATE app_settings
		SET country_code = 'US', timezone = 'America/New_York', currency = 'USD', llm_model = 'qwen2.5-3b'
		WHERE id = 1`,
	).Error; err != nil {
		t.Fatalf("seed app_settings: %v", err)
	}

	if _, err := loadValidatedSettings(gdb, true); err == nil || !strings.Contains(err.Error(), `Saved country "US"`) {
		t.Fatalf("expected unsupported country error on full sync, got: %v", err)
	}

	cfg, err := loadValidatedSettings(gdb, false)
	if err != nil {
		t.Fatalf("expected retry validation to allow unsupported saved country, got: %v", err)
	}
	if cfg.CountryCode == nil || *cfg.CountryCode != "US" {
		t.Fatalf("expected cfg.CountryCode=\"US\", got %v", cfg.CountryCode)
	}

	if err := gdb.Exec(`UPDATE app_settings SET country_code = NULL WHERE id = 1`).Error; err != nil {
		t.Fatalf("clear country_code: %v", err)
	}
	cfgNil, err := loadValidatedSettings(gdb, false)
	if err != nil {
		t.Fatalf("expected retry validation to allow NULL country_code, got: %v", err)
	}
	if cfgNil.CountryCode != nil {
		t.Fatalf("expected nil cfg.CountryCode, got %v", *cfgNil.CountryCode)
	}
}

func TestRunExtractionWorker_ForwardsCountryPromptRulesByActiveCountry(t *testing.T) {
	idCountry := "ID"
	usCountry := "US"

	cases := []struct {
		name           string
		countryCode    *string
		wantContainsID bool
	}{
		{name: "Indonesia forwards Indonesian rules from ID.json", countryCode: &idCountry, wantContainsID: true},
		{name: "Unrecognized country forwards no ID rules", countryCode: &usCountry, wantContainsID: false},
		{name: "Nil country forwards no ID rules", countryCode: nil, wantContainsID: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var systemPrompt string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Messages []struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode llm request: %v", err)
				}
				for _, m := range body.Messages {
					if m.Role == "system" {
						systemPrompt = m.Content
					}
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"choices": []map[string]any{
						{"message": map[string]any{"content": `{"email_kind":"non_transaction","amount_decimal":null,"currency":null,"transaction_date":null,"transaction_time":null,"timezone_offset":null,"merchant":null,"category":null,"card_last4":null,"payment_method":null,"bank_account_hint":null,"matched_account_id":null,"account_type_guess":null,"reference_code":null}`}},
					},
				})
			}))
			defer srv.Close()

			gdb := openTestDB(t)
			sender := seedSender(t, gdb, "sender-1", "alerts@example.com", nil)
			email := db.RawEmail{
				ID:             "email-1",
				GmailMessageID: "gmail-1",
				SenderID:       &sender.ID,
				FromAddress:    "alerts@example.com",
				Subject:        "Receipt from Coffee Shop",
				CleanedText:    "You paid Rp 35.000 at Coffee Shop.",
				InternalDateMs: 1795278003000,
				ReceivedAtUtc:  "2026-11-21T16:20:03Z",
				Status:         "pending",
			}
			if err := gdb.Create(&email).Error; err != nil {
				t.Fatalf("create email: %v", err)
			}
			run := db.SyncRun{
				ID:           "run-1",
				Status:       "running",
				Phase:        "extracting",
				LlmProvider:  "openai_compatible",
				LlmModel:     "qwen2.5",
				StartedAtUtc: "2026-11-22T00:00:00Z",
			}
			if err := gdb.Create(&run).Error; err != nil {
				t.Fatalf("create run: %v", err)
			}

			cfg := &validatedConfig{
				CountryCode:  tc.countryCode,
				Timezone:     "Asia/Jakarta",
				Currency:     "IDR",
				LlmProvider:  "openai_compatible",
				LlmBaseURL:   srv.URL + "/v1",
				LlmModel:     "qwen2.5",
				LlmTimeoutMs: 5000,
			}
			client, err := llm.NewClient(cfg.LlmBaseURL, 5*time.Second)
			if err != nil {
				t.Fatalf("new llm client: %v", err)
			}
			closedDone := make(chan struct{})
			close(closedDone)
			if err := runExtractionWorker(context.Background(), gdb, run.ID, cfg, client, nil, closedDone, nil); err != nil {
				t.Fatalf("extraction worker error: %v", err)
			}

			hasIDRule := strings.Contains(systemPrompt, "WIB")
			if hasIDRule != tc.wantContainsID {
				t.Fatalf("expected systemPrompt Indonesian rule=%v, got %v", tc.wantContainsID, hasIDRule)
			}
		})
	}
}

func TestRunExtractionWorker_SkipsBillingStatementWithoutCallingLLM(t *testing.T) {
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
		LlmProvider:  "openai_compatible",
		LlmModel:     "qwen2.5",
		StartedAtUtc: "2026-11-22T00:00:00Z",
	}
	if err := gdb.Create(&run).Error; err != nil {
		t.Fatalf("create sync_run: %v", err)
	}

	cfg := &validatedConfig{
		Timezone:     "Asia/Jakarta",
		Currency:     "IDR",
		LlmProvider:  "openai_compatible",
		LlmBaseURL:   "http://127.0.0.1:1/v1",
		LlmModel:     "qwen2.5",
		LlmTimeoutMs: 1000,
	}
	client, _ := llm.NewClient(cfg.LlmBaseURL, time.Second)
	closedDone := make(chan struct{})
	close(closedDone)
	if err := runExtractionWorker(context.Background(), gdb, run.ID, cfg, client, nil, closedDone, nil); err != nil {
		t.Fatalf("unexpected runtime error: %v", err)
	}
	finalizeCompleted(gdb, run.ID)

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

func TestFullSync_PreflightFailsFastBeforeFetchingGmail(t *testing.T) {
	gdb := openTestDB(t)
	seedSender(t, gdb, "sender-1", "alerts@bank.com", nil)
	gdb.Exec(`UPDATE app_settings SET country_code = 'ID', timezone = 'Asia/Jakarta', currency = 'IDR', llm_base_url = 'http://127.0.0.1:1/v1', llm_model = 'qwen2.5' WHERE id = 1`)
	gdb.Exec(`UPDATE gmail_oauth SET status = 'connected' WHERE id = 1`)

	var gmailCalls atomic.Int32
	origToken := getValidAccessTokenFn
	origFetch := fetchSenderMessagesFn
	t.Cleanup(func() {
		getValidAccessTokenFn = origToken
		fetchSenderMessagesFn = origFetch
	})
	getValidAccessTokenFn = func(ctx context.Context, gdb *gorm.DB, forceRefresh bool) (string, error) {
		gmailCalls.Add(1)
		return "tok", nil
	}
	fetchSenderMessagesFn = func(ctx context.Context, params gmail.FetchSenderMessagesParams) (*gmail.FetchSenderMessagesResult, error) {
		gmailCalls.Add(1)
		return &gmail.FetchSenderMessagesResult{}, nil
	}

	runID, err := TriggerSync(gdb)
	if err != nil {
		t.Fatalf("trigger sync: %v", err)
	}
	waitForRunDone(t, gdb, runID)

	if gmailCalls.Load() != 0 {
		t.Fatalf("expected 0 Gmail calls when LLM preflight fails, got %d", gmailCalls.Load())
	}
	var run db.SyncRun
	gdb.First(&run, "id = ?", runID)
	if run.Status != "failed" || run.ErrorMessage == nil || !strings.Contains(*run.ErrorMessage, "Cannot reach local LLM server") {
		t.Fatalf("expected failed preflight with unreachable error, got status=%s err=%v", run.Status, run.ErrorMessage)
	}
}

func TestFullSync_PipelinesFetchAndExtractWithProvisionalAccountPropagation(t *testing.T) {
	gdb := openTestDB(t)
	seedSender(t, gdb, "sender-1", "alerts@newbank.com", nil)
	gdb.Exec(`UPDATE gmail_oauth SET status = 'connected' WHERE id = 1`)

	firstExtracted := make(chan struct{})
	releaseSecondFetch := make(chan struct{})
	var secondSawProvisional atomic.Bool
	var extractCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "qwen2.5"}}})
			return
		}
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		n := extractCalls.Add(1)
		if n == 1 {
			close(firstExtracted)
		} else if n == 2 {
			for _, m := range body.Messages {
				if m.Role == "user" && strings.Contains(m.Content, "Known-Accounts:") && strings.Contains(m.Content, "4321") {
					secondSawProvisional.Store(true)
				}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"email_kind":"card_charge","amount_decimal":"50000","currency":"IDR","transaction_date":"2026-11-22","transaction_time":"12:00:00","timezone_offset":"+07:00","merchant":"Cafe","category":"Dining Out","card_last4":"4321","payment_method":"NewBank Card","bank_account_hint":null,"matched_account_id":null,"account_type_guess":"credit_card","reference_code":null}`}},
			},
		})
	}))
	defer srv.Close()

	gdb.Exec(`UPDATE app_settings SET country_code = 'ID', timezone = 'Asia/Jakarta', currency = 'IDR', llm_base_url = ?, llm_model = 'qwen2.5' WHERE id = 1`, srv.URL+"/v1")

	origToken := getValidAccessTokenFn
	origFetch := fetchSenderMessagesFn
	t.Cleanup(func() {
		getValidAccessTokenFn = origToken
		fetchSenderMessagesFn = origFetch
	})
	getValidAccessTokenFn = func(ctx context.Context, gdb *gorm.DB, forceRefresh bool) (string, error) {
		return "tok", nil
	}
	fetchSenderMessagesFn = func(ctx context.Context, params gmail.FetchSenderMessagesParams) (*gmail.FetchSenderMessagesResult, error) {
		msg1 := gmail.FetchedMessage{
			GmailMessageID: "g1",
			FromAddress:    "alerts@newbank.com",
			Subject:        "Charge 1",
			CleanedText:    "Rp 50.000 at Cafe card 4321",
			InternalDateMs: 1795278001000,
			ReceivedAtUtc:  "2026-11-22T05:00:01Z",
		}
		params.OnMessage(msg1)
		select {
		case <-firstExtracted:
		case <-time.After(3 * time.Second):
			t.Errorf("timed out waiting for extraction to overlap with fetching")
		}
		close(releaseSecondFetch)
		msg2 := gmail.FetchedMessage{
			GmailMessageID: "g2",
			FromAddress:    "alerts@newbank.com",
			Subject:        "Charge 2",
			CleanedText:    "Rp 50.000 at Cafe card 4321",
			InternalDateMs: 1795278002000,
			ReceivedAtUtc:  "2026-11-22T05:00:02Z",
		}
		params.OnMessage(msg2)
		return &gmail.FetchSenderMessagesResult{Messages: []gmail.FetchedMessage{msg1, msg2}, HasMore: false}, nil
	}

	runID, err := TriggerSync(gdb)
	if err != nil {
		t.Fatalf("trigger sync: %v", err)
	}
	waitForRunDone(t, gdb, runID)

	if !secondSawProvisional.Load() {
		t.Fatalf("expected email #2 to see provisional account created by email #1")
	}
	var acctCount int64
	gdb.Model(&db.Account{}).Count(&acctCount)
	if acctCount != 1 {
		t.Fatalf("expected exactly 1 provisional account (no duplicate), got %d", acctCount)
	}
	var run db.SyncRun
	gdb.First(&run, "id = ?", runID)
	if run.Status != "completed" || run.FetchedCount != 2 || run.ImportedCount != 2 {
		t.Fatalf("unexpected run state: %+v", run)
	}
}

func TestFullSync_MidRunLlmDropPreservesPendingAndRetryFinishesWithoutGmail(t *testing.T) {
	gdb := openTestDB(t)
	seedSender(t, gdb, "sender-1", "alerts@bank.com", nil)
	gdb.Exec(`UPDATE gmail_oauth SET status = 'connected' WHERE id = 1`)

	var shouldFail atomic.Bool
	var extractCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "qwen2.5"}}})
			return
		}
		if shouldFail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": "server crashed"}})
			return
		}
		n := extractCalls.Add(1)
		if n == 2 {
			shouldFail.Store(true)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"email_kind":"receipt","amount_decimal":"25000","currency":"IDR","transaction_date":"2026-11-22","transaction_time":"12:00:00","timezone_offset":"+07:00","merchant":"Shop","category":"Shopping","card_last4":null,"payment_method":null,"bank_account_hint":null,"matched_account_id":null,"account_type_guess":null,"reference_code":null}`}},
			},
		})
	}))
	defer srv.Close()

	gdb.Exec(`UPDATE app_settings SET country_code = 'ID', timezone = 'Asia/Jakarta', currency = 'IDR', llm_base_url = ?, llm_model = 'qwen2.5' WHERE id = 1`, srv.URL+"/v1")

	var gmailFetchCount atomic.Int32
	origToken := getValidAccessTokenFn
	origFetch := fetchSenderMessagesFn
	t.Cleanup(func() {
		getValidAccessTokenFn = origToken
		fetchSenderMessagesFn = origFetch
	})
	getValidAccessTokenFn = func(ctx context.Context, gdb *gorm.DB, forceRefresh bool) (string, error) {
		return "tok", nil
	}
	fetchSenderMessagesFn = func(ctx context.Context, params gmail.FetchSenderMessagesParams) (*gmail.FetchSenderMessagesResult, error) {
		gmailFetchCount.Add(1)
		msgs := make([]gmail.FetchedMessage, 0, 4)
		for i := 1; i <= 4; i++ {
			m := gmail.FetchedMessage{
				GmailMessageID: "g-" + string(rune('0'+i)),
				FromAddress:    "alerts@bank.com",
				Subject:        "Receipt",
				CleanedText:    "Rp 25.000",
				InternalDateMs: int64(1795278000000 + i*1000),
				ReceivedAtUtc:  "2026-11-22T05:00:00Z",
			}
			params.OnMessage(m)
			msgs = append(msgs, m)
		}
		return &gmail.FetchSenderMessagesResult{Messages: msgs, HasMore: false}, nil
	}

	runID, err := TriggerSync(gdb)
	if err != nil {
		t.Fatalf("trigger sync: %v", err)
	}
	waitForRunDone(t, gdb, runID)

	var run db.SyncRun
	gdb.First(&run, "id = ?", runID)
	if run.Status != "partial_error" || run.ImportedCount != 2 {
		t.Fatalf("expected partial_error with 2 imported, got status=%s imported=%d err=%v", run.Status, run.ImportedCount, run.ErrorMessage)
	}

	var pendingCnt int64
	gdb.Model(&db.RawEmail{}).Where("status = 'pending'").Count(&pendingCnt)
	if pendingCnt != 2 {
		t.Fatalf("expected 2 remaining pending emails, got %d", pendingCnt)
	}

	shouldFail.Store(false)
	retryID, err := RetryPendingAndFailedExtractions(gdb)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	waitForRunDone(t, gdb, retryID)

	if gmailFetchCount.Load() != 1 {
		t.Fatalf("expected Retry not to re-fetch Gmail (count=1), got %d", gmailFetchCount.Load())
	}
	var retryRun db.SyncRun
	gdb.First(&retryRun, "id = ?", retryID)
	if retryRun.Status != "completed" || retryRun.ImportedCount != 2 {
		t.Fatalf("expected retry completed with 2 imported, got %+v", retryRun)
	}
}

func TestFullSync_CancelSyncStopsBothWorkersCleanly(t *testing.T) {
	gdb := openTestDB(t)
	seedSender(t, gdb, "sender-1", "alerts@bank.com", nil)
	gdb.Exec(`UPDATE gmail_oauth SET status = 'connected' WHERE id = 1`)

	inFlightLLM := make(chan struct{})
	releaseLLM := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "qwen2.5"}}})
			return
		}
		select {
		case <-inFlightLLM:
		default:
			close(inFlightLLM)
		}
		select {
		case <-r.Context().Done():
		case <-releaseLLM:
		}
	}))
	defer func() {
		close(releaseLLM)
		srv.Close()
	}()

	gdb.Exec(`UPDATE app_settings SET country_code = 'ID', timezone = 'Asia/Jakarta', currency = 'IDR', llm_base_url = ?, llm_model = 'qwen2.5' WHERE id = 1`, srv.URL+"/v1")

	origToken := getValidAccessTokenFn
	origFetch := fetchSenderMessagesFn
	t.Cleanup(func() {
		getValidAccessTokenFn = origToken
		fetchSenderMessagesFn = origFetch
	})
	getValidAccessTokenFn = func(ctx context.Context, gdb *gorm.DB, forceRefresh bool) (string, error) {
		return "tok", nil
	}
	fetchSenderMessagesFn = func(ctx context.Context, params gmail.FetchSenderMessagesParams) (*gmail.FetchSenderMessagesResult, error) {
		params.OnMessage(gmail.FetchedMessage{
			GmailMessageID: "g-cancel-1",
			FromAddress:    "alerts@bank.com",
			Subject:        "Charge",
			CleanedText:    "Rp 10.000",
			InternalDateMs: 1795278001000,
			ReceivedAtUtc:  "2026-11-22T05:00:01Z",
		})
		<-ctx.Done()
		return nil, ctx.Err()
	}

	runID, err := TriggerSync(gdb)
	if err != nil {
		t.Fatalf("trigger sync: %v", err)
	}
	select {
	case <-inFlightLLM:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for in-flight LLM request")
	}

	if !CancelSync(gdb) {
		t.Fatalf("expected CancelSync to return true")
	}

	var run db.SyncRun
	gdb.First(&run, "id = ?", runID)
	if run.Status != "cancelled" || run.Phase != "done" {
		t.Fatalf("expected cancelled/done after CancelSync returns, got status=%s phase=%s", run.Status, run.Phase)
	}
}

func waitForRunDone(t *testing.T, gdb *gorm.DB, runID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, err := GetSyncStatus(gdb)
		if err == nil && !status.IsRunning && status.LatestRun != nil && status.LatestRun.ID == runID && status.LatestRun.Status != "running" {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for sync run %s to finish", runID)
}

func TestRunExtractionWorker_UsesCustomCategoriesFromDatabase(t *testing.T) {
	gdb := openTestDB(t)
	category, err := domain.CreateCategory(gdb, domain.CreateCategoryInput{
		Name: "Pet Supplies",
		Icon: "PawPrint",
	})
	if err != nil {
		t.Fatalf("create custom category: %v", err)
	}

	var seenSystemPrompt string
	var seenSchemaCategories bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			ResponseFormat map[string]any `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode llm request: %v", err)
		}
		for _, m := range body.Messages {
			if m.Role == "system" {
				seenSystemPrompt = m.Content
			}
		}
		rawBytes, _ := json.Marshal(body.ResponseFormat)
		if strings.Contains(string(rawBytes), "Pet Supplies") {
			seenSchemaCategories = true
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"email_kind":"receipt","amount_decimal":"120000","currency":"IDR","transaction_date":"2026-11-22","transaction_time":"11:00:00","timezone_offset":"+07:00","merchant":"Pet Kingdom","category":"Pet Supplies","card_last4":null,"payment_method":null,"bank_account_hint":null,"matched_account_id":null,"account_type_guess":null,"reference_code":null}`}},
			},
		})
	}))
	defer srv.Close()

	sender := seedSender(t, gdb, "sender-pet", "pet@example.com", nil)
	if err := gdb.Create(&db.RawEmail{
		ID:             "email-pet-1",
		GmailMessageID: "gmail-pet-1",
		SenderID:       &sender.ID,
		FromAddress:    "pet@example.com",
		Subject:        "Receipt from Pet Kingdom",
		CleanedText:    "Paid Rp 120.000 at Pet Kingdom",
		InternalDateMs: 1795278003000,
		ReceivedAtUtc:  "2026-11-22T04:00:00Z",
		Status:         "pending",
	}).Error; err != nil {
		t.Fatalf("create email: %v", err)
	}
	if err := gdb.Create(&db.SyncRun{
		ID:           "run-pet-1",
		Status:       "running",
		Phase:        "extracting",
		LlmProvider:  "openai_compatible",
		LlmModel:     "qwen2.5",
		StartedAtUtc: "2026-11-22T04:05:00Z",
	}).Error; err != nil {
		t.Fatalf("create run: %v", err)
	}

	cfg := &validatedConfig{
		Timezone:     "Asia/Jakarta",
		Currency:     "IDR",
		LlmProvider:  "openai_compatible",
		LlmBaseURL:   srv.URL + "/v1",
		LlmModel:     "qwen2.5",
		LlmTimeoutMs: 5000,
	}
	client, err := llm.NewClient(cfg.LlmBaseURL, 5*time.Second)
	if err != nil {
		t.Fatalf("new llm client: %v", err)
	}
	closedDone := make(chan struct{})
	close(closedDone)
	if err := runExtractionWorker(context.Background(), gdb, "run-pet-1", cfg, client, nil, closedDone, nil); err != nil {
		t.Fatalf("runExtractionWorker: %v", err)
	}

	if !strings.Contains(seenSystemPrompt, "Pet Supplies") || !seenSchemaCategories {
		t.Fatalf("expected custom category 'Pet Supplies' in LLM prompt and schema, got prompt=%q schemaHas=%v", seenSystemPrompt, seenSchemaCategories)
	}
	var tx db.Transaction
	if err := gdb.First(&tx, "merchant = ?", "Pet Kingdom").Error; err != nil {
		t.Fatalf("load created transaction: %v", err)
	}
	if tx.CategoryID != category.ID {
		t.Fatalf("expected persisted category ID %q, got %q", category.ID, tx.CategoryID)
	}
}

func TestRunExtractionWorker_LearnedPoliciesPrecedenceGhostStateAndRace(t *testing.T) {
	gdb := openTestDB(t)
	sender := seedSender(t, gdb, "sender-grab", "no-reply@grab.com", nil)

	ignoreSig := domain.DeriveEmailTemplateSignature(
		"no-reply@grab.com",
		"Ride Cover Plus: Sorry for the long wait. Here's your Rp11.000 voucher.",
		"Ride Cover\nHi Nagib Fariz,",
	)
	admitSig := domain.DeriveEmailTemplateSignature(
		"no-reply@grab.com",
		"Your Grab E-Receipt",
		"Grab Tips E-Receipt\nGrabFood\nRp 5000",
	)
	if err := gdb.Create([]db.SenderTriagePolicy{
		{
			ID:             "pol-ignore",
			SenderEmail:    ignoreSig.SenderEmail,
			SignatureKey:   ignoreSig.SignatureKey,
			SubjectPattern: ignoreSig.SubjectPattern,
			BodyCue:        ignoreSig.BodyCue,
			Action:         "ignore",
			CreatedAtUtc:   "2026-11-24T00:00:00Z",
			UpdatedAtUtc:   "2026-11-24T00:00:00Z",
		},
		{
			ID:             "pol-admit",
			SenderEmail:    admitSig.SenderEmail,
			SignatureKey:   admitSig.SignatureKey,
			SubjectPattern: admitSig.SubjectPattern,
			BodyCue:        admitSig.BodyCue,
			Action:         "admit",
			CreatedAtUtc:   "2026-11-24T00:00:00Z",
			UpdatedAtUtc:   "2026-11-24T00:00:00Z",
		},
	}).Error; err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	midFlightEntered := make(chan struct{})
	releaseMidFlight := make(chan struct{})
	raceSig := domain.DeriveEmailTemplateSignature(
		"no-reply@grab.com",
		"Race Test Transfer Alert",
		"Transaction Type\n:\nRace Payment\nRp 75000",
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		userPrompt := ""
		for _, m := range body.Messages {
			if m.Role == "user" {
				userPrompt = m.Content
			}
		}
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(userPrompt, "email-ignored"):
			t.Fatalf("learned ignore policy must skip LLM call")
		case strings.Contains(userPrompt, "email-admit-null"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"content": `{"email_kind":"non_transaction","amount_decimal":null,"currency":null,"transaction_date":null,"transaction_time":null,"timezone_offset":null,"merchant":null,"category":null,"card_last4":null,"payment_method":null,"bank_account_hint":null,"matched_account_id":null,"account_type_guess":null,"reference_code":null}`}},
				},
			})
		case strings.Contains(userPrompt, "email-admit-ok"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"content": `{"email_kind":"non_transaction","amount_decimal":"5000","currency":"IDR","transaction_date":"2026-11-23","transaction_time":"20:14:00","timezone_offset":"+07:00","merchant":"","category":"Transport","card_last4":"6039","payment_method":"GrabPay","bank_account_hint":null,"matched_account_id":null,"account_type_guess":"credit_card","reference_code":null}`}},
				},
			})
		case strings.Contains(userPrompt, "email-ghost"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"content": `{"email_kind":"non_transaction","amount_decimal":"100000","currency":"IDR","transaction_date":"2026-11-24","transaction_time":"11:34:40","timezone_offset":"+07:00","merchant":null,"category":null,"card_last4":null,"payment_method":"Bank Transfer","bank_account_hint":"****9988","matched_account_id":null,"account_type_guess":"debit_card","reference_code":null}`}},
				},
			})
		case strings.Contains(userPrompt, "email-zero"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"content": `{"email_kind":"non_transaction","amount_decimal":null,"currency":null,"transaction_date":null,"transaction_time":null,"timezone_offset":null,"merchant":null,"category":null,"card_last4":null,"payment_method":null,"bank_account_hint":null,"matched_account_id":null,"account_type_guess":null,"reference_code":null}`}},
				},
			})
		case strings.Contains(userPrompt, "email-race"):
			close(midFlightEntered)
			<-releaseMidFlight
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"content": `{"email_kind":"non_transaction","amount_decimal":"75000","currency":"IDR","transaction_date":"2026-11-24","transaction_time":"12:00:00","timezone_offset":"+07:00","merchant":"Race Merchant","category":"Shopping","card_last4":null,"payment_method":null,"bank_account_hint":null,"matched_account_id":null,"account_type_guess":null,"reference_code":null}`}},
				},
			})
		}
	}))
	defer srv.Close()

	emails := []db.RawEmail{
		{
			ID: "email-ignored", GmailMessageID: "gm-1", SenderID: &sender.ID, FromAddress: "no-reply@grab.com",
			Subject: "Ride Cover Plus: Sorry for the long wait. Here's your Rp25.000 voucher.", CleanedText: "Ride Cover\nHi Nagib Fariz,",
			InternalDateMs: 1795400001000, ReceivedAtUtc: "2026-11-24T01:00:01Z", Status: "pending",
		},
		{
			ID: "email-admit-ok", GmailMessageID: "gm-2", SenderID: &sender.ID, FromAddress: "no-reply@grab.com",
			Subject: "Your Grab E-Receipt", CleanedText: "Grab Tips E-Receipt\nGrabFood\nemail-admit-ok\nRp 5000",
			InternalDateMs: 1795400002000, ReceivedAtUtc: "2026-11-24T01:00:02Z", Status: "pending",
		},
		{
			ID: "email-admit-null", GmailMessageID: "gm-3", SenderID: &sender.ID, FromAddress: "no-reply@grab.com",
			Subject: "Your Grab E-Receipt", CleanedText: "Grab Tips E-Receipt\nGrabFood\nemail-admit-null",
			InternalDateMs: 1795400003000, ReceivedAtUtc: "2026-11-24T01:00:03Z", Status: "pending",
		},
		{
			ID: "email-ghost", GmailMessageID: "gm-4", SenderID: &sender.ID, FromAddress: "no-reply@grab.com",
			Subject: "Transfer Alert", CleanedText: "email-ghost",
			InternalDateMs: 1795400004000, ReceivedAtUtc: "2026-11-24T01:00:04Z", Status: "pending",
		},
		{
			ID: "email-zero", GmailMessageID: "gm-5", SenderID: &sender.ID, FromAddress: "no-reply@grab.com",
			Subject: "Security Alert", CleanedText: "email-zero",
			InternalDateMs: 1795400005000, ReceivedAtUtc: "2026-11-24T01:00:05Z", Status: "pending",
		},
		{
			ID: "email-race", GmailMessageID: "gm-6", SenderID: &sender.ID, FromAddress: "no-reply@grab.com",
			Subject: "Race Test Transfer Alert", CleanedText: "Transaction Type\n:\nRace Payment\nRp 75000\nemail-race",
			InternalDateMs: 1795400006000, ReceivedAtUtc: "2026-11-24T01:00:06Z", Status: "pending",
		},
	}
	for _, em := range emails {
		if err := gdb.Create(&em).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := gdb.Create(&db.SyncRun{
		ID: "run-triage", Status: "running", Phase: "extracting", LlmProvider: "openai_compatible", LlmModel: "qwen2.5", StartedAtUtc: "2026-11-24T02:00:00Z",
	}).Error; err != nil {
		t.Fatal(err)
	}

	cfg := &validatedConfig{
		Timezone: "Asia/Jakarta", Currency: "IDR", LlmProvider: "openai_compatible", LlmBaseURL: srv.URL + "/v1", LlmModel: "qwen2.5", LlmTimeoutMs: 5000,
	}
	client, err := llm.NewClient(cfg.LlmBaseURL, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	closedDone := make(chan struct{})
	close(closedDone)

	go func() {
		<-midFlightEntered
		_ = gdb.Create(&db.SenderTriagePolicy{
			ID:             "pol-race-admit",
			SenderEmail:    raceSig.SenderEmail,
			SignatureKey:   raceSig.SignatureKey,
			SubjectPattern: raceSig.SubjectPattern,
			BodyCue:        raceSig.BodyCue,
			Action:         "admit",
			CreatedAtUtc:   "2026-11-24T02:01:00Z",
			UpdatedAtUtc:   "2026-11-24T02:01:00Z",
		}).Error
		close(releaseMidFlight)
	}()

	if err := runExtractionWorker(context.Background(), gdb, "run-triage", cfg, client, nil, closedDone, nil); err != nil {
		t.Fatalf("worker failed: %v", err)
	}

	if calls.Load() != 5 {
		t.Fatalf("expected 5 LLM calls (ignored email skipped), got %d", calls.Load())
	}

	var extIgnored, extAdmitOK, extGhost, extZero, extRace db.EmailExtraction
	var reAdmitNull db.RawEmail
	gdb.First(&extIgnored, "email_id = ?", "email-ignored")
	if extIgnored.ExtractionModel != "policy:ignore" || extIgnored.TriageState == nil || *extIgnored.TriageState != "ignored" {
		t.Fatalf("unexpected ignored extraction: %+v", extIgnored)
	}
	gdb.First(&extAdmitOK, "email_id = ?", "email-admit-ok")
	var srcAdmitOK db.TransactionSource
	if err := gdb.First(&srcAdmitOK, "email_id = ?", "email-admit-ok").Error; err != nil || srcAdmitOK.Role != "card_charge" {
		t.Fatalf("expected forced admit transaction source with card_charge role, got %+v err=%v", srcAdmitOK, err)
	}
	var txAdmitOK db.Transaction
	gdb.First(&txAdmitOK, "id = ?", srcAdmitOK.TransactionID)
	if txAdmitOK.Reviewed != 0 || txAdmitOK.AmountMinor != 5000 {
		t.Fatalf("expected unreviewed 5000 transaction, got %+v", txAdmitOK)
	}
	if err := gdb.First(&reAdmitNull, "id = ?", "email-admit-null").Error; err != nil || reAdmitNull.Status != "failed" {
		t.Fatalf("expected admit-matched missing-amount email to mark failed, got %+v", reAdmitNull)
	}
	gdb.First(&extGhost, "email_id = ?", "email-ghost")
	if extGhost.TriageState == nil || *extGhost.TriageState != "ghost" || extGhost.AmountMinor == nil || *extGhost.AmountMinor != 100000 || extGhost.MerchantRaw != nil {
		t.Fatalf("expected ghost extraction with 100000 and NULL blank merchant_raw, got %+v", extGhost)
	}
	gdb.First(&extZero, "email_id = ?", "email-zero")
	if extZero.TriageState != nil {
		t.Fatalf("expected zero-amount email to have NULL triage_state, got %q", *extZero.TriageState)
	}
	gdb.First(&extRace, "email_id = ?", "email-race")
	var srcRace db.TransactionSource
	if err := gdb.First(&srcRace, "email_id = ?", "email-race").Error; err != nil {
		t.Fatalf("expected mid-sync admit policy race to force-admit email-race transaction, err=%v ext=%+v", err, extRace)
	}
}
