package syncrunner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"portspendernote/internal/db"
	"portspendernote/internal/domain"
	"portspendernote/internal/llm"
)

func openIntegrationDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), name)
	gdb, err := db.OpenDatabase(dbPath, false)
	if err != nil {
		t.Fatalf("open integration db: %v", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return gdb
}

func configureIntegrationSettings(t *testing.T, gdb *gorm.DB, tz, currency, llmBaseURL string) {
	t.Helper()
	updates := map[string]any{
		"timezone":       tz,
		"currency":       currency,
		"llm_provider":   "openai_compatible",
		"llm_timeout_ms": 5000,
	}
	if llmBaseURL != "" {
		updates["llm_base_url"] = llmBaseURL
		updates["llm_model"] = "qwen2.5"
	}
	if err := gdb.Model(&db.AppSetting{}).Where("id = ?", 1).Updates(updates).Error; err != nil {
		t.Fatalf("configure app_settings: %v", err)
	}
}

func runIntegrationExtractionWorker(t *testing.T, gdb *gorm.DB, tz, currency, baseURL string) {
	t.Helper()
	if err := gdb.Exec(`UPDATE sync_runs SET status = 'completed', phase = 'done' WHERE status = 'running'`).Error; err != nil {
		t.Fatalf("close prior sync_run: %v", err)
	}
	runID := "run-" + t.Name()
	var existing db.SyncRun
	if gdb.First(&existing, "id = ?", runID).Error == nil {
		runID = runID + "-" + time.Now().Format("150405.000000")
	}
	if err := gdb.Create(&db.SyncRun{
		ID:           runID,
		Status:       "running",
		Phase:        "extracting",
		LlmProvider:  "openai_compatible",
		LlmModel:     "qwen2.5",
		StartedAtUtc: "2026-11-24T12:00:00Z",
	}).Error; err != nil {
		t.Fatalf("create sync_run: %v", err)
	}

	cfg := &validatedConfig{
		Timezone:     tz,
		Currency:     currency,
		LlmProvider:  "openai_compatible",
		LlmBaseURL:   baseURL,
		LlmModel:     "qwen2.5",
		LlmTimeoutMs: 5000,
	}
	client, err := llm.NewClient(cfg.LlmBaseURL, 5*time.Second)
	if err != nil {
		t.Fatalf("llm.NewClient: %v", err)
	}
	closedDone := make(chan struct{})
	close(closedDone)
	if err := runExtractionWorker(context.Background(), gdb, runID, cfg, client, nil, closedDone, nil); err != nil {
		t.Fatalf("runExtractionWorker: %v", err)
	}
	if err := gdb.Exec(`UPDATE sync_runs SET status = 'completed', phase = 'done' WHERE id = ?`, runID).Error; err != nil {
		t.Fatalf("complete sync_run: %v", err)
	}
}

func TestTriageIntegration_PromoteRippleAndUndo(t *testing.T) {
	gdb := openIntegrationDB(t, "triage.db")
	configureIntegrationSettings(t, gdb, "Asia/Jakarta", "IDR", "")

	const bcaCursor int64 = 1795524154000
	if err := gdb.Exec(`
		INSERT INTO accounts (id, name, type, currency, status, initial_balance_minor, initial_balance_as_of_utc, created_at_utc)
		VALUES ('acct-bca', 'BCA Main', 'debit_card', 'IDR', 'confirmed', 5000000, '2026-11-01T00:00:00Z', '2026-11-01T00:00:00Z');

		INSERT INTO senders (id, email, role_hint, default_account_id, enabled, backfill_from_utc, last_internal_date_ms)
		VALUES
		  ('s-bca', 'bca@bca.co.id', 'auto', 'acct-bca', 1, '2026-10-01T00:00:00Z', ?),
		  ('s-jenius', 'jenius_noreply@smbci.com', 'auto', NULL, 1, '2026-10-01T00:00:00Z', 1795520000000),
		  ('s-grab', 'no-reply@grab.com', 'auto', NULL, 1, '2026-10-01T00:00:00Z', 1795520000000);
	`, bcaCursor).Error; err != nil {
		t.Fatalf("seed senders/accounts: %v", err)
	}

	type skippedFixture struct {
		emailID     string
		senderID    string
		from        string
		subject     string
		cleanedText string
		internalMs  int64
		receivedUtc string
		rawJSON     string
	}
	fixtures := []skippedFixture{
		{
			emailID:     "01b8eb35-fb08-421f-b683-f735db694b6c",
			senderID:    "s-bca",
			from:        "bca@bca.co.id",
			subject:     "Internet Transaction Journal",
			cleanedText: "Hello MUHAMMAD FARIZ,\nTransaction Date\n:\n24 Nov 2026 11:34:40\nTransfer Type\n:\nTransfer to BCA Account\nAmount\n:\nRp 150.000",
			internalMs:  1795494880000,
			receivedUtc: "2026-11-24T04:34:40Z",
			rawJSON:     `{"email_kind":"non_transaction","amount_decimal":"150000","currency":"IDR","transaction_date":"2026-11-24 11:34:40+07:00","merchant":"DINAR","category":"Other"}`,
		},
		{
			emailID:     "5ac46a86-0d1e-4f29-99ef-cc8f9882cace",
			senderID:    "s-bca",
			from:        "BCA <BCA@BCA.CO.ID>",
			subject:     "Internet Transaction Journal",
			cleanedText: "Hello MUHAMMAD FARIZ,\nTransaction Date\n:\n24 Nov 2026 19:42:34\nTransaction Type\n:\nQRIS Payment\nAmount\n:\nRp 45.000",
			internalMs:  1795524154000,
			receivedUtc: "2026-11-24T12:42:34Z",
			rawJSON:     `{"email_kind":"non_transaction","amount_decimal":"45000","currency":"IDR","transaction_date":"2026-11-24 19:42:34+07:00","merchant":"Ayam Keprabon Express","category":"Dining Out","payment_method":"BCA QRIS"}`,
		},
		{
			emailID:     "eb26eaca-ce81-4f8d-bedc-f13d4fa3456d",
			senderID:    "s-bca",
			from:        "bca@bca.co.id",
			subject:     "Internet Transaction Journal",
			cleanedText: "Hello MUHAMMAD FARIZ,\nTransaction Date\n:\n24 Nov 2026 19:38:41\nTransaction Type\n:\nQRIS Payment\nAmount\n:\nRp 75.000",
			internalMs:  1795523921000,
			receivedUtc: "2026-11-24T12:38:41Z",
			rawJSON:     `{"email_kind":"non_transaction","amount_decimal":"75000","currency":"IDR","transaction_date":"2026-11-24 19:38:41+07:00","merchant":"TIONG INDOGROSIR -HO","category":"Groceries","payment_method":"BCA QRIS"}`,
		},
		{
			emailID:     "9e83e37c-d03a-4ff2-a1d3-eb3bed1202ed",
			senderID:    "s-jenius",
			from:        "jenius_noreply@smbci.com",
			subject:     "You've transferred money from your Jenius account",
			cleanedText: "Hi Muhammad,\nYou have just transferred IDR100,000 to ANKIE ERTMA RAHFREEL.",
			internalMs:  1795491000000,
			receivedUtc: "2026-11-24T03:30:00Z",
			rawJSON:     `{"email_kind":"non_transaction","amount_decimal":"100000","currency":"IDR","transaction_date":"2026-11-24","transaction_time":"10:30:00","timezone_offset":"+07:00","merchant":null,"category":null}`,
		},
		{
			emailID:     "f9e81377-d39a-4bec-a3b5-67366146df45",
			senderID:    "s-grab",
			from:        "no-reply@grab.com",
			subject:     "Your Grab E-Receipt",
			cleanedText: "Grab Tips E-Receipt\nGrabFood\nRp 5.000",
			internalMs:  1795461240000,
			receivedUtc: "2026-11-23T13:14:00Z",
			rawJSON:     `{"email_kind":"non_transaction","amount_decimal":"5000","currency":"IDR","transaction_date":"2026-11-23T20:14:00+07:00","merchant":"Grab Tip","category":"Transport"}`,
		},
	}

	for _, f := range fixtures {
		if err := gdb.Exec(`
			INSERT INTO raw_emails (id, gmail_message_id, sender_id, from_address, subject, cleaned_text, internal_date_ms, received_at_utc, status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'non_transaction')`,
			f.emailID, "gm-"+f.emailID, f.senderID, f.from, f.subject, f.cleanedText, f.internalMs, f.receivedUtc,
		).Error; err != nil {
			t.Fatalf("seed raw_email %s: %v", f.emailID, err)
		}
		var raw llm.RawExtraction
		if err := json.Unmarshal([]byte(f.rawJSON), &raw); err != nil {
			t.Fatal(err)
		}
		draft, err := llm.NormalizeExtractedResult(&raw, f.rawJSON, llm.ExtractInput{
			EmailID: f.emailID, ReceivedAtUtc: f.receivedUtc, UserTimezone: "Asia/Jakarta",
		})
		if err != nil {
			t.Fatal(err)
		}
		sig := domain.DeriveEmailTemplateSignature(f.from, f.subject, f.cleanedText)
		extraction := db.EmailExtraction{
			ID: "ext-" + f.emailID, EmailID: f.emailID, EmailKind: "non_transaction",
			ExtractionModel: "qwen2.5", RawLlmJSON: f.rawJSON,
			TriageSignatureKey: &sig.SignatureKey, TriageSubjectPattern: &sig.SubjectPattern, TriageBodyCue: &sig.BodyCue,
		}
		if draft.AmountMinor > 0 && draft.Currency != "" {
			ghost := "ghost"
			extraction.AmountMinor, extraction.Currency = &draft.AmountMinor, &draft.Currency
			extraction.OccurredAtUtc, extraction.MerchantRaw = &draft.OccurredAtUtc, &draft.MerchantRaw
			extraction.Category, extraction.PaymentMethodRaw = &draft.Category, draft.PaymentMethodRaw
			extraction.TriageState = &ghost
		}
		if err := gdb.Create(&extraction).Error; err != nil {
			t.Fatalf("seed extraction %s: %v", f.emailID, err)
		}
	}

	listBefore, err := domain.ListTransactions(gdb, domain.ListTransactionsParams{Month: "2026-11", Page: 1, PageSize: 25})
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if listBefore.TotalCount != 0 || len(listBefore.GhostRows) != 5 || listBefore.GhostInRangeCount != 5 || listBefore.TotalGhostCount != 5 {
		t.Fatalf("expected 0 txs and 5 ghost rows, got total=%d ghosts=%d inRange=%d totalGhost=%d",
			listBefore.TotalCount, len(listBefore.GhostRows), listBefore.GhostInRangeCount, listBefore.TotalGhostCount)
	}
	summaryBefore, err := domain.GetMonthlySpendingSummary(gdb, "2026-11")
	if err != nil || summaryBefore.TotalSpentMinor != 0 {
		t.Fatalf("expected zero monthly spend while all skipped are Ghost Rows, got %+v err=%v", summaryBefore, err)
	}
	accountsBefore, err := domain.ListAccounts(gdb)
	if err != nil || len(accountsBefore) == 0 || accountsBefore[0].ComputedBalanceMinor == nil || *accountsBefore[0].ComputedBalanceMinor != 5000000 {
		t.Fatalf("expected BCA balance untouched at 5000000, got %+v err=%v", accountsBefore, err)
	}

	// Promote one BCA QRIS Ghost Row with zero LLM server needed.
	verdict, err := domain.ApplyTriageVerdict(gdb, domain.ApplyTriageVerdictInput{
		EmailID:   "5ac46a86-0d1e-4f29-99ef-cc8f9882cace",
		Action:    "promote",
		StartDate: "2026-11-24",
		EndDate:   "2026-11-24",
	})
	if err != nil {
		t.Fatalf("ApplyTriageVerdict(promote): %v", err)
	}
	if verdict.AffectedCount != 2 || verdict.OutOfRangeCount != 0 {
		t.Fatalf("expected 2 QRIS rows promoted in range, got affected=%d outOfRange=%d", verdict.AffectedCount, verdict.OutOfRangeCount)
	}

	var bcaAfter db.Sender
	if err := gdb.First(&bcaAfter, "id = ?", "s-bca").Error; err != nil {
		t.Fatal(err)
	}
	if bcaAfter.LastInternalDateMs == nil || *bcaAfter.LastInternalDateMs != bcaCursor {
		t.Fatalf("BCA last_internal_date_ms changed unexpectedly: got %v want %d", bcaAfter.LastInternalDateMs, bcaCursor)
	}

	listAfterPromote, err := domain.ListTransactions(gdb, domain.ListTransactionsParams{Month: "2026-11", Page: 1, PageSize: 25})
	if err != nil {
		t.Fatal(err)
	}
	if listAfterPromote.TotalCount != 2 || len(listAfterPromote.GhostRows) != 3 {
		t.Fatalf("expected 2 imported transactions and 3 remaining ghost rows (including BCA transfer), got txs=%d ghosts=%d",
			listAfterPromote.TotalCount, len(listAfterPromote.GhostRows))
	}
	hasTransferGhost := false
	for _, g := range listAfterPromote.GhostRows {
		if g.EmailID == "01b8eb35-fb08-421f-b683-f735db694b6c" {
			hasTransferGhost = true
		}
	}
	if !hasTransferGhost {
		t.Fatalf("expected BCA transfer email to remain a distinct Ghost Row")
	}
	for _, item := range listAfterPromote.Items {
		if item.Reviewed != 0 {
			t.Fatalf("promoted transaction %s must start unreviewed (0), got %d", item.ID, item.Reviewed)
		}
	}

	// Undo restores both QRIS emails to ghost state and removes the learned policy.
	undoRes, err := domain.UndoTriageVerdict(gdb, domain.TriageUndoInput{UndoPayload: verdict.UndoPayload})
	if err != nil {
		t.Fatalf("UndoTriageVerdict: %v", err)
	}
	if !undoRes.Ok || undoRes.RestoredCount != 2 {
		t.Fatalf("expected 2 restored rows on undo, got %+v", undoRes)
	}

	listAfterUndo, err := domain.ListTransactions(gdb, domain.ListTransactionsParams{Month: "2026-11", Page: 1, PageSize: 25})
	if err != nil {
		t.Fatal(err)
	}
	if listAfterUndo.TotalCount != 0 || len(listAfterUndo.GhostRows) != 5 {
		t.Fatalf("expected 0 transactions and 5 ghost rows after undo, got txs=%d ghosts=%d", listAfterUndo.TotalCount, len(listAfterUndo.GhostRows))
	}
	var polCount int64
	gdb.Model(&db.SenderTriagePolicy{}).Count(&polCount)
	if polCount != 0 {
		t.Fatalf("expected zero policies after undo, got %d", polCount)
	}
}

func TestTriageIntegration_LearnedAdmitForUnknownBankFormat(t *testing.T) {
	gdb := openIntegrationDB(t, "intl-bank.db")

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
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(userPrompt, "Jordan") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"content": `{"email_kind":"non_transaction","amount_decimal":"15.50","currency":"USD","transaction_date":"2026-11-24","transaction_time":"14:00:00","timezone_offset":"+00:00","merchant":"Jordan","category":"Other","card_last4":null,"payment_method":"Zelle","bank_account_hint":null,"matched_account_id":null,"account_type_guess":"debit_card","reference_code":null}`}},
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"email_kind":"non_transaction","amount_decimal":null,"currency":null,"transaction_date":null,"transaction_time":null,"timezone_offset":null,"merchant":null,"category":null,"card_last4":null,"payment_method":null,"bank_account_hint":null,"matched_account_id":null,"account_type_guess":null,"reference_code":null}`}},
			},
		})
	}))
	defer srv.Close()

	configureIntegrationSettings(t, gdb, "America/New_York", "USD", srv.URL+"/v1")
	sender := seedSender(t, gdb, "s-intl", "alerts@intlbank.example", nil)

	// Seed initial Ghost Row ("You sent $42.00 via Zelle to Alex") and promote it.
	if err := gdb.Create(&db.RawEmail{
		ID: "zelle-alex", GmailMessageID: "gm-zelle-alex", SenderID: &sender.ID,
		FromAddress: "Alerts <alerts@intlbank.example>", Subject: "You sent $42.00 via Zelle to Alex",
		CleanedText: "Transfer confirmation: $42.00 sent via Zelle to Alex.", InternalDateMs: 1795500000000, ReceivedAtUtc: "2026-11-24T10:00:00Z", Status: "non_transaction",
	}).Error; err != nil {
		t.Fatal(err)
	}
	sigAlex := domain.DeriveEmailTemplateSignature("Alerts <alerts@intlbank.example>", "You sent $42.00 via Zelle to Alex", "Transfer confirmation: $42.00 sent via Zelle to Alex.")
	if err := gdb.Exec(`
		INSERT INTO email_extractions (
			id, email_id, email_kind, amount_minor, currency, occurred_at_utc, merchant_raw, category,
			payment_method_raw, extraction_model, raw_llm_json, triage_signature_key, triage_subject_pattern, triage_body_cue, triage_state
		) VALUES ('ext-zelle-alex', 'zelle-alex', 'non_transaction', 4200, 'USD', '2026-11-24T10:00:00Z', 'Alex', 'Other', 'Zelle', 'qwen2.5', '{}', ?, ?, ?, 'ghost')`,
		sigAlex.SignatureKey, sigAlex.SubjectPattern, sigAlex.BodyCue,
	).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := domain.ApplyTriageVerdict(gdb, domain.ApplyTriageVerdictInput{
		EmailID: "zelle-alex",
		Action:  "promote",
	}); err != nil {
		t.Fatalf("promote Zelle Ghost Row: %v", err)
	}

	// Subsequent sync receives a new Zelle alert and an unrelated security alert.
	for _, em := range []db.RawEmail{
		{
			ID: "zelle-jordan", GmailMessageID: "gm-zelle-jordan", SenderID: &sender.ID,
			FromAddress: "alerts@intlbank.example", Subject: "You sent $15.50 via Zelle to Jordan",
			CleanedText: "Transfer confirmation: $15.50 sent via Zelle to Jordan.", InternalDateMs: 1795510000000, ReceivedAtUtc: "2026-11-24T14:00:00Z", Status: "pending",
		},
		{
			ID: "intl-sec", GmailMessageID: "gm-intl-sec", SenderID: &sender.ID,
			FromAddress: "alerts@intlbank.example", Subject: "Security alert: new login",
			CleanedText: "A new device signed in to your profile.", InternalDateMs: 1795511000000, ReceivedAtUtc: "2026-11-24T14:10:00Z", Status: "pending",
		},
	} {
		if err := gdb.Create(&em).Error; err != nil {
			t.Fatal(err)
		}
	}

	runIntegrationExtractionWorker(t, gdb, "America/New_York", "USD", srv.URL+"/v1")

	res, err := domain.ListTransactions(gdb, domain.ListTransactionsParams{Month: "2026-11", Page: 1, PageSize: 25})
	if err != nil {
		t.Fatal(err)
	}
	if res.TotalCount != 2 {
		t.Fatalf("expected both Zelle alerts (Alex + Jordan) imported, got %d (%+v)", res.TotalCount, res.Items)
	}
	if len(res.GhostRows) != 0 {
		t.Fatalf("expected security alert not to surface as ghost row, got %+v", res.GhostRows)
	}
	var secExt db.EmailExtraction
	if err := gdb.First(&secExt, "email_id = ?", "intl-sec").Error; err != nil || secExt.TriageState != nil {
		t.Fatalf("expected zero-amount security alert to keep triage_state=NULL, got %+v err=%v", secExt, err)
	}
}

func TestTriageIntegration_IgnorePreLlmSkipSettingsDeleteAndRecover(t *testing.T) {
	gdb := openIntegrationDB(t, "grab-triage.db")

	var mu sync.Mutex
	var promptLog []string
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
		mu.Lock()
		promptLog = append(promptLog, userPrompt)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(userPrompt, "voucher"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"content": `{"email_kind":"non_transaction","amount_decimal":"25000","currency":"IDR","transaction_date":"2026-11-24","transaction_time":"13:00:00","timezone_offset":"+07:00","merchant":"Grab Compensation","category":"Other","card_last4":null,"payment_method":"GrabPay","bank_account_hint":null,"matched_account_id":null,"account_type_guess":"digital_wallet","reference_code":null}`}},
				},
			})
		case strings.Contains(userPrompt, "Grab Tips"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"content": `{"email_kind":"receipt","amount_decimal":"5000","currency":"IDR","transaction_date":"2026-11-24","transaction_time":"13:05:00","timezone_offset":"+07:00","merchant":"Grab Driver Tip","category":"Transport","card_last4":null,"payment_method":"GrabPay","bank_account_hint":null,"matched_account_id":null,"account_type_guess":"digital_wallet","reference_code":null}`}},
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"content": `{"email_kind":"receipt","amount_decimal":"65000","currency":"IDR","transaction_date":"2026-11-24","transaction_time":"13:02:00","timezone_offset":"+07:00","merchant":"GrabFood Kitchen","category":"Dining Out","card_last4":null,"payment_method":"GrabPay","bank_account_hint":null,"matched_account_id":null,"account_type_guess":"digital_wallet","reference_code":null}`}},
				},
			})
		}
	}))
	defer srv.Close()

	configureIntegrationSettings(t, gdb, "Asia/Jakarta", "IDR", srv.URL+"/v1")
	grabCursor := int64(1795400000000)
	sender := seedSender(t, gdb, "s-grab", "no-reply@grab.com", &grabCursor)

	// Seed initial post-LLM Ghost Row compensation voucher and ignore it.
	voucherSubj1 := "Ride Cover Plus: Sorry for the long wait. Here's your Rp11.000 voucher."
	voucherBody1 := "Ride Cover\nHi Nagib Fariz,\nEnjoy this compensation voucher."
	sigVoucher := domain.DeriveEmailTemplateSignature("no-reply@grab.com", voucherSubj1, voucherBody1)
	if err := gdb.Create(&db.RawEmail{
		ID: "grab-voucher-1", GmailMessageID: "gm-gv-1", SenderID: &sender.ID,
		FromAddress: "no-reply@grab.com", Subject: voucherSubj1, CleanedText: voucherBody1,
		InternalDateMs: 1795400000000, ReceivedAtUtc: "2026-11-23T08:00:00Z", Status: "non_transaction",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := gdb.Exec(`
		INSERT INTO email_extractions (
			id, email_id, email_kind, amount_minor, currency, occurred_at_utc, merchant_raw, category,
			extraction_model, raw_llm_json, triage_signature_key, triage_subject_pattern, triage_body_cue, triage_state
		) VALUES ('ext-gv-1', 'grab-voucher-1', 'non_transaction', 11000, 'IDR', '2026-11-23T08:00:00Z', 'Grab Voucher', 'Other', 'qwen2.5', '{}', ?, ?, ?, 'ghost')`,
		sigVoucher.SignatureKey, sigVoucher.SubjectPattern, sigVoucher.BodyCue,
	).Error; err != nil {
		t.Fatal(err)
	}

	ignoreVerdict, err := domain.ApplyTriageVerdict(gdb, domain.ApplyTriageVerdictInput{
		EmailID: "grab-voucher-1",
		Action:  "ignore",
	})
	if err != nil {
		t.Fatalf("ignore initial voucher: %v", err)
	}

	// Next sync receives: another Ride Cover voucher, a GrabFood receipt, and a Grab Tips receipt.
	for _, em := range []db.RawEmail{
		{
			ID: "grab-voucher-2", GmailMessageID: "gm-gv-2", SenderID: &sender.ID,
			FromAddress:    "no-reply@grab.com",
			Subject:        "Ride Cover Plus: Sorry for the long wait. Here's your Rp25.000 voucher.",
			CleanedText:    "Ride Cover\nHi Nagib Fariz,\nEnjoy this Rp25.000 voucher.",
			InternalDateMs: 1795500001000, ReceivedAtUtc: "2026-11-24T06:00:00Z", Status: "pending",
		},
		{
			ID: "grab-food-1", GmailMessageID: "gm-gf-1", SenderID: &sender.ID,
			FromAddress: "no-reply@grab.com", Subject: "Your Grab E-Receipt",
			CleanedText:    "Selamat menikmati makanan anda!\nGrabFood\nRp 65.000",
			InternalDateMs: 1795500002000, ReceivedAtUtc: "2026-11-24T06:02:00Z", Status: "pending",
		},
		{
			ID: "grab-tip-1", GmailMessageID: "gm-gt-1", SenderID: &sender.ID,
			FromAddress: "no-reply@grab.com", Subject: "Your Grab E-Receipt",
			CleanedText:    "Grab Tips E-Receipt\nDriver Tip\nRp 5.000",
			InternalDateMs: 1795500003000, ReceivedAtUtc: "2026-11-24T06:05:00Z", Status: "pending",
		},
	} {
		if err := gdb.Create(&em).Error; err != nil {
			t.Fatal(err)
		}
	}

	runIntegrationExtractionWorker(t, gdb, "Asia/Jakarta", "IDR", srv.URL+"/v1")

	mu.Lock()
	for _, p := range promptLog {
		if strings.Contains(p, "voucher") {
			mu.Unlock()
			t.Fatalf("learned ignore policy must skip LLM call for matching voucher, got prompt: %s", p)
		}
	}
	callCountAfterSync := len(promptLog)
	mu.Unlock()
	if callCountAfterSync != 2 {
		t.Fatalf("expected exactly 2 LLM calls (GrabFood + Grab Tips), got %d", callCountAfterSync)
	}

	var extPreSkip db.EmailExtraction
	if err := gdb.First(&extPreSkip, "email_id = ?", "grab-voucher-2").Error; err != nil {
		t.Fatal(err)
	}
	if extPreSkip.ExtractionModel != "policy:ignore" || extPreSkip.TriageState == nil || *extPreSkip.TriageState != "ignored" {
		t.Fatalf("expected pre-LLM policy:ignore extraction state, got %+v", extPreSkip)
	}

	// Deleting ignore rule in Settings returns post-LLM voucher to Ghost Rows while keeping pre-LLM voucher in ListIgnoredEmails.
	delRes, err := domain.DeleteTriagePolicy(gdb, ignoreVerdict.Policy.ID)
	if err != nil {
		t.Fatalf("DeleteTriagePolicy: %v", err)
	}
	if delRes.RestoredGhostCount != 1 {
		t.Fatalf("expected 1 post-LLM voucher restored to ghost, got %d", delRes.RestoredGhostCount)
	}

	ignoredList, err := domain.ListIgnoredEmails(gdb, domain.ListIgnoredEmailsParams{SenderEmail: "no-reply@grab.com"})
	if err != nil {
		t.Fatal(err)
	}
	if ignoredList.TotalCount != 1 || ignoredList.Items[0].EmailID != "grab-voucher-2" || !ignoredList.Items[0].PreLlmSkipped {
		t.Fatalf("expected only pre-LLM grab-voucher-2 in ListIgnoredEmails, got %+v", ignoredList)
	}

	// Recover pre-LLM skipped voucher via on-demand PromoteIgnoredEmail (mock LLM emits non_transaction + amount > 0).
	promoted, err := domain.PromoteIgnoredEmail(gdb, context.Background(), "grab-voucher-2")
	if err != nil {
		t.Fatalf("PromoteIgnoredEmail: %v", err)
	}
	if !promoted.UsedLlm || promoted.Transaction.AmountMinor != 25000 || promoted.Transaction.Reviewed != 0 {
		t.Fatalf("unexpected pre-LLM promoted result: %+v", promoted)
	}

	var grabAfter db.Sender
	gdb.First(&grabAfter, "id = ?", sender.ID)
	if grabAfter.LastInternalDateMs == nil || *grabAfter.LastInternalDateMs != grabCursor {
		t.Fatalf("Grab sync cursor must stay invariant at %d, got %v", grabCursor, grabAfter.LastInternalDateMs)
	}

	// Future matching voucher now reaches the LLM again because the ignore policy was deleted.
	if err := gdb.Create(&db.RawEmail{
		ID: "grab-voucher-3", GmailMessageID: "gm-gv-3", SenderID: &sender.ID,
		FromAddress:    "no-reply@grab.com",
		Subject:        "Ride Cover Plus: Sorry for the long wait. Here's your Rp30.000 voucher.",
		CleanedText:    "Ride Cover\nHi Nagib Fariz,\nEnjoy this Rp30.000 voucher.",
		InternalDateMs: 1795500004000, ReceivedAtUtc: "2026-11-24T07:00:00Z", Status: "pending",
	}).Error; err != nil {
		t.Fatal(err)
	}
	runIntegrationExtractionWorker(t, gdb, "Asia/Jakarta", "IDR", srv.URL+"/v1")

	var extVoucher3 db.EmailExtraction
	if err := gdb.First(&extVoucher3, "email_id = ?", "grab-voucher-3").Error; err != nil {
		t.Fatal(err)
	}
	if extVoucher3.ExtractionModel == "policy:ignore" {
		t.Fatalf("expected voucher after policy delete to evaluate via LLM instead of policy:ignore")
	}
}

func TestTriageIntegration_DeleteTransactionDoesNotCreateIgnorePolicy(t *testing.T) {
	gdb := openIntegrationDB(t, "jenius-delete.db")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"email_kind":"card_charge","amount_decimal":"85000","currency":"IDR","transaction_date":"2026-11-24","transaction_time":"16:00:00","timezone_offset":"+07:00","merchant":"Tiket Store","category":"Shopping","card_last4":"6039","payment_method":"Jenius Credit Card","bank_account_hint":null,"matched_account_id":null,"account_type_guess":"credit_card","reference_code":null}`}},
			},
		})
	}))
	defer srv.Close()

	configureIntegrationSettings(t, gdb, "Asia/Jakarta", "IDR", srv.URL+"/v1")
	sender := seedSender(t, gdb, "s-jenius", "jenius_noreply@smbci.com", nil)

	if err := gdb.Create(&db.RawEmail{
		ID: "1e4bfdde-38a8-4ae3-aa41-492c5bfb7706", GmailMessageID: "gm-jc-1", SenderID: &sender.ID,
		FromAddress: "jenius_noreply@smbci.com", Subject: "Credit Card Transaction Notification",
		CleanedText:    "You used card 437896******6039 for Rp 85.000",
		InternalDateMs: 1795510000000, ReceivedAtUtc: "2026-11-24T09:00:00Z", Status: "pending",
	}).Error; err != nil {
		t.Fatal(err)
	}
	runIntegrationExtractionWorker(t, gdb, "Asia/Jakarta", "IDR", srv.URL+"/v1")

	list1, err := domain.ListTransactions(gdb, domain.ListTransactionsParams{Month: "2026-11", Page: 1, PageSize: 25})
	if err != nil || list1.TotalCount != 1 {
		t.Fatalf("expected 1 imported Jenius charge, got %+v err=%v", list1, err)
	}

	if err := domain.DeleteTransaction(gdb, list1.Items[0].ID); err != nil {
		t.Fatalf("DeleteTransaction: %v", err)
	}

	var reDeleted db.RawEmail
	gdb.First(&reDeleted, "id = ?", "1e4bfdde-38a8-4ae3-aa41-492c5bfb7706")
	if reDeleted.Status != "dismissed" {
		t.Fatalf("expected deleted transaction source email status=dismissed, got %q", reDeleted.Status)
	}

	policies, err := domain.ListTriagePolicies(gdb, domain.ListTriagePoliciesParams{})
	if err != nil || len(policies) != 0 {
		t.Fatalf("expected 0 triage policies created by transaction delete, got %+v err=%v", policies, err)
	}

	ignored, err := domain.ListIgnoredEmails(gdb, domain.ListIgnoredEmailsParams{})
	if err != nil || ignored.TotalCount != 0 {
		t.Fatalf("expected deleted transaction absent from ignored emails, got %+v err=%v", ignored, err)
	}

	// Second Jenius charge imports normally.
	if err := gdb.Create(&db.RawEmail{
		ID: "jenius-charge-2", GmailMessageID: "gm-jc-2", SenderID: &sender.ID,
		FromAddress: "jenius_noreply@smbci.com", Subject: "Credit Card Transaction Notification",
		CleanedText:    "You used card 437896******6039 for Rp 85.000",
		InternalDateMs: 1795515000000, ReceivedAtUtc: "2026-11-24T10:00:00Z", Status: "pending",
	}).Error; err != nil {
		t.Fatal(err)
	}
	runIntegrationExtractionWorker(t, gdb, "Asia/Jakarta", "IDR", srv.URL+"/v1")

	list2, err := domain.ListTransactions(gdb, domain.ListTransactionsParams{Month: "2026-11", Page: 1, PageSize: 25})
	if err != nil || list2.TotalCount != 1 || len(list2.GhostRows) != 0 {
		t.Fatalf("expected subsequent Jenius charge imported cleanly and dismissed charge hidden, got txs=%d ghosts=%d err=%v",
			list2.TotalCount, len(list2.GhostRows), err)
	}
}

