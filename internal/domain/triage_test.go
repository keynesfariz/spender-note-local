package domain

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"

	"portspendernote/internal/db"
	"portspendernote/internal/llm"
)

func seedSenderWithCursor(t *testing.T, gdb *gorm.DB, id, email string, cursorMs int64, roleHint string) db.Sender {
	t.Helper()
	s := db.Sender{
		ID:                 id,
		Email:              email,
		RoleHint:           roleHint,
		Enabled:            1,
		BackfillFromUtc:    "2026-10-01T00:00:00Z",
		LastInternalDateMs: &cursorMs,
	}
	if err := gdb.Create(&s).Error; err != nil {
		t.Fatalf("seed sender: %v", err)
	}
	return s
}

func seedSkippedEmail(t *testing.T, gdb *gorm.DB, id string, senderID *string, from, subject, body string, internalMs int64, receivedUtc, extractionModel, rawJSON string) {
	t.Helper()
	if err := gdb.Create(&db.RawEmail{
		ID:             id,
		GmailMessageID: "gm-" + id,
		SenderID:       senderID,
		FromAddress:    from,
		Subject:        subject,
		CleanedText:    body,
		InternalDateMs: internalMs,
		ReceivedAtUtc:  receivedUtc,
		Status:         "non_transaction",
	}).Error; err != nil {
		t.Fatalf("seed raw_email %s: %v", id, err)
	}
	var raw llm.RawExtraction
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		t.Fatal(err)
	}
	raw.EmailKind = "non_transaction"
	categories, err := ListCategoryNames(gdb)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := llm.NormalizeExtractedResult(&raw, rawJSON, llm.ExtractInput{
		ReceivedAtUtc: receivedUtc, UserTimezone: "Asia/Jakarta", Categories: categories,
	})
	if err != nil {
		t.Fatal(err)
	}
	sig := DeriveEmailTemplateSignature(from, subject, body)
	extraction := db.EmailExtraction{
		ID: "ext-" + id, EmailID: id, EmailKind: "non_transaction",
		ExtractionModel: extractionModel, RawLlmJSON: rawJSON,
		TriageSignatureKey: &sig.SignatureKey, TriageSubjectPattern: &sig.SubjectPattern, TriageBodyCue: &sig.BodyCue,
	}
	if draft.AmountMinor > 0 && draft.Currency != "" {
		ghost := "ghost"
		extraction.AmountMinor, extraction.Currency = &draft.AmountMinor, &draft.Currency
		extraction.OccurredAtUtc = strPtrOrNil(draft.OccurredAtUtc)
		extraction.MerchantRaw = strPtrOrNil(draft.MerchantRaw)
		extraction.Category = &draft.Category
		extraction.PaymentMethodRaw = draft.PaymentMethodRaw
		extraction.CardLast4 = draft.CardLast4
		extraction.AccountTypeGuess = draft.AccountTypeGuess
		extraction.TriageState = &ghost
	}
	if err := gdb.Create(&extraction).Error; err != nil {
		t.Fatalf("seed extraction %s: %v", id, err)
	}
}

