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
		if err := gdb.Raw(`
			SELECT account_id AS id FROM account_identifiers WHERE kind = 'card_last4' AND normalized_value = ?
			UNION
			SELECT id FROM accounts WHERE last4 = ?`, *params.CardLast4, *params.CardLast4).Scan(&ids).Error; err == nil {
			if len(ids) == 1 {
				return &ids[0]
			}
			if len(ids) > 1 {
				cardStatus = "collision"
			}
		}
	}

	// 2. Unique bank_account hint
	bankStatus := cardStatus
	if cardStatus == "none" && params.BankAccountHint != nil {
		digits := nonDigitsRe.ReplaceAllString(*params.BankAccountHint, "")
		if len(digits) >= 4 {
			var bankRows []db.AccountIdentifier
			if err := gdb.Where("kind = ?", "bank_account").Find(&bankRows).Error; err == nil {
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
	}

	// 3. Unique name or alias
	if bankStatus == "none" && params.PaymentMethod != nil && strings.TrimSpace(*params.PaymentMethod) != "" {
		norm := normalizeAliasText(*params.PaymentMethod)
		matches := map[string]struct{}{}
		var accts []db.Account
		if err := gdb.Select("id, name").Find(&accts).Error; err == nil {
			for _, a := range accts {
				if normalizeAliasText(a.Name) == norm {
					matches[a.ID] = struct{}{}
				}
			}
		}
		var aliasIds []string
		if err := gdb.Model(&db.AccountIdentifier{}).
			Distinct("account_id").
			Where("kind = ? AND normalized_value = ?", "alias", norm).
			Pluck("account_id", &aliasIds).Error; err == nil {
			for _, id := range aliasIds {
				matches[id] = struct{}{}
			}
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
		var accountID string
		if err := gdb.Raw(`
			SELECT accounts.id
			FROM senders
			JOIN accounts ON accounts.id = senders.default_account_id
			WHERE senders.id = ?`, *params.SenderID).Scan(&accountID).Error; err == nil && accountID != "" {
			return &accountID
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
		Last4:                 validLast4,
		Status:                "provisional",
		CreatedAtUtc:          nowUtc,
	}
	if err := tx.Create(&acct).Error; err != nil {
		return nil, err
	}
	syncedLast4, err := replaceAccountIdentifiers(tx, id, identifiers, nowUtc)
	if err != nil {
		return nil, err
	}
	if syncedLast4 != validLast4 {
		if err := tx.Model(&db.Account{}).Where("id = ?", id).Update("last4", syncedLast4).Error; err != nil {
			return nil, err
		}
		acct.Last4 = syncedLast4
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

		sourceIdents, err := listIdentifiersForAccount(tx, sourceAccountID)
		if err != nil {
			return err
		}
		movedIdents := 0
		for _, ident := range sourceIdents {
			res := tx.Exec(`
				INSERT OR IGNORE INTO account_identifiers (
					id, account_id, kind, value, normalized_value, created_at_utc
				) VALUES (?, ?, ?, ?, ?, ?)`,
				uuid.NewString(), targetAccountID, ident.Kind, ident.Value, ident.NormalizedValue, ident.CreatedAtUtc,
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
			"movedTransactions": txMove.RowsAffected,
			"movedIdentifiers":  movedIdents,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
