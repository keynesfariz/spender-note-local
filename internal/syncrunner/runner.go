package syncrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"portspendernote/internal/db"
	"portspendernote/internal/domain"
	"portspendernote/internal/gmail"
)

type SyncStatusView struct {
	LatestRun         *db.SyncRun `json:"latestRun"`
	IsRunning         bool        `json:"isRunning"`
	PendingEmailCount int64       `json:"pendingEmailCount"`
	FailedEmailCount  int64       `json:"failedEmailCount"`
}

type activeHandle struct {
	runID  string
	cancel context.CancelFunc
}

var (
	activeMu sync.Mutex
	active   *activeHandle
)

func getActive() *activeHandle {
	activeMu.Lock()
	defer activeMu.Unlock()
	return active
}

func setActive(h *activeHandle) {
	activeMu.Lock()
	defer activeMu.Unlock()
	active = h
}

func assertNotSyncing(gdb *gorm.DB) error {
	if getActive() != nil {
		return errors.New("A background sync is already running.")
	}
	var cnt int64
	gdb.Model(&db.SyncRun{}).Where("status = 'running'").Count(&cnt)
	if cnt > 0 {
		return errors.New("A background sync is already running.")
	}
	return nil
}

func GetSyncStatus(gdb *gorm.DB) (*SyncStatusView, error) {
	var latest db.SyncRun
	var latestPtr *db.SyncRun
	if err := gdb.Order("started_at_utc DESC, rowid DESC").First(&latest).Error; err == nil {
		latestPtr = &latest
	}

	var pendingCnt, failedCnt int64
	gdb.Model(&db.RawEmail{}).Where("status = 'pending'").Count(&pendingCnt)
	gdb.Model(&db.RawEmail{}).Where("status = 'failed'").Count(&failedCnt)

	isRunning := getActive() != nil || (latestPtr != nil && latestPtr.Status == "running")
	return &SyncStatusView{
		LatestRun:         latestPtr,
		IsRunning:         isRunning,
		PendingEmailCount: pendingCnt,
		FailedEmailCount:  failedCnt,
	}, nil
}

type validatedConfig struct {
	CountryCode    *string
	Timezone       string
	Currency       string
	LlmProvider    string
	LlmModelPath   string
	LlmContextSize int
	LlmTimeoutMs   int
	SyncBatchLimit int
}

func loadValidatedSettings(gdb *gorm.DB, requireCountryAndGmail bool) (*validatedConfig, error) {
	var s db.AppSetting
	if err := gdb.First(&s, 1).Error; err != nil {
		return nil, errors.New("Application settings are missing.")
	}

	if requireCountryAndGmail {
		if s.CountryCode == nil || *s.CountryCode == "" || s.Timezone == nil || *s.Timezone == "" || s.Currency == nil || *s.Currency == "" {
			return nil, errors.New("Configure your country, timezone, and currency in Settings before syncing.")
		}
		var g db.GmailOAuth
		_ = gdb.First(&g, 1).Error
		if g.Status != "connected" {
			if g.Status == "expired" {
				return nil, errors.New("Gmail authorization expired. Reconnect Gmail in Settings.")
			}
			return nil, errors.New("Connect your Gmail account in Settings before syncing.")
		}
		var enabledCnt int64
		gdb.Model(&db.Sender{}).Where("enabled = 1").Count(&enabledCnt)
		if enabledCnt == 0 {
			return nil, errors.New("Add at least one enabled sender in Settings before syncing.")
		}
	} else if s.Timezone == nil || *s.Timezone == "" || s.Currency == nil || *s.Currency == "" {
		return nil, errors.New("Configure your country, timezone, and currency in Settings first.")
	}

	if s.LlmModelPath == nil || *s.LlmModelPath == "" {
		return nil, errors.New("Select a local GGUF model in Settings before syncing.")
	}
	if _, err := domain.ResolveSafeGgufPath(domain.GetModelsDir(), *s.LlmModelPath); err != nil {
		return nil, err
	}

	return &validatedConfig{
		CountryCode:    s.CountryCode,
		Timezone:       *s.Timezone,
		Currency:       *s.Currency,
		LlmProvider:    s.LlmProvider,
		LlmModelPath:   *s.LlmModelPath,
		LlmContextSize: s.LlmContextSize,
		LlmTimeoutMs:   s.LlmTimeoutMs,
		SyncBatchLimit: s.SyncBatchLimit,
	}, nil
}