func TestListIgnoredEmails_FilteringAndPagination(t *testing.T) {
	gdb := openDomainTestDB(t)
	const receivedAt = "2026-11-24T05:10:00Z"
	for _, row := range []struct {
		id, sender, subject, body, rawStatus, triageState string
	}{
		{"a", `"Shop" <SHOP@example.com>`, "ÉTÉ %_", "", "non_transaction", "ignored"},
		{"b", "bad, header <SHOP@example.com>", "ÉTÉ %_", strings.Repeat("large email body ", 10000), "non_transaction", "ignored"},
		{"c", "another@example.com", "ÉTÉ XX", "ÉTÉ %_", "non_transaction", "ignored"},
		{"d", "other@example.com", "ÉTÉ %_", "excluded by raw status", "extracted", "ignored"},
		{"e", "other@example.com", "ÉTÉ %_", "excluded by triage state", "non_transaction", "promoted"},
	} {
		seedSkippedEmail(t, gdb, row.id, nil, row.sender, row.subject, row.body, 1795501000000, receivedAt, "policy:ignore", "{}")
		if err := gdb.Exec("UPDATE raw_emails SET status = ? WHERE id = ?", row.rawStatus, row.id).Error; err != nil {
			t.Fatal(err)
		}
		if err := gdb.Exec("UPDATE email_extractions SET triage_state = ?, triage_body_cue = '' WHERE email_id = ?", row.triageState, row.id).Error; err != nil {
			t.Fatal(err)
		}
	}

	t.Run("literal unicode search sorted by id for equal timestamps", func(t *testing.T) {
		for page, wantID := range []string{"b", "a", ""} {
			result, err := ListIgnoredEmails(gdb, ListIgnoredEmailsParams{Search: " été %_ ", Page: page + 1, PageSize: 1})
			if err != nil {
				t.Fatal(err)
			}
			if result.TotalCount != 2 || !slices.Equal(result.AvailableSenders, []string{"another@example.com", "shop@example.com"}) {
				t.Fatalf("page %d: total=%d senders=%v", page+1, result.TotalCount, result.AvailableSenders)
			}
			if wantID == "" {
				if len(result.Items) != 0 {
					t.Fatalf("expected empty page, got %+v", result.Items)
				}
				continue
			}
			if len(result.Items) != 1 || result.Items[0].EmailID != wantID || result.Items[0].SenderEmail != "shop@example.com" {
				t.Fatalf("page %d: expected %s from shop@example.com, got %+v", page+1, wantID, result.Items)
			}
		}
	})

	t.Run("sender options are global", func(t *testing.T) {
		result, err := ListIgnoredEmails(gdb, ListIgnoredEmailsParams{SenderEmail: "Another <ANOTHER@example.com>"})
		if err != nil {
			t.Fatal(err)
		}
		if result.TotalCount != 1 || len(result.Items) != 1 || result.Items[0].EmailID != "c" ||
			!slices.Equal(result.AvailableSenders, []string{"another@example.com", "shop@example.com"}) {
			t.Fatalf("unexpected sender-filtered result: %+v", result)
		}
	})

	t.Run("only the legacy list includes inline bodies", func(t *testing.T) {
		for _, summary := range []bool{false, true} {
			result, err := ListIgnoredEmails(gdb, ListIgnoredEmailsParams{
				SenderEmail: "shop@example.com",
				Page:        2,
				PageSize:    1,
				SummaryOnly: summary,
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Items) != 1 || result.Items[0].EmailID != "a" || !result.Items[0].PreLlmSkipped {
				t.Fatalf("expected the older pre-LLM ignored item, got %+v", result.Items)
			}
			data, err := json.Marshal(result.Items[0])
			if err != nil {
				t.Fatal(err)
			}
			if summary && strings.Contains(string(data), `"cleaned_text"`) {
				t.Fatalf("summary exposed cleaned text: %s", data)
			}
			if !summary && !strings.Contains(string(data), `"cleaned_text":""`) {
				t.Fatalf("legacy response omitted empty cleaned text: %s", data)
			}
		}
	})

	t.Run("body is available only for currently ignored emails", func(t *testing.T) {
		body, err := GetIgnoredEmailBody(gdb, "b")
		if err != nil || body != strings.Repeat("large email body ", 10000) {
			t.Fatalf("expected intact cleaned body, got %d bytes and error %v", len(body), err)
		}
		for _, id := range []string{"d", "e", "missing"} {
			var notFound *NotFoundError
			if _, err := GetIgnoredEmailBody(gdb, id); !errors.As(err, &notFound) {
				t.Fatalf("%s: expected NotFoundError, got %v", id, err)
			}
		}
	})

	t.Run("large page returns empty rather than overflowing offset", func(t *testing.T) {
		result, err := ListIgnoredEmails(gdb, ListIgnoredEmailsParams{Page: math.MaxInt/25 + 2})
		if err != nil {
			t.Fatal(err)
		}
		if result.TotalCount != 3 || len(result.Items) != 0 {
			t.Fatalf("unexpected out-of-range result: %+v", result)
		}
	})
}

func TestApplyAndUndoTriageVerdict_PromoteRippleDisplayNameCursorUndoAndConflicts(t *testing.T) {
	gdb := openDomainTestDB(t)
	cursorMs := int64(1795525000000)
	bca := seedSenderWithCursor(t, gdb, "s-bca", "bca@bca.co.id", cursorMs, "auto")
	acct := seedTestAccount(t, gdb, "acct-bca", "BCA Account", "IDR")
	if err := gdb.Model(&db.Sender{}).Where("id = ?", bca.ID).Update("default_account_id", acct).Error; err != nil {
		t.Fatal(err)
	}

	seedSkippedEmail(
		t, gdb, "qris-1", &bca.ID, "BCA <BCA@BCA.CO.ID>", "Internet Transaction Journal",
		"Hello,\nTransaction Type\n:\nQRIS Payment", 1795524154000, "2026-11-24T12:42:34Z", "qwen",
		`{"email_kind":"non_transaction","amount_decimal":"45000","currency":"IDR","transaction_date":"2026-11-24","transaction_time":"19:42:34","timezone_offset":"+07:00","merchant":"Ayam Keprabon","category":"Dining Out","payment_method":"BCA QRIS"}`,
	)
	seedSkippedEmail(
		t, gdb, "qris-2", &bca.ID, "bca@bca.co.id", "Internet Transaction Journal",
		"Hello,\nTransaction Type\n:\nQRIS Payment", 1795523921000, "2026-11-24T12:38:41Z", "qwen",
		`{"email_kind":"non_transaction","amount_decimal":"75000","currency":"IDR","transaction_date":"2026-11-24","transaction_time":"19:38:41","timezone_offset":"+07:00","merchant":"Indogrosir","category":"Groceries","payment_method":"BCA QRIS"}`,
	)
	seedSkippedEmail(
		t, gdb, "qris-older-day", &bca.ID, "bca@bca.co.id", "Internet Transaction Journal",
		"Hello,\nTransaction Type\n:\nQRIS Payment", 1795420000000, "2026-11-20T12:00:00Z", "qwen",
		`{"email_kind":"non_transaction","amount_decimal":"12000","currency":"IDR","transaction_date":"2026-11-20","transaction_time":"19:00:00","timezone_offset":"+07:00","merchant":"Older Coffee","category":"Dining Out"}`,
	)
	seedSkippedEmail(
		t, gdb, "qris-dismissed", &bca.ID, "bca@bca.co.id", "Internet Transaction Journal",
		"Hello,\nTransaction Type\n:\nQRIS Payment", 1795520000000, "2026-11-24T10:00:00Z", "qwen",
		`{"email_kind":"non_transaction","amount_decimal":"20000","currency":"IDR","transaction_date":"2026-11-24T10:00:00Z","merchant":"Dismissed Coffee","category":"Dining Out"}`,
	)
	if err := gdb.Model(&db.RawEmail{}).Where("id = ?", "qris-dismissed").Update("status", "dismissed").Error; err != nil {
		t.Fatal(err)
	}
	seedSkippedEmail(
		t, gdb, "bca-transfer", &bca.ID, "bca@bca.co.id", "Internet Transaction Journal",
		"Hello,\nTransfer Type\n:\nTransfer to BCA Account", 1795494880000, "2026-11-24T04:34:40Z", "qwen",
		`{"email_kind":"non_transaction","amount_decimal":"150000","currency":"IDR","transaction_date":"2026-11-24","transaction_time":"11:34:40","timezone_offset":"+07:00","merchant":null,"category":null}`,
	)

	verdict, err := ApplyTriageVerdict(gdb, ApplyTriageVerdictInput{
		EmailID:   "qris-1",
		Action:    "promote",
		StartDate: "2026-11-24",
		EndDate:   "2026-11-24",
	})
	if err != nil {
		t.Fatalf("ApplyTriageVerdict(promote): %v", err)
	}
	if verdict.AffectedCount != 3 || verdict.OutOfRangeCount != 1 {
		t.Fatalf("expected 3 promoted rows (1 out of 2026-11-24 range), got affected=%d outOfRange=%d", verdict.AffectedCount, verdict.OutOfRangeCount)
	}
	if verdict.Policy.Action != "admit" || verdict.Policy.SenderEmail != "bca@bca.co.id" {
		t.Fatalf("unexpected saved policy: %+v", verdict.Policy)
	}

	var bcaAfter db.Sender
	gdb.First(&bcaAfter, "id = ?", bca.ID)
	if bcaAfter.LastInternalDateMs == nil || *bcaAfter.LastInternalDateMs != cursorMs {
		t.Fatalf("sender sync cursor must stay %d, got %v", cursorMs, bcaAfter.LastInternalDateMs)
	}

	var dismissedEmail db.RawEmail
	gdb.First(&dismissedEmail, "id = ?", "qris-dismissed")
	if dismissedEmail.Status != "dismissed" {
		t.Fatalf("dismissed email must remain untouched, got %q", dismissedEmail.Status)
	}

	listAfterPromote, err := ListTransactions(gdb, ListTransactionsParams{Month: "2026-11", Page: 1, PageSize: 25})
	if err != nil {
		t.Fatal(err)
	}
	if listAfterPromote.TotalCount != 3 || len(listAfterPromote.GhostRows) != 1 || listAfterPromote.GhostRows[0].EmailID != "bca-transfer" {
		t.Fatalf("expected 3 imported + 1 remaining bca-transfer ghost, got total=%d ghosts=%+v", listAfterPromote.TotalCount, listAfterPromote.GhostRows)
	}
	for _, item := range listAfterPromote.Items {
		if item.Reviewed != 0 {
			t.Fatalf("promoted transaction %s must land unreviewed (0), got %d", item.ID, item.Reviewed)
		}
	}

	// Undo immediately restores policy + all 3 Ghost Rows.
	undoRes, err := UndoTriageVerdict(gdb, TriageUndoInput{UndoPayload: verdict.UndoPayload})
	if err != nil {
		t.Fatalf("UndoTriageVerdict: %v", err)
	}
	if !undoRes.Ok || undoRes.RestoredCount != 3 {
		t.Fatalf("unexpected undo result: %+v", undoRes)
	}
	var polCount int64
	gdb.Model(&db.SenderTriagePolicy{}).Where("id = ?", verdict.Policy.ID).Count(&polCount)
	if polCount != 0 {
		t.Fatalf("expected created policy removed on undo, found %d", polCount)
	}

	// Null-merchant Ghost Row promotion uses subject fallback without LLM.
	transferVerdict, err := ApplyTriageVerdict(gdb, ApplyTriageVerdictInput{
		EmailID: "bca-transfer",
		Action:  "promote",
	})
	if err != nil {
		t.Fatalf("promote null-merchant ghost: %v", err)
	}
	var promotedTransfer db.Transaction
	gdb.First(&promotedTransfer, "id = ?", transferVerdict.CreatedTransactionIDs[0])
	if promotedTransfer.Merchant != "Internet Transaction Journal" || promotedTransfer.Reviewed != 0 {
		t.Fatalf("expected subject fallback title & unreviewed, got %+v", promotedTransfer)
	}

	// Edited promoted tx blocks Undo with 409 ConflictError.
	newMerch := "Edited Merchant"
	if _, err := UpdateTransaction(gdb, promotedTransfer.ID, UpdateTransactionInput{Merchant: &newMerch}); err != nil {
		t.Fatal(err)
	}
	_, undoErr := UndoTriageVerdict(gdb, TriageUndoInput{UndoPayload: transferVerdict.UndoPayload})
	var conflictErr *ConflictError
	if !errors.As(undoErr, &conflictErr) {
		t.Fatalf("expected ConflictError after editing promoted tx before Undo, got %T: %v", undoErr, undoErr)
	}
}

func TestUndoTriageVerdict_ProtectsDefaultAccountAndIgnoreRecoveryConflicts(t *testing.T) {
	gdb := openDomainTestDB(t)
	sender := seedSenderWithCursor(t, gdb, "s-prov", "wallet@pay.example", 1000, "auto")

	seedSkippedEmail(
		t, gdb, "prov-ghost", &sender.ID, "wallet@pay.example", "Wallet Payment",
		"Paid IDR 25000", 1795520000000, "2026-11-24T10:00:00Z", "qwen",
		`{"email_kind":"non_transaction","amount_decimal":"25000","currency":"IDR","card_last4":"7788","account_type_guess":"credit_card","merchant":"Cafe"}`,
	)

	v, err := ApplyTriageVerdict(gdb, ApplyTriageVerdictInput{EmailID: "prov-ghost", Action: "promote"})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.UndoPayload.CreatedProvisionalAccountIDs) != 1 {
		t.Fatalf("expected 1 created provisional account in undo payload, got %v", v.UndoPayload.CreatedProvisionalAccountIDs)
	}
	provID := v.UndoPayload.CreatedProvisionalAccountIDs[0]

	// Point sender default_account_id to provisional account; Undo must keep it intact!
	if err := gdb.Model(&db.Sender{}).Where("id = ?", sender.ID).Update("default_account_id", provID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := UndoTriageVerdict(gdb, TriageUndoInput{UndoPayload: v.UndoPayload}); err != nil {
		t.Fatalf("Undo with sender reference failed: %v", err)
	}
	var acctCnt int64
	gdb.Model(&db.Account{}).Where("id = ?", provID).Count(&acctCnt)
	if acctCnt != 1 {
		t.Fatalf("expected provisional account referenced by sender.default_account_id to stay intact, got count=%d", acctCnt)
	}

	// Clear sender default_account_id and repeat promote+undo -> now unreferenced provisional account IS deleted.
	_ = gdb.Model(&db.Sender{}).Where("id = ?", sender.ID).Update("default_account_id", nil).Error
	_ = gdb.Where("id = ?", provID).Delete(&db.Account{}).Error
	v2, err := ApplyTriageVerdict(gdb, ApplyTriageVerdictInput{EmailID: "prov-ghost", Action: "promote"})
	if err != nil {
		t.Fatal(err)
	}
	provID2 := v2.UndoPayload.CreatedProvisionalAccountIDs[0]
	if _, err := UndoTriageVerdict(gdb, TriageUndoInput{UndoPayload: v2.UndoPayload}); err != nil {
		t.Fatal(err)
	}
	gdb.Model(&db.Account{}).Where("id = ?", provID2).Count(&acctCnt)
	if acctCnt != 0 {
		t.Fatalf("expected unreferenced provisional account deleted on undo, got count=%d", acctCnt)
	}

	// Ignore verdict followed by Settings promotion of that ignored email must trigger ConflictError on Undo.
	ignoreVerdict, err := ApplyTriageVerdict(gdb, ApplyTriageVerdictInput{EmailID: "prov-ghost", Action: "ignore"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PromoteIgnoredEmail(gdb, context.Background(), "prov-ghost"); err != nil {
		t.Fatal(err)
	}
	_, ignoreUndoErr := UndoTriageVerdict(gdb, TriageUndoInput{UndoPayload: ignoreVerdict.UndoPayload})
	var ce *ConflictError
	if !errors.As(ignoreUndoErr, &ce) {
		t.Fatalf("expected ConflictError on ignore Undo after Settings promote, got %T: %v", ignoreUndoErr, ignoreUndoErr)
	}
}

func TestSettingsPolicyRecoveryAndDeletionIndependence(t *testing.T) {
	gdb := openDomainTestDB(t)
	grab := seedSenderWithCursor(t, gdb, "s-grab", "no-reply@grab.com", 1795500000000, "auto")
	jenius := seedSenderWithCursor(t, gdb, "s-jenius", "jenius_noreply@smbci.com", 1795500000000, "auto")

	// Seed 1 post-LLM voucher (ignored via ghost verdict) and 1 pre-LLM policy:ignore voucher.
	seedSkippedEmail(
		t, gdb, "vouch-post", &grab.ID, "no-reply@grab.com", "Ride Cover Plus: Here's your Rp11.000 voucher.",
		"Ride Cover\nRp 11.000 voucher", 1795490000000, "2026-11-24T03:00:00Z", "qwen",
		`{"email_kind":"non_transaction","amount_decimal":"11000","currency":"IDR","merchant":"Grab Ride Cover","category":"Other"}`,
	)
	if _, err := ApplyTriageVerdict(gdb, ApplyTriageVerdictInput{EmailID: "vouch-post", Action: "ignore"}); err != nil {
		t.Fatal(err)
	}

	sigPre := DeriveEmailTemplateSignature("no-reply@grab.com", "Ride Cover Plus: Here's your Rp25.000 voucher.", "Ride Cover\nRp 25.000 voucher")
	if err := gdb.Create(&db.RawEmail{
		ID: "vouch-pre", GmailMessageID: "gm-vouch-pre", SenderID: &grab.ID,
		FromAddress: "no-reply@grab.com", Subject: "Ride Cover Plus: Here's your Rp25.000 voucher.",
		CleanedText: "Ride Cover\nRp 25.000 voucher", InternalDateMs: 1795500000000, ReceivedAtUtc: "2026-11-24T05:00:00Z", Status: "non_transaction",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := gdb.Exec(`
		INSERT INTO email_extractions (id, email_id, email_kind, extraction_model, raw_llm_json, triage_signature_key, triage_subject_pattern, triage_body_cue, triage_state)
		VALUES ('ext-pre', 'vouch-pre', 'non_transaction', 'policy:ignore', '{}', ?, ?, ?, 'ignored')`,
		sigPre.SignatureKey, sigPre.SubjectPattern, sigPre.BodyCue,
	).Error; err != nil {
		t.Fatal(err)
	}

	policies, err := ListTriagePolicies(gdb, ListTriagePoliciesParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(policies) != 1 || policies[0].IgnoredCount != 2 {
		t.Fatalf("expected 1 Grab policy with 2 ignored emails, got %+v", policies)
	}

	var llmCalls atomic.Int32
	var forceNullAmount atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llmCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if forceNullAmount.Load() {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{"message": map[string]any{"content": `{"email_kind":"non_transaction","amount_decimal":null,"currency":null,"transaction_date":null,"transaction_time":null,"timezone_offset":null,"merchant":null,"category":null,"card_last4":null,"payment_method":null,"bank_account_hint":null,"matched_account_id":null,"account_type_guess":null,"reference_code":null}`}}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": `{"email_kind":"non_transaction","amount_decimal":"25000","currency":"IDR","transaction_date":"2026-11-24","transaction_time":"12:00:00","timezone_offset":"+07:00","merchant":"Grab Voucher","category":"Other","card_last4":null,"payment_method":"GrabPay","bank_account_hint":null,"matched_account_id":null,"account_type_guess":null,"reference_code":null}`}}},
		})
	}))
	defer srv.Close()
	_ = gdb.Model(&db.AppSetting{}).Where("id = 1").Updates(map[string]any{
		"llm_base_url": srv.URL + "/v1",
		"llm_model":    "qwen2.5",
	}).Error

	// Promote pre-LLM row even while Always ignore policy stays intact; LLM returns non_transaction + 25000 -> force-admitted!
	promRes, err := PromoteIgnoredEmail(gdb, context.Background(), "vouch-pre")
	if err != nil {
		t.Fatalf("PromoteIgnoredEmail pre-LLM: %v", err)
	}
	if !promRes.UsedLlm || promRes.Transaction.AmountMinor != 25000 || promRes.Transaction.Reviewed != 0 {
		t.Fatalf("unexpected pre-LLM promote result: %+v", promRes)
	}
	if llmCalls.Load() != 1 {
		t.Fatalf("expected 1 LLM call for pre-LLM promote, got %d", llmCalls.Load())
	}

	// Re-promoting same already-promoted email returns 409 ConflictError.
	_, dupErr := PromoteIgnoredEmail(gdb, context.Background(), "vouch-pre")
	var ce *ConflictError
	if !errors.As(dupErr, &ce) {
		t.Fatalf("expected ConflictError on duplicate promote, got %T: %v", dupErr, dupErr)
	}

	// Delete ignore rule: post-LLM ignored row returns to Ghost Rows while pre-LLM row stays in Settings if newly seeded.
	delRes, err := DeleteTriagePolicy(gdb, policies[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if delRes.RestoredGhostCount != 1 {
		t.Fatalf("expected 1 restored ghost row from deleting ignore rule, got %d", delRes.RestoredGhostCount)
	}

	// Pre-LLM row where LLM yields no positive amount returns 422 UnprocessableError.
	if err := gdb.Create(&db.RawEmail{
		ID: "pre-zero", GmailMessageID: "gm-pre-zero", SenderID: &grab.ID,
		FromAddress: "no-reply@grab.com", Subject: "Ride Cover",
		CleanedText: "Promo note", InternalDateMs: 1795501000000, ReceivedAtUtc: "2026-11-24T05:10:00Z", Status: "non_transaction",
	}).Error; err != nil {
		t.Fatal(err)
	}
	_ = gdb.Exec(`INSERT INTO email_extractions (id, email_id, email_kind, extraction_model, raw_llm_json, triage_signature_key, triage_subject_pattern, triage_body_cue, triage_state) VALUES ('ext-pre-zero', 'pre-zero', 'non_transaction', 'policy:ignore', '{}', ?, ?, '', 'ignored')`, sigPre.SignatureKey, sigPre.SubjectPattern).Error

	forceNullAmount.Store(true)
	_, zeroErr := PromoteIgnoredEmail(gdb, context.Background(), "pre-zero")
	var ue *UnprocessableError
	if !errors.As(zeroErr, &ue) {
		t.Fatalf("expected UnprocessableError when LLM finds no positive amount, got %T: %v", zeroErr, zeroErr)
	}

	ignoredPage, err := ListIgnoredEmails(gdb, ListIgnoredEmailsParams{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(ignoredPage.AvailableSenders, "no-reply@grab.com") {
		t.Fatalf("expected availableSenders to contain no-reply@grab.com, got %v", ignoredPage.AvailableSenders)
	}

	// DeleteTransaction marks email dismissed, creates zero ignore policies, and excludes from ghost/ignored queries.
	if err := gdb.Create(&db.RawEmail{
		ID: "1e4bfdde-38a8-4ae3-aa41-492c5bfb7706", GmailMessageID: "gm-jenius-cancel", SenderID: &jenius.ID,
		FromAddress: "jenius_noreply@smbci.com", Subject: "Credit Card Transaction Notification",
		CleanedText: "Rp 85000", InternalDateMs: 1795502000000, ReceivedAtUtc: "2026-11-24T05:20:00Z", Status: "extracted",
	}).Error; err != nil {
		t.Fatal(err)
	}
	txDelID, err := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
		EmailID: "1e4bfdde-38a8-4ae3-aa41-492c5bfb7706", EmailKind: "card_charge", AmountMinor: 85000, Currency: "IDR",
		OccurredAtUtc: "2026-11-24T05:20:00Z", MerchantRaw: "Cancelled Order", Category: "Shopping",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := DeleteTransaction(gdb, txDelID); err != nil {
		t.Fatal(err)
	}
	var polAfterDel int64
	gdb.Model(&db.SenderTriagePolicy{}).Count(&polAfterDel)
	if polAfterDel != 0 {
		t.Fatalf("expected zero triage policies after DeleteTransaction, got %d", polAfterDel)
	}
}

func TestPromoteHistoricalGhostRow_PreservesReconciledBalanceAnchor(t *testing.T) {
	gdb := openDomainTestDB(t)
	acctID := seedTestAccount(t, gdb, "acct-rec", "Main Card", "IDR")
	if err := gdb.Model(&db.Account{}).Where("id = ?", acctID).Updates(map[string]any{
		"initial_balance_minor":     int64(1000000),
		"initial_balance_as_of_utc": "2026-11-01T00:00:00Z",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := gdb.Create(&db.AccountReconciliation{
		ID:                   "rec-1",
		AccountID:            acctID,
		Currency:             "IDR",
		ComputedBalanceMinor: 900000,
		ActualBalanceMinor:   900000,
		AdjustmentMinor:      0,
		RecordedAtUtc:        "2026-11-24T00:00:00Z",
	}).Error; err != nil {
		t.Fatal(err)
	}

	sender := seedSenderWithCursor(t, gdb, "s-bca", "bca@bca.co.id", 1795500000000, "auto")
	if err := gdb.Model(&db.Sender{}).Where("id = ?", sender.ID).Update("default_account_id", acctID).Error; err != nil {
		t.Fatal(err)
	}
	seedSkippedEmail(
		t, gdb, "hist-ghost", &sender.ID, "bca@bca.co.id", "Internet Transaction Journal",
		"Transaction Type\n:\nQRIS Payment", 1795200000000, "2026-11-20T05:00:00Z", "qwen",
		`{"email_kind":"non_transaction","amount_decimal":"45000","currency":"IDR","transaction_date":"2026-11-20","transaction_time":"12:00:00","timezone_offset":"+07:00","merchant":"Historical Cafe","category":"Dining Out"}`,
	)

	if _, err := ApplyTriageVerdict(gdb, ApplyTriageVerdictInput{EmailID: "hist-ghost", Action: "promote"}); err != nil {
		t.Fatal(err)
	}

	summary, err := GetMonthlySpendingSummary(gdb, "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if summary.TotalSpentMinor != 45000 {
		t.Fatalf("expected monthly spending to increase by 45000, got %d", summary.TotalSpentMinor)
	}
	accts, err := ListAccounts(gdb)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range accts {
		if a.ID == acctID {
			if a.ComputedBalanceMinor == nil || *a.ComputedBalanceMinor != 900000 {
				t.Fatalf("expected reconciled balance invariant intact at 900000, got %v", a.ComputedBalanceMinor)
			}
		}
	}
}

func TestListSkippedTransactions_IndependentPagination(t *testing.T) {
	gdb := openDomainTestDB(t)
	configureTestLocale(t, gdb, "Asia/Jakarta", "IDR")
	const received = "2026-10-12T12:00:00Z"
	for _, id := range []string{"a", "b", "c", "d", "ignored", "invalid", "statement", "dismissed", "zero"} {
		seedSkippedEmail(t, gdb, id, nil, "merchant@example.com", "Payment receipt", "Paid 100", 10, received, "fixture", "{}")
		if err := gdb.Exec("UPDATE email_extractions SET triage_state = 'ghost', currency = 'IDR', amount_minor = 100 WHERE email_id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, sql := range []string{
		"UPDATE email_extractions SET triage_state = 'ignored' WHERE email_id = 'ignored'",
		"UPDATE email_extractions SET currency = 'XX' WHERE email_id = 'invalid'",
		"UPDATE email_extractions SET extraction_model = 'rule:billing_statement' WHERE email_id = 'statement'",
		"UPDATE raw_emails SET status = 'dismissed' WHERE id = 'dismissed'",
		"UPDATE email_extractions SET amount_minor = 0 WHERE email_id = 'zero'",
	} {
		if err := gdb.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	seedSkippedEmail(t, gdb, "e", nil, "merchant@example.com", "Payment receipt", "Paid 100", 10, received, "fixture",
		`{"email_kind":"non_transaction","amount_decimal":"100","currency":"IDR"}`)

	wantPages := [][]string{{"e", "d"}, {"c", "b"}, {"a"}, {}}
	for i, wantIDs := range wantPages {
		page := i + 1
		result, err := ListSkippedTransactions(gdb, page, 2)
		if err != nil {
			t.Fatal(err)
		}
		if result.TotalCount != 5 || result.Page != page || result.PageSize != 2 || result.Items == nil {
			t.Fatalf("unexpected pagination: %+v", result)
		}
		var ids []string
		for _, row := range result.Items {
			if row.OccurredAtUtc != received {
				t.Errorf("expected received-time fallback, got %q", row.OccurredAtUtc)
			}
			ids = append(ids, row.EmailID)
		}
		if !slices.Equal(ids, wantIDs) {
			t.Errorf("page %d skipped IDs = %v, want %v", page, ids, wantIDs)
		}
	}
	largePage, err := ListSkippedTransactions(gdb, 1<<30, 1000)
	if err != nil || largePage.TotalCount != 5 || largePage.PageSize != 100 || len(largePage.Items) != 0 {
		t.Fatalf("oversized request should return an empty page with a bounded size: %+v, %v", largePage, err)
	}

	hide := false
	ledger, err := ListTransactions(gdb, ListTransactionsParams{ShowSkipped: &hide})
	if err != nil || ledger.TotalCount != 0 || ledger.TotalGhostCount != 5 || len(ledger.GhostRows) != 0 {
		t.Fatalf("ledger should show neither ghost rows nor ghost spending: %+v, %v", ledger, err)
	}
}
