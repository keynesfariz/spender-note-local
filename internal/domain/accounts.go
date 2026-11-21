package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"portspendernote/internal/db"
)

var (
	fourDigitsRe  = regexp.MustCompile(`^\d{4}$`)
	nonDigitsRe   = regexp.MustCompile(`\D`)
	multiSpaceRe  = regexp.MustCompile(`\s+`)
	displayNameRe = regexp.MustCompile(`^"?([^"<]+?)"?\s*<[^>]+>$`)
	angleEmailRe  = regexp.MustCompile(`<([^>]+)>`)
)

type AccountIdentifierInput struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type AccountSummary struct {
	db.Account
	Identifiers           []db.AccountIdentifier    `json:"identifiers"`
	ComputedBalanceMinor  *int64                    `json:"computed_balance_minor"`
	NetFlowMinor          int64                     `json:"net_flow_minor"`
	UncountedForeignCount int64                     `json:"uncounted_foreign_count"`
	LatestReconciliation  *db.AccountReconciliation `json:"latest_reconciliation"`
	TypeLocked            bool                      `json:"type_locked"`
}

type ExtractionAccountContext struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Identifiers []string `json:"identifiers"`
}

func isValidAccountType(t string) bool {
	return t == "credit_card" || t == "debit_card" || t == "digital_wallet"
}

func isValidIdentifierKind(k string) bool {
	return k == "card_last4" || k == "bank_account" || k == "alias"
}

func AssertAccountExists(gdb *gorm.DB, accountID string, message string) error {
	if message == "" {
		message = "Selected account does not exist."
	}
	var cnt int64
	gdb.Model(&db.Account{}).Where("id = ?", accountID).Count(&cnt)
	if cnt == 0 {
		return errors.New(message)
	}
	return nil
}

func normalizeAliasText(raw string) string {
	return multiSpaceRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(raw)), " ")
}

func normalizeIdentifierValue(kind, rawValue string) (string, string, error) {
	value := strings.TrimSpace(rawValue)
	if value == "" {
		return "", "", errors.New("Identifier value is required.")
	}
	switch kind {
	case "card_last4":
		if !fourDigitsRe.MatchString(value) {
			return "", "", errors.New("Card last 4 digits must be exactly 4 numeric digits.")
		}
		return value, value, nil
	case "bank_account":
		digits := nonDigitsRe.ReplaceAllString(value, "")
		if len(digits) < 4 {
			return "", "", errors.New("Bank account number must contain at least 4 digits.")
		}
		return value, digits, nil
	case "alias":
		return value, normalizeAliasText(value), nil
	default:
		return "", "", fmt.Errorf("Invalid identifier kind: %s", kind)
	}
}

func validateLast4(last4 *string) (*string, error) {
	if last4 == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*last4)
	if trimmed == "" {
		return nil, nil
	}
	_, norm, err := normalizeIdentifierValue("card_last4", trimmed)
	if err != nil {
		return nil, err
	}
	return &norm, nil
}

func listIdentifiersForAccount(gdb *gorm.DB, accountID string) []db.AccountIdentifier {
	rows := []db.AccountIdentifier{}
	gdb.Where("account_id = ?", accountID).Order("created_at_utc ASC, rowid ASC").Find(&rows)
	return rows
}

func replaceAccountIdentifiers(tx *gorm.DB, accountID string, identifiers []AccountIdentifierInput, createdAtUtc string) (*string, error) {
	seen := map[string]struct{}{}
	type normRow struct {
		kind, value, normalized string
	}
	var normalizedRows []normRow
	for _, item := range identifiers {
		if !isValidIdentifierKind(item.Kind) {
			return nil, fmt.Errorf("Invalid identifier kind: %s", item.Kind)
		}
		val, norm, err := normalizeIdentifierValue(item.Kind, item.Value)
		if err != nil {
			return nil, err
		}
		key := item.Kind + ":" + norm
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalizedRows = append(normalizedRows, normRow{item.Kind, val, norm})
	}

	if err := tx.Where("account_id = ?", accountID).Delete(&db.AccountIdentifier{}).Error; err != nil {
		return nil, err
	}

	var firstLast4 *string
	for _, row := range normalizedRows {
		ident := db.AccountIdentifier{
			ID:              uuid.NewString(),
			AccountID:       accountID,
			Kind:            row.kind,
			Value:           row.value,
			NormalizedValue: row.normalized,
			CreatedAtUtc:    createdAtUtc,
		}
		if err := tx.Create(&ident).Error; err != nil {
			return nil, err
		}
		if row.kind == "card_last4" && firstLast4 == nil {
			v := row.normalized
			firstLast4 = &v
		}
	}
	return firstLast4, nil
}

