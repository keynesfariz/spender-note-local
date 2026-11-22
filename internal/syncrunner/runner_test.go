package syncrunner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"portspendernote/internal/db"
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

	// Retry mode (requireCountryAndGmail=false) must still succeed for unrecognized or NULL country_code.
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

	// Pass an unreachable LLM URL to prove the deterministic filter skips the LLM call.
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
				{"message": map[string]any{"content": `{"email_kind":"card_charge","amount_decimal":"50000","currency":"IDR","transaction_date":"2026-11-22","transaction_time":"12:00:00","timezone_offset":"+07:00","merchant":"Cafe","category":"Dining","card_last4":"4321","payment_method":"NewBank Card","bank_account_hint":null,"matched_account_id":null,"account_type_guess":"credit_card","reference_code":null}`}},
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
		// Prove extraction starts BEFORE second email is even fetched!
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

	// Restore LLM server and click Retry — must not call Gmail again.
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