func TriggerSync(gdb *gorm.DB, llmBaseURL string) (string, error) {
	if err := assertNotSyncing(gdb); err != nil {
		return "", err
	}
	cfg, err := loadValidatedSettings(gdb, true)
	if err != nil {
		return "", err
	}

	runID := uuid.NewString()
	nowUtc := domain.NowUtcIsoSeconds()
	if err := gdb.Create(&db.SyncRun{
		ID:           runID,
		Status:       "running",
		Phase:        "preflight",
		LlmProvider:  cfg.LlmProvider,
		LlmModel:     cfg.LlmModelPath,
		StartedAtUtc: nowUtc,
	}).Error; err != nil {
		return "", err
	}

	ctx, cancel := context.WithCancel(context.Background())
	setActive(&activeHandle{runID: runID, cancel: cancel})

	go func() {
		defer func() {
			activeMu.Lock()
			if active != nil && active.runID == runID {
				active = nil
			}
			activeMu.Unlock()
		}()
		runFullSyncJob(ctx, gdb, runID, cfg, llmBaseURL)
	}()

	return runID, nil
}

func RetryPendingAndFailedExtractions(gdb *gorm.DB, llmBaseURL string) (string, error) {
	if err := assertNotSyncing(gdb); err != nil {
		return "", err
	}
	cfg, err := loadValidatedSettings(gdb, false)
	if err != nil {
		return "", err
	}

	runID := uuid.NewString()
	nowUtc := domain.NowUtcIsoSeconds()
	err = gdb.Transaction(func(tx *gorm.DB) error {
		tx.Model(&db.RawEmail{}).Where("status = 'failed'").Updates(map[string]any{
			"status":        "pending",
			"error_message": nil,
		})
		return tx.Create(&db.SyncRun{
			ID:           runID,
			Status:       "running",
			Phase:        "extracting",
			LlmProvider:  cfg.LlmProvider,
			LlmModel:     cfg.LlmModelPath,
			StartedAtUtc: nowUtc,
		}).Error
	})
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithCancel(context.Background())
	setActive(&activeHandle{runID: runID, cancel: cancel})

	go func() {
		defer func() {
			activeMu.Lock()
			if active != nil && active.runID == runID {
				active = nil
			}
			activeMu.Unlock()
		}()
		runExtractionPhase(ctx, gdb, runID, cfg, llmBaseURL)
	}()

	return runID, nil
}

func CancelSync(gdb *gorm.DB) bool {
	h := getActive()
	if h != nil {
		h.cancel()
	}
	nowUtc := domain.NowUtcIsoSeconds()
	res := gdb.Exec(`
		UPDATE sync_runs
		SET status = 'cancelled', phase = 'done', finished_at_utc = COALESCE(finished_at_utc, ?)
		WHERE status = 'running'`, nowUtc)
	return h != nil || res.RowsAffected > 0
}

func DismissPendingQueue(gdb *gorm.DB) (int64, error) {
	if err := assertNotSyncing(gdb); err != nil {
		return 0, err
	}
	res := gdb.Model(&db.RawEmail{}).Where("status IN ?", []string{"pending", "failed"}).Update("status", "dismissed")
	return res.RowsAffected, res.Error
}