func getConfiguredAppCurrency(gdb *gorm.DB) (string, error) {
	var row db.AppSetting
	if err := gdb.First(&row, 1).Error; err != nil || row.Currency == nil || *row.Currency == "" {
		return "", errors.New("Configure your country and currency in Settings before managing accounts.")
	}
	return *row.Currency, nil
}

func hasAccountLedgerRows(gdb *gorm.DB, accountID string) bool {
	var txCnt int64
	gdb.Model(&db.Transaction{}).Where("account_id = ?", accountID).Count(&txCnt)
	if txCnt > 0 {
		return true
	}
	var recCnt int64
	gdb.Model(&db.AccountReconciliation{}).Where("account_id = ?", accountID).Count(&recCnt)
	return recCnt > 0
}

type accountBalanceInfo struct {
	ComputedBalanceMinor  *int64
	NetFlowMinor          int64
	UncountedForeignCount int64
	LatestReconciliation  *db.AccountReconciliation
	Currency              string
}

func computeAccountBalance(gdb *gorm.DB, account db.Account, targetCurrency string) accountBalanceInfo {
	settingsCurrency := targetCurrency
	if settingsCurrency == "" {
		var s db.AppSetting
		if err := gdb.First(&s, 1).Error; err == nil && s.Currency != nil && *s.Currency != "" {
			settingsCurrency = *s.Currency
		} else {
			settingsCurrency = account.Currency
		}
	}

	var latestRec db.AccountReconciliation
	var latestRecPtr *db.AccountReconciliation
	if err := gdb.Where("account_id = ?", account.ID).Order("recorded_at_utc DESC, rowid DESC").First(&latestRec).Error; err == nil {
		latestRecPtr = &latestRec
	}

	var anchorBalance *int64
	var anchorTime string
	timeOp := ">="
	if latestRecPtr != nil {
		b := latestRecPtr.ActualBalanceMinor
		anchorBalance = &b
		anchorTime = latestRecPtr.RecordedAtUtc
		timeOp = ">"
	} else {
		anchorBalance = account.InitialBalanceMinor
		anchorTime = account.InitialBalanceAsOfUtc
	}

	type aggResult struct {
		LocalTotal int64 `gorm:"column:local_total"`
		ForeignCnt int64 `gorm:"column:foreign_cnt"`
	}
	var agg aggResult
	if anchorBalance == nil {
		gdb.Raw(`
			SELECT
				COALESCE(SUM(CASE WHEN currency = ? THEN amount_minor ELSE 0 END), 0) AS local_total,
				COALESCE(SUM(CASE WHEN currency != ? THEN 1 ELSE 0 END), 0) AS foreign_cnt
			FROM transactions
			WHERE account_id = ?`,
			settingsCurrency, settingsCurrency, account.ID,
		).Scan(&agg)
	} else {
		gdb.Raw(fmt.Sprintf(`
			SELECT
				COALESCE(SUM(CASE WHEN currency = ? THEN amount_minor ELSE 0 END), 0) AS local_total,
				COALESCE(SUM(CASE WHEN currency != ? THEN 1 ELSE 0 END), 0) AS foreign_cnt
			FROM transactions
			WHERE account_id = ? AND occurred_at_utc %s ?`, timeOp),
			settingsCurrency, settingsCurrency, account.ID, anchorTime,
		).Scan(&agg)
	}

	var netFlow int64
	if agg.LocalTotal != 0 {
		if account.Type == "credit_card" {
			netFlow = agg.LocalTotal
		} else {
			netFlow = -agg.LocalTotal
		}
	}

	var computed *int64
	if anchorBalance != nil {
		v := *anchorBalance + netFlow
		computed = &v
	}

	return accountBalanceInfo{
		ComputedBalanceMinor:  computed,
		NetFlowMinor:          netFlow,
		UncountedForeignCount: agg.ForeignCnt,
		LatestReconciliation:  latestRecPtr,
		Currency:              settingsCurrency,
	}
}

