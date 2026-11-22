package syncrunner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"portspendernote/internal/db"
	"portspendernote/internal/domain"
	"portspendernote/internal/gmail"
	"portspendernote/internal/llm"
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
	done   chan struct{}
}

var (
	activeMu sync.Mutex
	active   *activeHandle

	fetchSenderMessagesFn = gmail.FetchSenderMessages
	getValidAccessTokenFn = gmail.GetValidAccessToken
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
	LlmBaseURL     string
	LlmModel       string
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
		if domain.FindCountryByCode(*s.CountryCode) == nil {
			code := strings.ToUpper(strings.TrimSpace(*s.CountryCode))
			return nil, fmt.Errorf("Saved country %q is not available in countries/. Select an available country in Settings or add countries/%s.json before syncing.", code, code)
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

	baseURL, err := llm.NormalizeBaseURL(s.LlmBaseURL)
	if err != nil {
		return nil, err
	}
	if s.LlmModel == nil || strings.TrimSpace(*s.LlmModel) == "" {
		return nil, errors.New("Select a model from your local LLM server in Settings before syncing.")
	}

	return &validatedConfig{
		CountryCode:    s.CountryCode,
		Timezone:       *s.Timezone,
		Currency:       *s.Currency,
		LlmProvider:    s.LlmProvider,
		LlmBaseURL:     baseURL,
		LlmModel:       strings.TrimSpace(*s.LlmModel),
		LlmTimeoutMs:   s.LlmTimeoutMs,
		SyncBatchLimit: s.SyncBatchLimit,
	}, nil
}

func TriggerSync(gdb *gorm.DB) (string, error) {
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
		LlmModel:     cfg.LlmModel,
		StartedAtUtc: nowUtc,
	}).Error; err != nil {
		return "", err
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	setActive(&activeHandle{runID: runID, cancel: cancel, done: done})

	go func() {
		defer func() {
			activeMu.Lock()
			if active != nil && active.runID == runID {
				active = nil
			}
			activeMu.Unlock()
			close(done)
		}()
		runFullSyncJob(ctx, gdb, runID, cfg)
	}()

	return runID, nil
}

func RetryPendingAndFailedExtractions(gdb *gorm.DB) (string, error) {
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
		if err := tx.Model(&db.RawEmail{}).Where("status = 'failed'").Updates(map[string]any{
			"status":        "pending",
			"error_message": nil,
		}).Error; err != nil {
			return err
		}
		return tx.Create(&db.SyncRun{
			ID:           runID,
			Status:       "running",
			Phase:        "preflight",
			LlmProvider:  cfg.LlmProvider,
			LlmModel:     cfg.LlmModel,
			StartedAtUtc: nowUtc,
		}).Error
	})
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	setActive(&activeHandle{runID: runID, cancel: cancel, done: done})

	go func() {
		defer func() {
			activeMu.Lock()
			if active != nil && active.runID == runID {
				active = nil
			}
			activeMu.Unlock()
			close(done)
		}()
		runRetryJob(ctx, gdb, runID, cfg)
	}()

	return runID, nil
}

func CancelSync(gdb *gorm.DB) bool {
	h := getActive()
	if h != nil {
		h.cancel()
		select {
		case <-h.done:
		case <-time.After(10 * time.Second):
		}
		return true
	}
	nowUtc := domain.NowUtcIsoSeconds()
	res := gdb.Exec(`
		UPDATE sync_runs
		SET status = 'cancelled', phase = 'done', finished_at_utc = COALESCE(finished_at_utc, ?)
		WHERE status = 'running'`, nowUtc)
	return res.RowsAffected > 0
}

func DismissPendingQueue(gdb *gorm.DB) (int64, error) {
	if err := assertNotSyncing(gdb); err != nil {
		return 0, err
	}
	res := gdb.Model(&db.RawEmail{}).Where("status IN ?", []string{"pending", "failed"}).Update("status", "dismissed")
	return res.RowsAffected, res.Error
}

func toLlmAccountContexts(src []domain.ExtractionAccountContext) []llm.AccountContext {
	out := make([]llm.AccountContext, 0, len(src))
	for _, a := range src {
		out = append(out, llm.AccountContext{
			ID:          a.ID,
			Name:        a.Name,
			Type:        a.Type,
			Identifiers: append([]string(nil), a.Identifiers...),
		})
	}
	return out
}