func runFullSyncJob(ctx context.Context, gdb *gorm.DB, runID string, cfg *validatedConfig, llmBaseURL string) {
	gdb.Model(&db.SyncRun{}).Where("id = ?", runID).Update("phase", "fetching")

	var senders []db.Sender
	gdb.Where("enabled = 1").Order("email ASC").Find(&senders)

	var knownRows []string
	gdb.Model(&db.RawEmail{}).Pluck("gmail_message_id", &knownRows)
	knownIDs := map[string]struct{}{}
	for _, id := range knownRows {
		knownIDs[id] = struct{}{}
	}

	const cursorOverlapMs int64 = 60_000
	remainingBudget := max(1, cfg.SyncBatchLimit)
	hasMoreEmails := false

	failRun := func(err error) {
		if ctx.Err() != nil {
			finalizeCancelled(gdb, runID)
			return
		}
		gdb.Exec(`
			UPDATE sync_runs
			SET status = 'failed', phase = 'done', error_message = ?, finished_at_utc = ?
			WHERE id = ? AND status = 'running'`, err.Error(), domain.NowUtcIsoSeconds(), runID)
	}

	for _, sender := range senders {
		if ctx.Err() != nil {
			break
		}
		if remainingBudget <= 0 {
			hasMoreEmails = true
			break
		}

		var overlapStartMs int64
		if sender.LastInternalDateMs != nil {
			overlapStartMs = max(0, *sender.LastInternalDateMs-cursorOverlapMs)
		} else {
			ms, _ := domain.UtcIsoToEpochMs(sender.BackfillFromUtc)
			overlapStartMs = max(0, ms)
		}
		afterEpochSec := overlapStartMs / 1000

		var persistMu sync.Mutex
		persistedIDs := map[string]struct{}{}
		var maxInternalDateMs *int64
		senderFetchedCount := 0

		persistMessage := func(msg gmail.FetchedMessage) {
			persistMu.Lock()
			defer persistMu.Unlock()
			if _, ok := persistedIDs[msg.GmailMessageID]; ok {
				return
			}
			persistedIDs[msg.GmailMessageID] = struct{}{}
			knownIDs[msg.GmailMessageID] = struct{}{}
			if maxInternalDateMs == nil || msg.InternalDateMs > *maxInternalDateMs {
				v := msg.InternalDateMs
				maxInternalDateMs = &v
			}
			_ = gdb.Transaction(func(tx *gorm.DB) error {
				res := tx.Exec(`
					INSERT OR IGNORE INTO raw_emails (
						id, gmail_message_id, sender_id, from_address, subject, cleaned_text,
						internal_date_ms, received_at_utc, status
					) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending')`,
					uuid.NewString(), msg.GmailMessageID, sender.ID, msg.FromAddress,
					msg.Subject, msg.CleanedText, msg.InternalDateMs, msg.ReceivedAtUtc,
				)
				if res.RowsAffected > 0 {
					senderFetchedCount++
					tx.Exec(`UPDATE sync_runs SET fetched_count = fetched_count + 1, cleaned_count = cleaned_count + 1 WHERE id = ?`, runID)
				} else {
					tx.Exec(`UPDATE sync_runs SET skipped_count = skipped_count + 1 WHERE id = ?`, runID)
				}
				return nil
			})
		}

		accessToken, err := gmail.GetValidAccessToken(ctx, gdb, false)
		if err != nil {
			failRun(err)
			return
		}

		result, err := gmail.FetchSenderMessages(ctx, gmail.FetchSenderMessagesParams{
			AccessToken:     accessToken,
			SenderEmail:     sender.Email,
			AfterEpochSec:   afterEpochSec,
			KnownMessageIDs: knownIDs,
			MaxMessages:     remainingBudget,
			OnMessage:       persistMessage,
		})
		if err != nil {
			var unauth gmail.UnauthorizedError
			if errors.As(err, &unauth) {
				accessToken, err = gmail.GetValidAccessToken(ctx, gdb, true)
				if err != nil {
					failRun(err)
					return
				}
				result, err = gmail.FetchSenderMessages(ctx, gmail.FetchSenderMessagesParams{
					AccessToken:     accessToken,
					SenderEmail:     sender.Email,
					AfterEpochSec:   afterEpochSec,
					KnownMessageIDs: knownIDs,
					MaxMessages:     remainingBudget,
					OnMessage:       persistMessage,
				})
				if err != nil {
					if errors.As(err, &unauth) {
						gdb.Model(&db.GmailOAuth{}).Where("id = 1").Update("status", "expired")
						failRun(&gmail.AuthExpiredError{Message: "Gmail authorization expired or was revoked. Reconnect Gmail in Settings."})
						return
					}
					failRun(err)
					return
				}
			} else {
				failRun(err)
				return
			}
		}

		for _, msg := range result.Messages {
			persistMessage(msg)
		}

		remainingBudget = max(0, remainingBudget-senderFetchedCount)
		if result.HasMore {
			hasMoreEmails = true
		}
		if ctx.Err() != nil {
			break
		}

		updateSenderSyncState(gdb, sender, maxInternalDateMs, result.HasMore)
	}

	if hasMoreEmails {
		gdb.Model(&db.SyncRun{}).Where("id = ?", runID).Update("has_more_emails", 1)
	}
	if ctx.Err() != nil {
		finalizeCancelled(gdb, runID)
		return
	}

	runExtractionPhase(ctx, gdb, runID, cfg, llmBaseURL)
}

