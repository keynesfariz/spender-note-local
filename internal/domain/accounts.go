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
	displayNameRe = regexp.MustCompile(`^"?([^"<]+?)"?\s*<[^>]+>$`)
	angleEmailRe  = regexp.MustCompile(`<([^>]+)>`)
)

type AccountIdentifierInput struct {
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

type AccountOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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

func replaceAccountIdentifiers(tx *gorm.DB, accountID string, identifiers []AccountIdentifierInput, createdAtUtc string) error {
	if err := tx.Where("account_id = ?", accountID).Delete(&db.AccountIdentifier{}).Error; err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, item := range identifiers {
		value := strings.TrimSpace(item.Value)
		if value == "" {
			return errors.New("Identifier value is required.")
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		if err := tx.Create(&db.AccountIdentifier{
			ID: uuid.NewString(), AccountID: accountID, Value: value, CreatedAtUtc: createdAtUtc,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func getConfiguredAppCurrency(gdb *gorm.DB) (string, error) {
	var row db.AppSetting
	if err := gdb.First(&row, 1).Error; err != nil || row.Currency == nil || *row.Currency == "" {
		return "", errors.New("Configure your country and currency in Settings before managing accounts.")
	}
	return *row.Currency, nil
}

func hasAccountLedgerRows(gdb *gorm.DB, accountID string) (bool, error) {
	var exists bool
	if err := gdb.Raw(`
		SELECT EXISTS (
			SELECT 1 FROM transactions WHERE account_id = ? OR destination_account_id = ?
		)`, accountID, accountID).Scan(&exists).Error; err != nil {
		return false, err
	}
	if exists {
		return true, nil
	}

	err := gdb.Raw(`
		SELECT EXISTS (SELECT 1 FROM account_reconciliations WHERE account_id = ?)
	`, accountID).Scan(&exists).Error
	return exists && err == nil, err
}

func hasTypedAccountMovements(gdb *gorm.DB, accountID string) (bool, error) {
	var exists bool
	err := gdb.Raw(`
		SELECT EXISTS (
			SELECT 1 FROM transactions
			WHERE (account_id = ? AND kind IN ('income', 'transfer')) OR destination_account_id = ?
		)`, accountID, accountID).Scan(&exists).Error
	return exists, err
}

func ListAccounts(gdb *gorm.DB) ([]AccountSummary, error) {
	return listAccountsAt(gdb, time.Now().UTC())
}

func ListAccountOptions(gdb *gorm.DB) ([]AccountOption, error) {
	options := []AccountOption{}
	err := gdb.Model(&db.Account{}).Select("id", "name").
		Order("created_at_utc ASC, name ASC").Find(&options).Error
	return options, err
}

func listAccountsAt(gdb *gorm.DB, now time.Time) ([]AccountSummary, error) {
	targetTimezone, err := resolveAccountTimezone(gdb, "")
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

	return buildAccountSummariesAt(gdb, rows, byAcct, targetTimezone, now)
}

type CreateAccountInput struct {
	Name                  string                   `json:"name"`
	Type                  string                   `json:"type"`
	InitialBalanceMinor   int64                    `json:"initial_balance_minor"`
	InitialBalanceAsOfUtc *string                  `json:"initial_balance_as_of_utc"`
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
			StatementDay:          statementDay,
			Status:                "confirmed",
			CreatedAtUtc:          createdAt,
		}
		if err := tx.Create(&acct).Error; err != nil {
			return err
		}

		if input.HasIdentifiers {
			if err := replaceAccountIdentifiers(tx, id, input.Identifiers, createdAt); err != nil {
				return err
			}
		}

		built, err := buildAccountSummary(tx, acct, nil)
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
	HasStatementDay       bool                     `json:"-"`
	StatementDay          *int                     `json:"statement_day"`
	HasIdentifiers        bool                     `json:"-"`
	Identifiers           []AccountIdentifierInput `json:"identifiers"`
}

func UpdateAccount(gdb *gorm.DB, id string, patch UpdateAccountInput) (*AccountSummary, error) {
	var summary AccountSummary
	err := db.RunImmediateTx(gdb, func(tx *gorm.DB) error {
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

		bal, err := computeAccountBalance(tx, current)
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

		if patch.HasIdentifiers {
			if err := replaceAccountIdentifiers(tx, id, patch.Identifiers, NowUtcIsoSeconds()); err != nil {
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
			"statement_day":             statementDay,
			"status":                    "confirmed",
		}).Error; err != nil {
			return err
		}

		var updated db.Account
		if err := tx.Where("id = ?", id).First(&updated).Error; err != nil {
			return err
		}
		built, err := buildAccountSummary(tx, updated, nil)
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
		built, err := buildAccountSummary(tx, current, nil)
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
	return db.RunImmediateTx(gdb, func(tx *gorm.DB) error {
		hasMovements, err := hasTypedAccountMovements(tx, id)
		if err != nil {
			return err
		}
		if hasMovements {
			return errors.New("This account still has income or transfers. Move or delete those transactions first.")
		}
		res := tx.Where("id = ?", id).Delete(&db.Account{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errors.New("Account not found.")
		}
		return nil
	})
}
