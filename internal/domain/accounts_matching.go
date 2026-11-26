package domain

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"portspendernote/internal/db"
)

func ListExtractionAccountContexts(gdb *gorm.DB) []ExtractionAccountContext {
	var accounts []db.Account
	if err := gdb.Order("created_at_utc ASC, name ASC").Find(&accounts).Error; err != nil || len(accounts) == 0 {
		return []ExtractionAccountContext{}
	}
	var idents []db.AccountIdentifier
	if err := gdb.Order("created_at_utc ASC, rowid ASC").Find(&idents).Error; err != nil {
		return []ExtractionAccountContext{}
	}
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

func SenderDefaultAccountID(gdb *gorm.DB, senderID *string) *string {
	if senderID == nil || *senderID == "" {
		return nil
	}
	var accountID string
	if err := gdb.Raw(`
		SELECT accounts.id
		FROM senders
		JOIN accounts ON accounts.id = senders.default_account_id
		WHERE senders.id = ?`, *senderID).Scan(&accountID).Error; err == nil && accountID != "" {
		return &accountID
	}
	return nil
}

func MatchAccountForExtraction(gdb *gorm.DB, senderID, llmAccountID *string) *string {
	if accountID := SenderDefaultAccountID(gdb, senderID); accountID != nil {
		return accountID
	}
	if llmAccountID != nil && strings.TrimSpace(*llmAccountID) != "" {
		var acct db.Account
		if err := gdb.Select("id").Where("id = ?", strings.TrimSpace(*llmAccountID)).First(&acct).Error; err == nil {
			return &acct.ID
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
	FromAddress string

	BankAccountHint  *string
	PaymentMethod    *string
	AccountTypeGuess *string
	Currency         *string
}

func CreateProvisionalAccountForExtraction(gdb *gorm.DB, params ProvisionalAccountParams) (*AccountSummary, error) {
	var summary *AccountSummary
	err := gdb.Transaction(func(tx *gorm.DB) error {
		var createErr error
		summary, createErr = CreateProvisionalAccountForExtractionInTx(tx, params)
		return createErr
	})
	if err != nil {
		return nil, err
	}
	return summary, nil
}

// CreateProvisionalAccountForExtractionInTx creates a provisional account inside an
// already-open transaction without starting a nested transaction.
func CreateProvisionalAccountForExtractionInTx(tx *gorm.DB, params ProvisionalAccountParams) (*AccountSummary, error) {
	var identifiers []AccountIdentifierInput

	bankHint := ""
	if params.BankAccountHint != nil {
		bankHint = strings.TrimSpace(*params.BankAccountHint)
	}
	if bankHint != "" {
		identifiers = append(identifiers, AccountIdentifierInput{Value: bankHint})
	}
	wallet := ""
	if params.PaymentMethod != nil {
		wallet = strings.TrimSpace(*params.PaymentMethod)
	}
	if wallet != "" {
		identifiers = append(identifiers, AccountIdentifierInput{Value: wallet})
	}
	if len(identifiers) == 0 {
		return nil, nil
	}

	resolvedType := "digital_wallet"
	if params.AccountTypeGuess != nil && isValidAccountType(*params.AccountTypeGuess) {
		resolvedType = *params.AccountTypeGuess
	} else if bankHint != "" {
		resolvedType = "debit_card"
	}

	name := deriveSenderLabel(params.FromAddress) + " " + identifiers[0].Value
	currency := ""
	if params.Currency != nil && len(strings.TrimSpace(*params.Currency)) == 3 {
		currency = strings.ToUpper(strings.TrimSpace(*params.Currency))
	} else {
		var err error
		currency, err = getConfiguredAppCurrency(tx)
		if err != nil {
			return nil, err
		}
	}

	id := uuid.NewString()
	nowUtc := NowUtcIsoSeconds()

	acct := db.Account{
		ID:                    id,
		Name:                  name,
		Type:                  resolvedType,
		Currency:              currency,
		InitialBalanceMinor:   nil,
		InitialBalanceAsOfUtc: nowUtc,
		Status:                "provisional",
		CreatedAtUtc:          nowUtc,
	}
	if err := tx.Create(&acct).Error; err != nil {
		return nil, err
	}
	if err := replaceAccountIdentifiers(tx, id, identifiers, nowUtc); err != nil {
		return nil, err
	}
	idents, err := listIdentifiersForAccount(tx, id)
	if err != nil {
		return nil, err
	}
	summary := &AccountSummary{
		Account:               acct,
		Identifiers:           idents,
		ComputedBalanceMinor:  nil,
		NetFlowMinor:          0,
		UncountedForeignCount: 0,
		LatestReconciliation:  nil,
		TypeLocked:            false,
	}
	return summary, nil
}

func MergeAccounts(gdb *gorm.DB, sourceAccountID, targetAccountID string) (map[string]any, error) {
	if sourceAccountID == targetAccountID {
		return nil, errors.New("Cannot merge an account into itself.")
	}

	var result map[string]any
	err := db.RunImmediateTx(gdb, func(tx *gorm.DB) error {
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

		var collapsed db.Transaction
		err := tx.Select("id").Where(
			"(account_id = ? AND destination_account_id = ?) OR (account_id = ? AND destination_account_id = ?)",
			sourceAccountID, targetAccountID, targetAccountID, sourceAccountID,
		).Take(&collapsed).Error
		if err == nil {
			return fmt.Errorf("Resolve transfer %s between these accounts before merging them.", collapsed.ID)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		txMove := tx.Model(&db.Transaction{}).Where("account_id = ?", sourceAccountID).Update("account_id", targetAccountID)
		if txMove.Error != nil {
			return txMove.Error
		}
		destinationMove := tx.Model(&db.Transaction{}).
			Where("destination_account_id = ?", sourceAccountID).
			Update("destination_account_id", targetAccountID)
		if destinationMove.Error != nil {
			return destinationMove.Error
		}

		sourceIdents, err := listIdentifiersForAccount(tx, sourceAccountID)
		if err != nil {
			return err
		}
		movedIdents := 0
		for _, ident := range sourceIdents {
			res := tx.Exec(`
				INSERT OR IGNORE INTO account_identifiers (
					id, account_id, value, created_at_utc
				) VALUES (?, ?, ?, ?)`,
				uuid.NewString(), targetAccountID, ident.Value, ident.CreatedAtUtc,
			)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected > 0 {
				movedIdents++
			}
		}

		if err := tx.Model(&db.Sender{}).Where("default_account_id = ?", sourceAccountID).Update("default_account_id", targetAccountID).Error; err != nil {
			return err
		}

		updatedTarget := target
		if target.InitialBalanceMinor == nil {
			var targetRecCnt int64
			if err := tx.Model(&db.AccountReconciliation{}).Where("account_id = ?", targetAccountID).Count(&targetRecCnt).Error; err != nil {
				return err
			}
			if targetRecCnt == 0 {
				var sourceRec db.AccountReconciliation
				recErr := tx.Where("account_id = ?", sourceAccountID).Order("recorded_at_utc DESC, rowid DESC").First(&sourceRec).Error
				if recErr != nil && !errors.Is(recErr, gorm.ErrRecordNotFound) {
					return recErr
				}
				if recErr == nil {
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
					if err := tx.Create(&newRec).Error; err != nil {
						return err
					}
				} else if source.InitialBalanceMinor != nil {
					if err := tx.Model(&db.Account{}).Where("id = ?", targetAccountID).Updates(map[string]any{
						"initial_balance_minor":     source.InitialBalanceMinor,
						"initial_balance_as_of_utc": source.InitialBalanceAsOfUtc,
					}).Error; err != nil {
						return err
					}
					updatedTarget.InitialBalanceMinor = source.InitialBalanceMinor
					updatedTarget.InitialBalanceAsOfUtc = source.InitialBalanceAsOfUtc
				}
			}
		}

		if err := tx.Where("id = ?", sourceAccountID).Delete(&db.Account{}).Error; err != nil {
			return err
		}

		built, err := buildAccountSummary(tx, updatedTarget, nil)
		if err != nil {
			return err
		}
		result = map[string]any{
			"account":           built,
			"movedTransactions": txMove.RowsAffected + destinationMove.RowsAffected,
			"movedIdentifiers":  movedIdents,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