func updateSenderSyncState(gdb *gorm.DB, sender db.Sender, maxInternalDateMs *int64, hasMore bool) {
	updates := map[string]any{
		"last_sync_at_utc": domain.NowUtcIsoSeconds(),
	}
	if !hasMore {
		nextCursor := sender.LastInternalDateMs
		if maxInternalDateMs != nil {
			v := *maxInternalDateMs
			if nextCursor == nil || v > *nextCursor {
				nextCursor = &v
			}
		}
		var maxRow struct {
			MaxMs *int64 `gorm:"column:max_ms"`
		}
		gdb.Raw(`SELECT MAX(internal_date_ms) AS max_ms FROM raw_emails WHERE sender_id = ?`, sender.ID).Scan(&maxRow)
		if maxRow.MaxMs != nil {
			v := *maxRow.MaxMs
			if nextCursor == nil || v > *nextCursor {
				nextCursor = &v
			}
		}
		updates["last_internal_date_ms"] = nextCursor
	}
	gdb.Model(&db.Sender{}).Where("id = ?", sender.ID).Updates(updates)
}

type llmExtractRequest struct {
	ModelPath     string                            `json:"modelPath"`
	ContextSize   int                               `json:"contextSize"`
	TimeoutMs     int                               `json:"timeoutMs"`
	EmailID       string                            `json:"emailId"`
	FromAddress   string                            `json:"fromAddress"`
	Subject       string                            `json:"subject"`
	CleanedText   string                            `json:"cleanedText"`
	ReceivedAtUtc string                            `json:"receivedAtUtc"`
	UserCountry   *string                           `json:"userCountry"`
	UserTimezone  string                            `json:"userTimezone"`
	UserCurrency  string                            `json:"userCurrency"`
	Accounts      []domain.ExtractionAccountContext `json:"accounts"`
}

type llmExtractResponse struct {
	EmailKind        string  `json:"emailKind"`
	AmountMinor      int64   `json:"amountMinor"`
	Currency         string  `json:"currency"`
	OccurredAtUtc    string  `json:"occurredAtUtc"`
	MerchantRaw      string  `json:"merchantRaw"`
	Category         string  `json:"category"`
	CardLast4        *string `json:"cardLast4"`
	PaymentMethodRaw *string `json:"paymentMethodRaw"`
	BankAccountHint  *string `json:"bankAccountHint"`
	MatchedAccountID *string `json:"matchedAccountId"`
	AccountTypeGuess *string `json:"accountTypeGuess"`
	ReferenceCode    *string `json:"referenceCode"`
	RawJson          string  `json:"rawJson"`
	Error            string  `json:"error,omitempty"`
	ErrorKind        string  `json:"errorKind,omitempty"`
}

func callNodeLlamaExtract(ctx context.Context, baseURL string, payload llmExtractRequest) (*llmExtractResponse, string, error) {
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, "runtime", err
	}
	timeout := time.Duration(payload.TimeoutMs+15_000) * time.Millisecond
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, baseURL+"/api/llm/extract", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, "runtime", err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "runtime", fmt.Errorf("LLM service request failed: %w", err)
	}
	defer res.Body.Close()

	respBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, "runtime", err
	}

	var parsed llmExtractResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, "runtime", fmt.Errorf("Invalid LLM response (HTTP %d)", res.StatusCode)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 || parsed.Error != "" {
		kind := parsed.ErrorKind
		if kind == "" {
			kind = "runtime"
		}
		msg := parsed.Error
		if msg == "" {
			msg = fmt.Sprintf("LLM extraction failed (HTTP %d)", res.StatusCode)
		}
		return nil, kind, errors.New(msg)
	}
	return &parsed, "", nil
}