func runRetryJob(runCtx context.Context, gdb *gorm.DB, runID string, cfg *validatedConfig) {
	llmClient, err := llm.NewClient(cfg.LlmBaseURL, time.Duration(cfg.LlmTimeoutMs)*time.Millisecond)
	if err != nil {
		finalizeWithError(gdb, runID, runCtx, err)
		return
	}
	if err := llmClient.Preflight(runCtx, cfg.LlmModel); err != nil {
		finalizeWithError(gdb, runID, runCtx, err)
		return
	}
	if runCtx.Err() != nil {
		finalizeCancelled(gdb, runID)
		return
	}

	gdb.Exec(`UPDATE sync_runs SET phase = 'extracting' WHERE id = ? AND status = 'running'`, runID)

	closedDone := make(chan struct{})
	close(closedDone)
	runtimeErr := runExtractionWorker(runCtx, gdb, runID, cfg, llmClient, nil, closedDone, nil)
	if runCtx.Err() != nil {
		finalizeCancelled(gdb, runID)
		return
	}
	if runtimeErr != nil {
		finalizeWithError(gdb, runID, runCtx, runtimeErr)
		return
	}
	finalizeCompleted(gdb, runID)
}

func runFullSyncJob(runCtx context.Context, gdb *gorm.DB, runID string, cfg *validatedConfig) {
	llmClient, err := llm.NewClient(cfg.LlmBaseURL, time.Duration(cfg.LlmTimeoutMs)*time.Millisecond)
	if err != nil {
		finalizeWithError(gdb, runID, runCtx, err)
		return
	}
	if err := llmClient.Preflight(runCtx, cfg.LlmModel); err != nil {
		finalizeWithError(gdb, runID, runCtx, err)
		return
	}
	if runCtx.Err() != nil {
		finalizeCancelled(gdb, runID)
		return
	}

	gdb.Exec(`UPDATE sync_runs SET phase = 'syncing' WHERE id = ? AND status = 'running'`, runID)

	fetchCtx, cancelFetch := context.WithCancel(runCtx)
	defer cancelFetch()

	notifyCh := make(chan struct{}, 1)
	fetchDoneCh := make(chan struct{})

	var (
		wg                sync.WaitGroup
		fetchErr          error
		extractRuntimeErr error
	)

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(fetchDoneCh)
		fetchErr = runFetchWorker(fetchCtx, gdb, runID, cfg, notifyCh)
		if fetchCtx.Err() == nil && fetchErr == nil {
			gdb.Exec(`UPDATE sync_runs SET phase = 'extracting' WHERE id = ? AND status = 'running' AND phase = 'syncing'`, runID)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		extractRuntimeErr = runExtractionWorker(runCtx, gdb, runID, cfg, llmClient, notifyCh, fetchDoneCh, cancelFetch)
	}()

	wg.Wait()

	if runCtx.Err() != nil {
		finalizeCancelled(gdb, runID)
		return
	}
	if extractRuntimeErr != nil {
		finalizeWithError(gdb, runID, runCtx, extractRuntimeErr)
		return
	}
	if fetchErr != nil {
		finalizeWithError(gdb, runID, runCtx, fetchErr)
		return
	}
	finalizeCompleted(gdb, runID)
}