func isAccountTypeLocked(gdb *gorm.DB, account db.Account, hasRec bool) bool {
	if hasRec {
		return true
	}
	if account.Status == "provisional" {
		return false
	}
	return hasAccountLedgerRows(gdb, account.ID)
}

func buildAccountSummary(gdb *gorm.DB, account db.Account, targetCurrency string, identifiers []db.AccountIdentifier) AccountSummary {
	bal := computeAccountBalance(gdb, account, targetCurrency)
	if identifiers == nil {
		identifiers = listIdentifiersForAccount(gdb, account.ID)
	}
	return AccountSummary{
		Account:               account,
		Identifiers:           identifiers,
		ComputedBalanceMinor:  bal.ComputedBalanceMinor,
		NetFlowMinor:          bal.NetFlowMinor,
		UncountedForeignCount: bal.UncountedForeignCount,
		LatestReconciliation:  bal.LatestReconciliation,
		TypeLocked:            isAccountTypeLocked(gdb, account, bal.LatestReconciliation != nil),
	}
}

func ListAccounts(gdb *gorm.DB) ([]AccountSummary, error) {
	var s db.AppSetting
	targetCurrency := ""
	if err := gdb.First(&s, 1).Error; err == nil && s.Currency != nil {
		targetCurrency = *s.Currency
	}

	var rows []db.Account
	if err := gdb.Order("created_at_utc ASC, name ASC").Find(&rows).Error; err != nil {
		return nil, err
	}

	var allIdents []db.AccountIdentifier
	gdb.Order("created_at_utc ASC, rowid ASC").Find(&allIdents)
	byAcct := map[string][]db.AccountIdentifier{}
	for _, ident := range allIdents {
		byAcct[ident.AccountID] = append(byAcct[ident.AccountID], ident)
	}

	out := make([]AccountSummary, 0, len(rows))
	for _, row := range rows {
		idents := byAcct[row.ID]
		if idents == nil {
			idents = []db.AccountIdentifier{}
		}
		out = append(out, buildAccountSummary(gdb, row, targetCurrency, idents))
	}
	return out, nil
}

type CreateAccountInput struct {
	Name                  string                   `json:"name"`
	Type                  string                   `json:"type"`
	InitialBalanceMinor   int64                    `json:"initial_balance_minor"`
	InitialBalanceAsOfUtc *string                  `json:"initial_balance_as_of_utc"`
	Last4                 *string                  `json:"last4"`
	Identifiers           []AccountIdentifierInput `json:"identifiers"`
	HasIdentifiers        bool                     `json:"-"`
}