func runExtractionPhase(ctx context.Context, gdb *gorm.DB, runID string, cfg *validatedConfig, llmBaseURL string) {
	gdb.Model(&db.SyncRun{}).Where("id = ?", runID).Update("phase", "extracting")

	var pendingIDs []string
	gdb.Model(&db.RawEmail{}).Where("status = 'pending'").Order("internal_date_ms ASC, rowid ASC").Pluck("id", &pendingIDs)
	if len(pendingIDs) == 0 {
		finalizeCompleted(gdb, runID)
		return
	}

	accountContexts := domain.ListExtractionAccountContexts(gdb)
	var runtimeErr error

	for _, pendingID := range pendingIDs {
		if ctx.Err() != nil {
			break
		}

		type pendingRow struct {
			ID            string  `gorm:"column:id"`
			SenderID      *string `gorm:"column:sender_id"`
			FromAddress   string  `gorm:"column:from_address"`
			Subject       string  `gorm:"column:subject"`
			CleanedText   string  `gorm:"column:cleaned_text"`
			ReceivedAtUtc string  `gorm:"column:received_at_utc"`
			RoleHint      *string `gorm:"column:role_hint"`
		}
		var rows []pendingRow
		gdb.Raw(`
			SELECT re.id, re.sender_id, re.from_address, re.subject, re.cleaned_text, re.received_at_utc, s.role_hint
			FROM raw_emails re
			LEFT JOIN senders s ON s.id = re.sender_id
			WHERE re.id = ?`, pendingID).Scan(&rows)
		if len(rows) == 0 {
			continue
		}
		email := rows[0]

		if IsBillingStatementEmail(email.Subject, email.CleanedText) {
			txErr := gdb.Transaction(func(tx *gorm.DB) error {
				if err := tx.Exec(`
					INSERT OR REPLACE INTO email_extractions (
						id, email_id, email_kind, extraction_model, raw_llm_json
					) VALUES (?, ?, 'non_transaction', 'rule:billing_statement', ?)`,
					uuid.NewString(), email.ID, `{"email_kind":"non_transaction","reason":"billing_statement_filter"}`,
				).Error; err != nil {
					return err
				}
				if err := tx.Model(&db.RawEmail{}).Where("id = ?", email.ID).Updates(map[string]any{
					"status":        "non_transaction",
					"error_message": nil,
				}).Error; err != nil {
					return err
				}
				return tx.Exec(`UPDATE sync_runs SET skipped_count = skipped_count + 1 WHERE id = ?`, runID).Error
			})
			if txErr != nil {
				gdb.Model(&db.RawEmail{}).Where("id = ?", email.ID).Updates(map[string]any{
					"status":        "failed",
					"error_message": txErr.Error(),
				})
				gdb.Exec(`UPDATE sync_runs SET failed_count = failed_count + 1 WHERE id = ?`, runID)
			}
			continue
		}

		extracted, errKind, err := callNodeLlamaExtract(ctx, llmBaseURL, llmExtractRequest{
			ModelPath:     cfg.LlmModelPath,
			ContextSize:   cfg.LlmContextSize,
			TimeoutMs:     cfg.LlmTimeoutMs,
			EmailID:       email.ID,
			FromAddress:   email.FromAddress,
			Subject:       email.Subject,
			CleanedText:   email.CleanedText,
			ReceivedAtUtc: email.ReceivedAtUtc,
			UserCountry:   cfg.CountryCode,
			UserTimezone:  cfg.Timezone,
			UserCurrency:  cfg.Currency,
			Accounts:      accountContexts,
		})
		if ctx.Err() != nil {
			break
		}
		if err != nil {
			if errKind == "runtime" {
				runtimeErr = err
				break
			}
			gdb.Model(&db.RawEmail{}).Where("id = ?", email.ID).Updates(map[string]any{
				"status":        "failed",
				"error_message": err.Error(),
			})
			gdb.Exec(`UPDATE sync_runs SET failed_count = failed_count + 1 WHERE id = ?`, runID)
			continue
		}

		txErr := gdb.Transaction(func(tx *gorm.DB) error {
			if extracted.EmailKind == "non_transaction" {
				tx.Exec(`
					INSERT OR REPLACE INTO email_extractions (
						id, email_id, email_kind, extraction_model, raw_llm_json
					) VALUES (?, ?, 'non_transaction', ?, ?)`,
					uuid.NewString(), email.ID, cfg.LlmModelPath, extracted.RawJson,
				)
				tx.Model(&db.RawEmail{}).Where("id = ?", email.ID).Updates(map[string]any{
					"status":        "non_transaction",
					"error_message": nil,
				})
				tx.Exec(`UPDATE sync_runs SET skipped_count = skipped_count + 1 WHERE id = ?`, runID)
				return nil
			}

			effectiveRole := extracted.EmailKind
			if email.RoleHint != nil && (*email.RoleHint == "card_charge" || *email.RoleHint == "receipt") {
				effectiveRole = *email.RoleHint
			}

			tx.Exec(`
				INSERT OR REPLACE INTO email_extractions (
					id, email_id, email_kind, amount_minor, currency, occurred_at_utc,
					merchant_raw, merchant_norm, category, card_last4,
					payment_method_raw, bank_account_hint, llm_account_id, reference_code,
					extraction_model, raw_llm_json
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				uuid.NewString(), email.ID, effectiveRole, extracted.AmountMinor, extracted.Currency,
				extracted.OccurredAtUtc, extracted.MerchantRaw, domain.NormalizeMerchant(extracted.MerchantRaw),
				extracted.Category, extracted.CardLast4, extracted.PaymentMethodRaw, extracted.BankAccountHint,
				extracted.MatchedAccountID, extracted.ReferenceCode, cfg.LlmModelPath, extracted.RawJson,
			)

			accountID := domain.MatchAccountForExtraction(tx, domain.MatchAccountParams{
				SenderID:        email.SenderID,
				CardLast4:       extracted.CardLast4,
				BankAccountHint: extracted.BankAccountHint,
				PaymentMethod:   extracted.PaymentMethodRaw,
				LlmAccountID:    extracted.MatchedAccountID,
			})

			provisionalCreated := 0
			if accountID == nil {
				cur := extracted.Currency
				prov, provErr := domain.CreateProvisionalAccountForExtraction(tx, domain.ProvisionalAccountParams{
					FromAddress:      email.FromAddress,
					CardLast4:        extracted.CardLast4,
					BankAccountHint:  extracted.BankAccountHint,
					PaymentMethod:    extracted.PaymentMethodRaw,
					AccountTypeGuess: extracted.AccountTypeGuess,
					Currency:         &cur,
				})
				if provErr == nil && prov != nil {
					id := prov.ID
					accountID = &id
					provisionalCreated = 1
					idents := make([]string, 0, len(prov.Identifiers))
					for _, ident := range prov.Identifiers {
						idents = append(idents, ident.Value)
					}
					accountContexts = append(accountContexts, domain.ExtractionAccountContext{
						ID:          prov.ID,
						Name:        prov.Name,
						Type:        prov.Type,
						Identifiers: idents,
					})
				}
			}

			if _, err := domain.SaveExtractedTransaction(tx, domain.ExtractedTransactionInput{
				EmailID:           email.ID,
				EmailKind:         effectiveRole,
				AmountMinor:       extracted.AmountMinor,
				Currency:          extracted.Currency,
				OccurredAtUtc:     extracted.OccurredAtUtc,
				MerchantRaw:       extracted.MerchantRaw,
				Category:          extracted.Category,
				CardLast4:         extracted.CardLast4,
				ReferenceCode:     extracted.ReferenceCode,
				ResolvedAccountID: accountID,
			}); err != nil {
				return err
			}

			tx.Model(&db.RawEmail{}).Where("id = ?", email.ID).Updates(map[string]any{
				"status":        "extracted",
				"error_message": nil,
			})

			tx.Exec(`
				UPDATE sync_runs
				SET imported_count = imported_count + 1,
				    provisional_accounts_created = provisional_accounts_created + ?
				WHERE id = ?`, provisionalCreated, runID)
			return nil
		})
		if txErr != nil {
			gdb.Model(&db.RawEmail{}).Where("id = ?", email.ID).Updates(map[string]any{
				"status":        "failed",
				"error_message": txErr.Error(),
			})
			gdb.Exec(`UPDATE sync_runs SET failed_count = failed_count + 1 WHERE id = ?`, runID)
		}
	}

	if ctx.Err() != nil {
		finalizeCancelled(gdb, runID)
		return
	}
	if runtimeErr != nil {
		gdb.Exec(`
			UPDATE sync_runs
			SET status = CASE WHEN imported_count > 0 OR merged_count > 0 THEN 'partial_error' ELSE 'failed' END,
			    phase = 'done',
			    error_message = ?,
			    finished_at_utc = ?
			WHERE id = ? AND status = 'running'`, runtimeErr.Error(), domain.NowUtcIsoSeconds(), runID)
		return
	}
	finalizeCompleted(gdb, runID)
}

func finalizeCancelled(gdb *gorm.DB, runID string) {
	gdb.Exec(`
		UPDATE sync_runs
		SET status = 'cancelled', phase = 'done', finished_at_utc = COALESCE(finished_at_utc, ?)
		WHERE id = ? AND status = 'running'`, domain.NowUtcIsoSeconds(), runID)
}

func finalizeCompleted(gdb *gorm.DB, runID string) {
	gdb.Exec(`
		UPDATE sync_runs
		SET status = CASE WHEN failed_count > 0 THEN 'partial_error' ELSE 'completed' END,
		    phase = 'done',
		    finished_at_utc = ?
		WHERE id = ? AND status = 'running'`, domain.NowUtcIsoSeconds(), runID)
}