func runFetchWorker(ctx context.Context, gdb *gorm.DB, runID string, cfg *validatedConfig, notifyCh chan<- struct{}) error {
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
			if ctx.Err() != nil {
				return
			}
			if _, ok := persistedIDs[msg.GmailMessageID]; ok {
				return
			}
			persistedIDs[msg.GmailMessageID] = struct{}{}
			knownIDs[msg.GmailMessageID] = struct{}{}
			if maxInternalDateMs == nil || msg.InternalDateMs > *maxInternalDateMs {
				v := msg.InternalDateMs
				maxInternalDateMs = &v
			}
			insertedNew := false
			_ = gdb.Transaction(func(tx *gorm.DB) error {
				res := tx.Exec(`
					INSERT OR IGNORE INTO raw_emails (
						id, gmail_message_id, sender_id, from_address, subject, cleaned_text,
						internal_date_ms, received_at_utc, status
					) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending')`,
					uuid.NewString(), msg.GmailMessageID, sender.ID, msg.FromAddress,
					msg.Subject, msg.CleanedText, msg.InternalDateMs, msg.ReceivedAtUtc,
				)
				if res.Error != nil {
					return res.Error
				}
				if res.RowsAffected > 0 {
					insertedNew = true
					senderFetchedCount++
					if err := tx.Exec(`UPDATE sync_runs SET fetched_count = fetched_count + 1, cleaned_count = cleaned_count + 1 WHERE id = ?`, runID).Error; err != nil {
						return err
					}
				} else {
					if err := tx.Exec(`UPDATE sync_runs SET skipped_count = skipped_count + 1 WHERE id = ?`, runID).Error; err != nil {
						return err
					}
				}
				return nil
			})
			if insertedNew && notifyCh != nil {
				select {
				case notifyCh <- struct{}{}:
				default:
				}
			}
		}

		accessToken, err := getValidAccessTokenFn(ctx, gdb, false)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}

		result, err := fetchSenderMessagesFn(ctx, gmail.FetchSenderMessagesParams{
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
				accessToken, err = getValidAccessTokenFn(ctx, gdb, true)
				if err != nil {
					if ctx.Err() != nil {
						return nil
					}
					return err
				}
				result, err = fetchSenderMessagesFn(ctx, gmail.FetchSenderMessagesParams{
					AccessToken:     accessToken,
					SenderEmail:     sender.Email,
					AfterEpochSec:   afterEpochSec,
					KnownMessageIDs: knownIDs,
					MaxMessages:     remainingBudget,
					OnMessage:       persistMessage,
				})
				if err != nil {
					if ctx.Err() != nil {
						return nil
					}
					if errors.As(err, &unauth) {
						gdb.Model(&db.GmailOAuth{}).Where("id = 1").Update("status", "expired")
						return &gmail.AuthExpiredError{Message: "Gmail authorization expired or was revoked. Reconnect Gmail in Settings."}
					}
					return err
				}
			} else {
				if ctx.Err() != nil {
					return nil
				}
				return err
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

	if hasMoreEmails && ctx.Err() == nil {
		gdb.Model(&db.SyncRun{}).Where("id = ? AND status = 'running'", runID).Update("has_more_emails", 1)
	}
	return nil
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

type pendingEmailRow struct {
	ID            string  `gorm:"column:id"`
	SenderID      *string `gorm:"column:sender_id"`
	FromAddress   string  `gorm:"column:from_address"`
	Subject       string  `gorm:"column:subject"`
	CleanedText   string  `gorm:"column:cleaned_text"`
	ReceivedAtUtc string  `gorm:"column:received_at_utc"`
	RoleHint      *string `gorm:"column:role_hint"`
}

func nextPendingEmail(gdb *gorm.DB) (*pendingEmailRow, bool) {
	var rows []pendingEmailRow
	gdb.Raw(`
		SELECT re.id, re.sender_id, re.from_address, re.subject, re.cleaned_text, re.received_at_utc, s.role_hint
		FROM raw_emails re
		LEFT JOIN senders s ON s.id = re.sender_id
		WHERE re.status = 'pending'
		ORDER BY re.internal_date_ms ASC, re.rowid ASC
		LIMIT 1`).Scan(&rows)
	if len(rows) == 0 {
		return nil, false
	}
	return &rows[0], true
}

func runExtractionWorker(
	runCtx context.Context,
	gdb *gorm.DB,
	runID string,
	cfg *validatedConfig,
	llmClient *llm.Client,
	notifyCh <-chan struct{},
	fetchDoneCh <-chan struct{},
	cancelFetch context.CancelFunc,
) error {
	accountContexts := toLlmAccountContexts(domain.ListExtractionAccountContexts(gdb))

	var countryPromptRules []string
	if cfg.CountryCode != nil && strings.TrimSpace(*cfg.CountryCode) != "" {
		if pack := domain.FindCountryPackByCode(*cfg.CountryCode); pack != nil && len(pack.LlmPromptRules) > 0 {
			countryPromptRules = pack.LlmPromptRules
		}
	}

	fetchDone := false
	for {
		if runCtx.Err() != nil {
			return nil
		}

		email, found := nextPendingEmail(gdb)
		if !found {
			if fetchDone {
				return nil
			}
			select {
			case <-runCtx.Done():
				return nil
			case <-notifyCh:
			case <-fetchDoneCh:
				fetchDone = true
				fetchDoneCh = nil
			}
			continue
		}

		if IsBillingStatementEmail(email.Subject, email.CleanedText, cfg.CountryCode) {
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
				markEmailFailed(gdb, runID, email.ID, txErr.Error())
			}
			continue
		}

		userCurrency := cfg.Currency
		extracted, err := llmClient.ExtractTransaction(runCtx, cfg.LlmModel, llm.ExtractInput{
			EmailID:            email.ID,
			FromAddress:        email.FromAddress,
			Subject:            email.Subject,
			CleanedText:        email.CleanedText,
			ReceivedAtUtc:      email.ReceivedAtUtc,
			UserCountry:        cfg.CountryCode,
			CountryPromptRules: countryPromptRules,
			UserTimezone:       cfg.Timezone,
			UserCurrency:       &userCurrency,
			Accounts:           accountContexts,
		})
		if runCtx.Err() != nil {
			return nil
		}
		if err != nil {
			if llm.IsRuntimeError(err) {
				if cancelFetch != nil {
					cancelFetch()
				}
				return err
			}
			markEmailFailed(gdb, runID, email.ID, err.Error())
			continue
		}

		var createdProvisional *domain.AccountSummary
		txErr := gdb.Transaction(func(tx *gorm.DB) error {
			if extracted.EmailKind == "non_transaction" {
				if err := tx.Exec(`
					INSERT OR REPLACE INTO email_extractions (
						id, email_id, email_kind, extraction_model, raw_llm_json
					) VALUES (?, ?, 'non_transaction', ?, ?)`,
					uuid.NewString(), email.ID, cfg.LlmModel, extracted.RawJSON,
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
			}

			effectiveRole := extracted.EmailKind
			if email.RoleHint != nil && (*email.RoleHint == "card_charge" || *email.RoleHint == "receipt") {
				effectiveRole = *email.RoleHint
			}

			if err := tx.Exec(`
				INSERT OR REPLACE INTO email_extractions (
					id, email_id, email_kind, amount_minor, currency, occurred_at_utc,
					merchant_raw, merchant_norm, category, card_last4,
					payment_method_raw, bank_account_hint, llm_account_id, reference_code,
					extraction_model, raw_llm_json
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				uuid.NewString(), email.ID, effectiveRole, extracted.AmountMinor, extracted.Currency,
				extracted.OccurredAtUtc, extracted.MerchantRaw, domain.NormalizeMerchant(extracted.MerchantRaw),
				extracted.Category, extracted.CardLast4, extracted.PaymentMethodRaw, extracted.BankAccountHint,
				extracted.MatchedAccountID, extracted.ReferenceCode, cfg.LlmModel, extracted.RawJSON,
			).Error; err != nil {
				return err
			}

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
					createdProvisional = prov
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

			if err := tx.Model(&db.RawEmail{}).Where("id = ?", email.ID).Updates(map[string]any{
				"status":        "extracted",
				"error_message": nil,
			}).Error; err != nil {
				return err
			}

			return tx.Exec(`
				UPDATE sync_runs
				SET imported_count = imported_count + 1,
				    provisional_accounts_created = provisional_accounts_created + ?
				WHERE id = ?`, provisionalCreated, runID).Error
		})
		if txErr != nil {
			markEmailFailed(gdb, runID, email.ID, txErr.Error())
			continue
		}
		if createdProvisional != nil {
			idents := make([]string, 0, len(createdProvisional.Identifiers))
			for _, ident := range createdProvisional.Identifiers {
				idents = append(idents, ident.Value)
			}
			accountContexts = append(accountContexts, llm.AccountContext{
				ID:          createdProvisional.ID,
				Name:        createdProvisional.Name,
				Type:        createdProvisional.Type,
				Identifiers: idents,
			})
		}
	}
}

func markEmailFailed(gdb *gorm.DB, runID, emailID, errMsg string) {
	gdb.Model(&db.RawEmail{}).Where("id = ?", emailID).Updates(map[string]any{
		"status":        "failed",
		"error_message": errMsg,
	})
	gdb.Exec(`UPDATE sync_runs SET failed_count = failed_count + 1 WHERE id = ?`, runID)
}

func finalizeWithError(gdb *gorm.DB, runID string, ctx context.Context, err error) {
	if ctx.Err() != nil {
		finalizeCancelled(gdb, runID)
		return
	}
	gdb.Exec(`
		UPDATE sync_runs
		SET status = CASE WHEN imported_count > 0 OR merged_count > 0 THEN 'partial_error' ELSE 'failed' END,
		    phase = 'done',
		    error_message = ?,
		    finished_at_utc = ?
		WHERE id = ? AND status = 'running'`, err.Error(), domain.NowUtcIsoSeconds(), runID)
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