func CreateAccount(gdb *gorm.DB, input CreateAccountInput) (*AccountSummary, error) {
	currency, err := getConfiguredAppCurrency(gdb)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errors.New("Account name is required.")
	}
	if !isValidAccountType(input.Type) {
		return nil, fmt.Errorf("Invalid account type: %s", input.Type)
	}

	asOfUtc := NowUtcIsoSeconds()
	if input.InitialBalanceAsOfUtc != nil && strings.TrimSpace(*input.InitialBalanceAsOfUtc) != "" {
		asOfUtc = strings.TrimSpace(*input.InitialBalanceAsOfUtc)
	}
	if !IsValidUtcIsoSeconds(asOfUtc) {
		return nil, errors.New("Initial balance timestamp must be a valid UTC ISO-8601 string.")
	}

	fallbackLast4, err := validateLast4(input.Last4)
	if err != nil {
		return nil, err
	}

	id := uuid.NewString()
	createdAt := NowUtcIsoSeconds()
	initBal := input.InitialBalanceMinor

	var summary AccountSummary
	err = gdb.Transaction(func(tx *gorm.DB) error {
		acct := db.Account{
			ID:                    id,
			Name:                  name,
			Type:                  input.Type,
			Currency:              currency,
			InitialBalanceMinor:   &initBal,
			InitialBalanceAsOfUtc: asOfUtc,
			Last4:                 fallbackLast4,
			Status:                "confirmed",
			CreatedAtUtc:          createdAt,
		}
		if err := tx.Create(&acct).Error; err != nil {
			return err
		}

		if input.HasIdentifiers {
			syncedLast4, err := replaceAccountIdentifiers(tx, id, input.Identifiers, createdAt)
			if err != nil {
				return err
			}
			if err := tx.Model(&db.Account{}).Where("id = ?", id).Update("last4", syncedLast4).Error; err != nil {
				return err
			}
			acct.Last4 = syncedLast4
		} else if fallbackLast4 != nil {
			if _, err := replaceAccountIdentifiers(tx, id, []AccountIdentifierInput{{Kind: "card_last4", Value: *fallbackLast4}}, createdAt); err != nil {
				return err
			}
		}

		summary = buildAccountSummary(tx, acct, currency, nil)
		summary.TypeLocked = false
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

type UpdateAccountInput struct {
	Name                  *string                  `json:"name"`
	Type                  *string                  `json:"type"`
	HasInitialBalance     bool                     `json:"-"`
	InitialBalanceMinor   *int64                   `json:"initial_balance_minor"`
	InitialBalanceAsOfUtc *string                  `json:"initial_balance_as_of_utc"`
	HasLast4              bool                     `json:"-"`
	Last4                 *string                  `json:"last4"`
	HasIdentifiers        bool                     `json:"-"`
	Identifiers           []AccountIdentifierInput `json:"identifiers"`
}

func UpdateAccount(gdb *gorm.DB, id string, patch UpdateAccountInput) (*AccountSummary, error) {
	var summary AccountSummary
	err := gdb.Transaction(func(tx *gorm.DB) error {
		var current db.Account
		if err := tx.Where("id = ?", id).First(&current).Error; err != nil {
			return errors.New("Account not found.")
		}

		name := current.Name
		if patch.Name != nil {
			name = strings.TrimSpace(*patch.Name)
		}
		if name == "" {
			return errors.New("Account name is required.")
		}

		nextType := current.Type
		if patch.Type != nil {
			nextType = *patch.Type
		}
		if !isValidAccountType(nextType) {
			return fmt.Errorf("Invalid account type: %s", nextType)
		}

		bal := computeAccountBalance(tx, current, "")
		locked := isAccountTypeLocked(tx, current, bal.LatestReconciliation != nil)
		if nextType != current.Type && locked {
			return errors.New("Account type cannot be changed after transactions or reconciliations exist.")
		}

		initBal := current.InitialBalanceMinor
		if patch.HasInitialBalance {
			initBal = patch.InitialBalanceMinor
		}

		asOfUtc := current.InitialBalanceAsOfUtc
		if patch.InitialBalanceAsOfUtc != nil {
			asOfUtc = strings.TrimSpace(*patch.InitialBalanceAsOfUtc)
		}
		if !IsValidUtcIsoSeconds(asOfUtc) {
			return errors.New("Initial balance timestamp must be a valid UTC ISO-8601 string.")
		}

		last4 := current.Last4
		if patch.HasLast4 {
			var err error
			last4, err = validateLast4(patch.Last4)
			if err != nil {
				return err
			}
		}

		if patch.HasIdentifiers {
			var err error
			last4, err = replaceAccountIdentifiers(tx, id, patch.Identifiers, NowUtcIsoSeconds())
			if err != nil {
				return err
			}
		} else if patch.HasLast4 {
			existing := listIdentifiersForAccount(tx, id)
			var nextList []AccountIdentifierInput
			if last4 != nil {
				nextList = append(nextList, AccountIdentifierInput{Kind: "card_last4", Value: *last4})
			}
			for _, r := range existing {
				if r.Kind != "card_last4" {
					nextList = append(nextList, AccountIdentifierInput{Kind: r.Kind, Value: r.Value})
				}
			}
			var err error
			last4, err = replaceAccountIdentifiers(tx, id, nextList, NowUtcIsoSeconds())
			if err != nil {
				return err
			}
		}

		if err := tx.Model(&db.Account{}).Where("id = ?", id).Updates(map[string]any{
			"name":                      name,
			"type":                      nextType,
			"initial_balance_minor":     initBal,
			"initial_balance_as_of_utc": asOfUtc,
			"last4":                     last4,
			"status":                    "confirmed",
		}).Error; err != nil {
			return err
		}

		var updated db.Account
		if err := tx.Where("id = ?", id).First(&updated).Error; err != nil {
			return err
		}
		summary = buildAccountSummary(tx, updated, "", nil)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

func ConfirmAccount(gdb *gorm.DB, id string) (*AccountSummary, error) {
	var summary AccountSummary
	err := gdb.Transaction(func(tx *gorm.DB) error {
		var current db.Account
		if err := tx.Where("id = ?", id).First(&current).Error; err != nil {
			return errors.New("Account not found.")
		}
		if current.Status != "confirmed" {
			if err := tx.Model(&db.Account{}).Where("id = ?", id).Update("status", "confirmed").Error; err != nil {
				return err
			}
			current.Status = "confirmed"
		}
		summary = buildAccountSummary(tx, current, "", nil)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

func DeleteAccount(gdb *gorm.DB, id string) error {
	res := gdb.Where("id = ?", id).Delete(&db.Account{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("Account not found.")
	}
	return nil
}

type ReconcileAccountInput struct {
	AccountID          string  `json:"account_id"`
	ActualBalanceMinor int64   `json:"actual_balance_minor"`
	Note               *string `json:"note"`
	RecordedAtUtc      *string `json:"recorded_at_utc"`
}

func ReconcileAccount(gdb *gorm.DB, input ReconcileAccountInput) (map[string]any, error) {
	recordedAt := NowUtcIsoSeconds()
	if input.RecordedAtUtc != nil && strings.TrimSpace(*input.RecordedAtUtc) != "" {
		recordedAt = strings.TrimSpace(*input.RecordedAtUtc)
	}
	if !IsValidUtcIsoSeconds(recordedAt) {
		return nil, errors.New("Reconciliation timestamp must be a valid UTC ISO-8601 string.")
	}

	var rec db.AccountReconciliation
	var summary AccountSummary
	err := gdb.Transaction(func(tx *gorm.DB) error {
		var current db.Account
		if err := tx.Where("id = ?", input.AccountID).First(&current).Error; err != nil {
			return errors.New("Account not found.")
		}

		balBefore := computeAccountBalance(tx, current, "")
		computedMinor := input.ActualBalanceMinor
		if balBefore.ComputedBalanceMinor != nil {
			computedMinor = *balBefore.ComputedBalanceMinor
		}
		adjMinor := input.ActualBalanceMinor - computedMinor

		var note *string
		if input.Note != nil && strings.TrimSpace(*input.Note) != "" {
			n := strings.TrimSpace(*input.Note)
			note = &n
		}

		rec = db.AccountReconciliation{
			ID:                   uuid.NewString(),
			AccountID:            input.AccountID,
			Currency:             balBefore.Currency,
			ComputedBalanceMinor: computedMinor,
			ActualBalanceMinor:   input.ActualBalanceMinor,
			AdjustmentMinor:      adjMinor,
			Note:                 note,
			RecordedAtUtc:        recordedAt,
		}
		if err := tx.Create(&rec).Error; err != nil {
			return err
		}

		if current.Status != "confirmed" {
			if err := tx.Model(&db.Account{}).Where("id = ?", input.AccountID).Update("status", "confirmed").Error; err != nil {
				return err
			}
			current.Status = "confirmed"
		}

		summary = buildAccountSummary(tx, current, "", nil)
		summary.TypeLocked = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"reconciliation": rec,
		"account":        summary,
	}, nil
}

func ListAccountReconciliations(gdb *gorm.DB, accountID string) ([]db.AccountReconciliation, error) {
	rows := []db.AccountReconciliation{}
	if err := gdb.Where("account_id = ?", accountID).Order("recorded_at_utc DESC, rowid DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func ListExtractionAccountContexts(gdb *gorm.DB) []ExtractionAccountContext {
	var accounts []db.Account
	gdb.Order("created_at_utc ASC, name ASC").Find(&accounts)
	if len(accounts) == 0 {
		return []ExtractionAccountContext{}
	}
	var idents []db.AccountIdentifier
	gdb.Order("created_at_utc ASC, rowid ASC").Find(&idents)
	byAcct := map[string][]string{}
	for _, ident := range idents {
		byAcct[ident.AccountID] = append(byAcct[ident.AccountID], ident.Value)
	}
	out := make([]ExtractionAccountContext, 0, len(accounts))
	for _, a := range accounts {
		vals := byAcct[a.ID]
		if vals == nil {
			vals = []string{}
		}
		out = append(out, ExtractionAccountContext{
			ID:          a.ID,
			Name:        a.Name,
			Type:        a.Type,
			Identifiers: vals,
		})
	}
	return out
}

type MatchAccountParams struct {
	SenderID        *string
	CardLast4       *string
	BankAccountHint *string
	PaymentMethod   *string
	LlmAccountID    *string
}

func MatchAccountForExtraction(gdb *gorm.DB, params MatchAccountParams) *string {
	// 1. Unique card_last4
	cardStatus := "none"
	if params.CardLast4 != nil && fourDigitsRe.MatchString(*params.CardLast4) {
		var ids []string
		gdb.Raw(`
			SELECT account_id AS id FROM account_identifiers WHERE kind = 'card_last4' AND normalized_value = ?
			UNION
			SELECT id FROM accounts WHERE last4 = ?`, *params.CardLast4, *params.CardLast4).Scan(&ids)
		if len(ids) == 1 {
			return &ids[0]
		}
		if len(ids) > 1 {
			cardStatus = "collision"
		}
	}

	// 2. Unique bank_account hint
	bankStatus := cardStatus
	if cardStatus == "none" && params.BankAccountHint != nil {
		digits := nonDigitsRe.ReplaceAllString(*params.BankAccountHint, "")
		if len(digits) >= 4 {
			var bankRows []db.AccountIdentifier
			gdb.Where("kind = ?", "bank_account").Find(&bankRows)
			matches := map[string]struct{}{}
			for _, row := range bankRows {
				if row.NormalizedValue == digits ||
					strings.HasSuffix(row.NormalizedValue, digits) ||
					strings.HasSuffix(digits, row.NormalizedValue) {
					matches[row.AccountID] = struct{}{}
				}
			}
			if len(matches) == 1 {
				for id := range matches {
					return &id
				}
			}
			if len(matches) > 1 {
				bankStatus = "collision"
			}
		}
	}

	// 3. Unique name or alias
	if bankStatus == "none" && params.PaymentMethod != nil && strings.TrimSpace(*params.PaymentMethod) != "" {
		norm := normalizeAliasText(*params.PaymentMethod)
		matches := map[string]struct{}{}
		var accts []db.Account
		gdb.Select("id, name").Find(&accts)
		for _, a := range accts {
			if normalizeAliasText(a.Name) == norm {
				matches[a.ID] = struct{}{}
			}
		}
		var aliasIds []string
		gdb.Model(&db.AccountIdentifier{}).
			Distinct("account_id").
			Where("kind = ? AND normalized_value = ?", "alias", norm).
			Pluck("account_id", &aliasIds)
		for _, id := range aliasIds {
			matches[id] = struct{}{}
		}
		if len(matches) == 1 {
			for id := range matches {
				return &id
			}
		}
	}

	// 4. LLM matched_account_id
	if params.LlmAccountID != nil && strings.TrimSpace(*params.LlmAccountID) != "" {
		trimmed := strings.TrimSpace(*params.LlmAccountID)
		var acct db.Account
		if err := gdb.Select("id").Where("id = ?", trimmed).First(&acct).Error; err == nil {
			return &acct.ID
		}
	}

	// 5. Sender default account
	if params.SenderID != nil && *params.SenderID != "" {
		var sender db.Sender
		if err := gdb.Select("default_account_id").Where("id = ?", *params.SenderID).First(&sender).Error; err == nil {
			if sender.DefaultAccountID != nil && *sender.DefaultAccountID != "" {
				return sender.DefaultAccountID
			}
		}
	}

	return nil
}

func deriveSenderLabel(fromAddress string) string {
	trimmed := strings.TrimSpace(fromAddress)
	if trimmed == "" {
		return "Account"
	}
	if m := displayNameRe.FindStringSubmatch(trimmed); m != nil && strings.TrimSpace(m[1]) != "" {
		return strings.TrimSpace(m[1])
	}
	addr := trimmed
	if m := angleEmailRe.FindStringSubmatch(trimmed); m != nil {
		addr = strings.TrimSpace(m[1])
	}
	if atIdx := strings.IndexByte(addr, '@'); atIdx != -1 {
		domainPart := strings.Split(addr[atIdx+1:], ".")[0]
		if domainPart != "" {
			if len(domainPart) <= 4 {
				return strings.ToUpper(domainPart)
			}
			return strings.ToUpper(domainPart[:1]) + domainPart[1:]
		}
	}
	if len(addr) > 24 {
		return addr[:24]
	}
	return addr
}

type ProvisionalAccountParams struct {
	FromAddress      string
	CardLast4        *string
	BankAccountHint  *string
	PaymentMethod    *string
	AccountTypeGuess *string
	Currency         *string
}

func CreateProvisionalAccountForExtraction(gdb *gorm.DB, params ProvisionalAccountParams) (*AccountSummary, error) {
	var identifiers []AccountIdentifierInput
	identifierLabel := ""

	validLast4, _ := validateLast4(params.CardLast4)
	if validLast4 != nil {
		identifiers = append(identifiers, AccountIdentifierInput{Kind: "card_last4", Value: *validLast4})
		identifierLabel = "••" + *validLast4
	}
	bankHint := ""
	if params.BankAccountHint != nil {
		bankHint = strings.TrimSpace(*params.BankAccountHint)
	}
	if bankHint != "" {
		identifiers = append(identifiers, AccountIdentifierInput{Kind: "bank_account", Value: bankHint})
		if identifierLabel == "" {
			identifierLabel = bankHint
		}
	}
	wallet := ""
	if params.PaymentMethod != nil {
		wallet = strings.TrimSpace(*params.PaymentMethod)
	}
	if wallet != "" {
		identifiers = append(identifiers, AccountIdentifierInput{Kind: "alias", Value: wallet})
		if identifierLabel == "" {
			identifierLabel = wallet
		}
	}
	if len(identifiers) == 0 {
		return nil, nil
	}

	resolvedType := "digital_wallet"
	if params.AccountTypeGuess != nil && isValidAccountType(*params.AccountTypeGuess) {
		resolvedType = *params.AccountTypeGuess
	} else if validLast4 != nil {
		resolvedType = "credit_card"
	} else if bankHint != "" {
		resolvedType = "debit_card"
	}

	name := deriveSenderLabel(params.FromAddress) + " " + identifierLabel
	currency := ""
	if params.Currency != nil && len(strings.TrimSpace(*params.Currency)) == 3 {
		currency = strings.ToUpper(strings.TrimSpace(*params.Currency))
	} else {
		var err error
		currency, err = getConfiguredAppCurrency(gdb)
		if err != nil {
			return nil, err
		}
	}

	id := uuid.NewString()
	nowUtc := NowUtcIsoSeconds()

	var summary AccountSummary
	err := gdb.Transaction(func(tx *gorm.DB) error {
		acct := db.Account{
			ID:                    id,
			Name:                  name,
			Type:                  resolvedType,
			Currency:              currency,
			InitialBalanceMinor:   nil,
			InitialBalanceAsOfUtc: nowUtc,
			Last4:                 validLast4,
			Status:                "provisional",
			CreatedAtUtc:          nowUtc,
		}
		if err := tx.Create(&acct).Error; err != nil {
			return err
		}
		syncedLast4, err := replaceAccountIdentifiers(tx, id, identifiers, nowUtc)
		if err != nil {
			return err
		}
		if syncedLast4 != validLast4 {
			_ = tx.Model(&db.Account{}).Where("id = ?", id).Update("last4", syncedLast4).Error
			acct.Last4 = syncedLast4
		}
		summary = AccountSummary{
			Account:               acct,
			Identifiers:           listIdentifiersForAccount(tx, id),
			ComputedBalanceMinor:  nil,
			NetFlowMinor:          0,
			UncountedForeignCount: 0,
			LatestReconciliation:  nil,
			TypeLocked:            false,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

func MergeAccounts(gdb *gorm.DB, sourceAccountID, targetAccountID string) (map[string]any, error) {
	if sourceAccountID == targetAccountID {
		return nil, errors.New("Cannot merge an account into itself.")
	}

	var result map[string]any
	err := gdb.Transaction(func(tx *gorm.DB) error {
		var source, target db.Account
		if err := tx.Where("id = ?", sourceAccountID).First(&source).Error; err != nil {
			return errors.New("Source account not found.")
		}
		if err := tx.Where("id = ?", targetAccountID).First(&target).Error; err != nil {
			return errors.New("Destination account not found.")
		}
		if source.Currency != target.Currency {
			return fmt.Errorf("Cannot merge accounts with different currencies (%s vs %s).", source.Currency, target.Currency)
		}
		if (source.Type == "credit_card") != (target.Type == "credit_card") {
			return errors.New("Credit card accounts can only be merged with other credit card accounts.")
		}

		txMove := tx.Model(&db.Transaction{}).Where("account_id = ?", sourceAccountID).Update("account_id", targetAccountID)
		if txMove.Error != nil {
			return txMove.Error
		}

		sourceIdents := listIdentifiersForAccount(tx, sourceAccountID)
		movedIdents := 0
		for _, ident := range sourceIdents {
			res := tx.Exec(`
				INSERT OR IGNORE INTO account_identifiers (
					id, account_id, kind, value, normalized_value, created_at_utc
				) VALUES (?, ?, ?, ?, ?, ?)`,
				uuid.NewString(), targetAccountID, ident.Kind, ident.Value, ident.NormalizedValue, ident.CreatedAtUtc,
			)
			if res.RowsAffected > 0 {
				movedIdents++
			}
		}

		tx.Model(&db.Sender{}).Where("default_account_id = ?", sourceAccountID).Update("default_account_id", targetAccountID)

		updatedTarget := target
		if target.InitialBalanceMinor == nil {
			var targetRecCnt int64
			tx.Model(&db.AccountReconciliation{}).Where("account_id = ?", targetAccountID).Count(&targetRecCnt)
			if targetRecCnt == 0 {
				var sourceRec db.AccountReconciliation
				if err := tx.Where("account_id = ?", sourceAccountID).Order("recorded_at_utc DESC, rowid DESC").First(&sourceRec).Error; err == nil {
					newRec := db.AccountReconciliation{
						ID:                   uuid.NewString(),
						AccountID:            targetAccountID,
						Currency:             sourceRec.Currency,
						ComputedBalanceMinor: sourceRec.ComputedBalanceMinor,
						ActualBalanceMinor:   sourceRec.ActualBalanceMinor,
						AdjustmentMinor:      sourceRec.AdjustmentMinor,
						Note:                 sourceRec.Note,
						RecordedAtUtc:        sourceRec.RecordedAtUtc,
					}
					_ = tx.Create(&newRec).Error
				} else if source.InitialBalanceMinor != nil {
					_ = tx.Model(&db.Account{}).Where("id = ?", targetAccountID).Updates(map[string]any{
						"initial_balance_minor":     source.InitialBalanceMinor,
						"initial_balance_as_of_utc": source.InitialBalanceAsOfUtc,
					}).Error
					updatedTarget.InitialBalanceMinor = source.InitialBalanceMinor
					updatedTarget.InitialBalanceAsOfUtc = source.InitialBalanceAsOfUtc
				}
			}
		}

		if err := tx.Where("id = ?", sourceAccountID).Delete(&db.Account{}).Error; err != nil {
			return err
		}

		suggestionIDs := RunPostMergeDedupForAccount(tx, targetAccountID, NowUtcIsoSeconds())
		result = map[string]any{
			"account":           buildAccountSummary(tx, updatedTarget, updatedTarget.Currency, nil),
			"movedTransactions": txMove.RowsAffected,
			"movedIdentifiers":  movedIdents,
			"suggestionIds":     suggestionIDs,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