func TestTriageIntegration_HistoricalGhostPromotionPreservesReconciledBalance(t *testing.T) {
	gdb := openIntegrationDB(t, "reconcile-anchor.db")
	configureIntegrationSettings(t, gdb, "Asia/Jakarta", "IDR", "")

	acctID := "acct-reconciled"
	if err := gdb.Create(&db.Account{
		ID:                    acctID,
		Name:                  "Main Debit",
		Type:                  "debit_card",
		Currency:              "IDR",
		Status:                "confirmed",
		InitialBalanceMinor:   nil,
		InitialBalanceAsOfUtc: "2026-11-01T00:00:00Z",
		CreatedAtUtc:          "2026-11-01T00:00:00Z",
	}).Error; err != nil {
		t.Fatal(err)
	}
	// Reconcile balance at 900,000 IDR on 2026-11-24T00:00:00Z.
	if err := gdb.Create(&db.AccountReconciliation{
		ID:                   "rec-anchor",
		AccountID:            acctID,
		Currency:             "IDR",
		ComputedBalanceMinor: 900000,
		ActualBalanceMinor:   900000,
		AdjustmentMinor:      0,
		RecordedAtUtc:        "2026-11-24T00:00:00Z",
	}).Error; err != nil {
		t.Fatal(err)
	}
	// Post-reconciliation spend of 10,000 IDR brings current computed balance to 890,000 IDR.
	if err := gdb.Create(&db.Transaction{
		ID:            "tx-post-rec",
		SourceType:    "manual",
		AccountID:     &acctID,
		AmountMinor:   10000,
		Currency:      "IDR",
		OccurredAtUtc: "2026-11-24T08:00:00Z",
		Merchant:      "Post Rec Coffee",
		CategoryID:    "cat-dining-out",
		Reviewed:      1,
		CreatedAtUtc:  "2026-11-24T08:00:00Z",
		UpdatedAtUtc:  "2026-11-24T08:00:00Z",
	}).Error; err != nil {
		t.Fatal(err)
	}

	sender := seedSender(t, gdb, "s-bca", "bca@bca.co.id", nil)
	if err := gdb.Model(&db.Sender{}).Where("id = ?", sender.ID).Update("default_account_id", acctID).Error; err != nil {
		t.Fatal(err)
	}
	histSubj := "Internet Transaction Journal"
	histBody := "Transaction Type\n:\nQRIS Payment\nRp 45.000"
	sigHist := domain.DeriveEmailTemplateSignature("bca@bca.co.id", histSubj, histBody)
	if err := gdb.Create(&db.RawEmail{
		ID: "ghost-before-rec", GmailMessageID: "gm-hist-rec", SenderID: &sender.ID,
		FromAddress: "bca@bca.co.id", Subject: histSubj, CleanedText: histBody,
		InternalDateMs: 1795150000000, ReceivedAtUtc: "2026-11-20T05:00:00Z", Status: "non_transaction",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := gdb.Exec(`
		INSERT INTO email_extractions (
			id, email_id, email_kind, amount_minor, currency, occurred_at_utc, merchant_raw, category,
			extraction_model, raw_llm_json, triage_signature_key, triage_subject_pattern, triage_body_cue, triage_state
		) VALUES ('ext-hist-rec', 'ghost-before-rec', 'non_transaction', 45000, 'IDR', '2026-11-20T05:00:00Z', 'Historical Stall', 'Dining Out', 'qwen2.5', '{}', ?, ?, ?, 'ghost')`,
		sigHist.SignatureKey, sigHist.SubjectPattern, sigHist.BodyCue,
	).Error; err != nil {
		t.Fatal(err)
	}

	summaryBefore, err := domain.GetMonthlySpendingSummary(gdb, "2026-11")
	if err != nil || summaryBefore.TotalSpentMinor != 10000 {
		t.Fatalf("expected initial monthly spend 10000, got %+v err=%v", summaryBefore, err)
	}

	if _, err := domain.ApplyTriageVerdict(gdb, domain.ApplyTriageVerdictInput{
		EmailID: "ghost-before-rec",
		Action:  "promote",
	}); err != nil {
		t.Fatalf("promote historical Ghost Row: %v", err)
	}

	summaryAfter, err := domain.GetMonthlySpendingSummary(gdb, "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if summaryAfter.TotalSpentMinor != 55000 {
		t.Fatalf("expected monthly spend to increase to 55000 after historical promotion, got %d", summaryAfter.TotalSpentMinor)
	}
	accts, err := domain.ListAccounts(gdb)
	if err != nil {
		t.Fatal(err)
	}
	idx := slices.IndexFunc(accts, func(a domain.AccountSummary) bool { return a.ID == acctID })
	if idx < 0 || accts[idx].ComputedBalanceMinor == nil || *accts[idx].ComputedBalanceMinor != 890000 {
		t.Fatalf("expected current account balance invariant at 890000, got %+v", accts)
	}
}
