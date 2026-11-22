package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

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

type BillingCycleSpendSummary struct {
	StartDate        string `json:"start_date"`
	EndDate          string `json:"end_date"`
	SpentMinor       int64  `json:"spent_minor"`
	TransactionCount int64  `json:"transaction_count"`
}

type CreditCardBillingCycleSummary struct {
	StatementDay      int                      `json:"statement_day"`
	LastStatementDate string                   `json:"last_statement_date"`
	CurrentCycle      BillingCycleSpendSummary `json:"current_cycle"`
	LastCycle         BillingCycleSpendSummary `json:"last_cycle"`
}

type AccountSummary struct {
	db.Account
	Identifiers            []db.AccountIdentifier         `json:"identifiers"`
	ComputedBalanceMinor   *int64                         `json:"computed_balance_minor"`
	NetFlowMinor           int64                          `json:"net_flow_minor"`
	UncountedForeignCount  int64                          `json:"uncounted_foreign_count"`
	FutureTransactionCount int64                          `json:"future_transaction_count"`
	LatestReconciliation   *db.AccountReconciliation      `json:"latest_reconciliation"`
	BillingCycle           *CreditCardBillingCycleSummary `json:"billing_cycle"`
	TypeLocked             bool                           `json:"type_locked"`
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
	if err := gdb.Model(&db.Account{}).Where("id = ?", accountID).Count(&cnt).Error; err != nil {
		return err
	}
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

func validateStatementDay(accountType string, statementDay *int) (*int, error) {
	if statementDay == nil {
		return nil, nil
	}
	if accountType != "credit_card" {
		return nil, errors.New("Statement day is only supported for credit card accounts.")
	}
	if *statementDay < 1 || *statementDay > 31 {
		return nil, errors.New("Statement day must be between 1 and 31.")
	}
	v := *statementDay
	return &v, nil
}

func listIdentifiersForAccount(gdb *gorm.DB, accountID string) ([]db.AccountIdentifier, error) {
	rows := []db.AccountIdentifier{}
	if err := gdb.Where("account_id = ?", accountID).Order("created_at_utc ASC, rowid ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
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

func hasAccountLedgerRows(gdb *gorm.DB, accountID string) (bool, error) {
	var txCnt int64
	if err := gdb.Model(&db.Transaction{}).Where("account_id = ?", accountID).Count(&txCnt).Error; err != nil {
		return false, err
	}
	if txCnt > 0 {
		return true, nil
	}
	var recCnt int64
	if err := gdb.Model(&db.AccountReconciliation{}).Where("account_id = ?", accountID).Count(&recCnt).Error; err != nil {
		return false, err
	}
	return recCnt > 0, nil
}

func ListAccounts(gdb *gorm.DB) ([]AccountSummary, error) {
	return listAccountsAt(gdb, time.Now().UTC())
}

func listAccountsAt(gdb *gorm.DB, now time.Time) ([]AccountSummary, error) {
	targetCurrency, targetTimezone, err := resolveAppCurrencyAndTimezone(gdb, "", "")
	if err != nil {
		return nil, err
	}

	var rows []db.Account
	if err := gdb.Order("created_at_utc ASC, name ASC").Find(&rows).Error; err != nil {
		return nil, err
	}

	var allIdents []db.AccountIdentifier
	if err := gdb.Order("created_at_utc ASC, rowid ASC").Find(&allIdents).Error; err != nil {
		return nil, err
	}
	byAcct := map[string][]db.AccountIdentifier{}
	for _, ident := range allIdents {
		byAcct[ident.AccountID] = append(byAcct[ident.AccountID], ident)
	}

	if len(rows) == 0 {
		return []AccountSummary{}, nil
	}

	return buildAccountSummariesAt(gdb, rows, byAcct, targetCurrency, targetTimezone, now)
}

type CreateAccountInput struct {
	Name                  string                   `json:"name"`
	Type                  string                   `json:"type"`
	InitialBalanceMinor   int64                    `json:"initial_balance_minor"`
	InitialBalanceAsOfUtc *string                  `json:"initial_balance_as_of_utc"`
	Last4                 *string                  `json:"last4"`
	StatementDay          *int                     `json:"statement_day"`
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
	statementDay, err := validateStatementDay(input.Type, input.StatementDay)
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
			StatementDay:          statementDay,
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

		built, err := buildAccountSummary(tx, acct, currency, nil)
		if err != nil {
			return err
		}
		summary = built
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
	HasStatementDay       bool                     `json:"-"`
	StatementDay          *int                     `json:"statement_day"`
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

		bal, err := computeAccountBalance(tx, current, "")
		if err != nil {
			return err
		}
		locked, err := isAccountTypeLocked(tx, current, bal.LatestReconciliation != nil)
		if err != nil {
			return err
		}
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
			last4, err = validateLast4(patch.Last4)
			if err != nil {
				return err
			}
		}

		if patch.HasIdentifiers {
			last4, err = replaceAccountIdentifiers(tx, id, patch.Identifiers, NowUtcIsoSeconds())
			if err != nil {
				return err
			}
		} else if patch.HasLast4 {
			existing, err := listIdentifiersForAccount(tx, id)
			if err != nil {
				return err
			}
			var nextList []AccountIdentifierInput
			if last4 != nil {
				nextList = append(nextList, AccountIdentifierInput{Kind: "card_last4", Value: *last4})
			}
			for _, r := range existing {
				if r.Kind != "card_last4" {
					nextList = append(nextList, AccountIdentifierInput{Kind: r.Kind, Value: r.Value})
				}
			}
			last4, err = replaceAccountIdentifiers(tx, id, nextList, NowUtcIsoSeconds())
			if err != nil {
				return err
			}
		}

		statementDay := current.StatementDay
		if patch.HasStatementDay {
			statementDay = patch.StatementDay
		} else if nextType != "credit_card" {
			statementDay = nil
		}
		statementDay, err = validateStatementDay(nextType, statementDay)
		if err != nil {
			return err
		}

		if err := tx.Model(&db.Account{}).Where("id = ?", id).Updates(map[string]any{
			"name":                      name,
			"type":                      nextType,
			"initial_balance_minor":     initBal,
			"initial_balance_as_of_utc": asOfUtc,
			"last4":                     last4,
			"statement_day":             statementDay,
			"status":                    "confirmed",
		}).Error; err != nil {
			return err
		}

		var updated db.Account
		if err := tx.Where("id = ?", id).First(&updated).Error; err != nil {
			return err
		}
		built, err := buildAccountSummary(tx, updated, "", nil)
		if err != nil {
			return err
		}
		summary = built
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
		built, err := buildAccountSummary(tx, current, "", nil)
		if err != nil {
			return err
		}
		summary = built
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
