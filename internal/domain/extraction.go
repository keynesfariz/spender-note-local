package domain

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"portspendernote/internal/db"
)

type ExtractedTransactionInput struct {
	EmailID       string
	EmailKind     string
	AmountMinor   int64
	Currency      string
	OccurredAtUtc string
	MerchantRaw   string
	Category      string

	ReferenceCode     *string
	ResolvedAccountID *string
	NowUtc            string
}

func RecomputeTransactionFromSources(tx *gorm.DB, transactionID string, nowUtc string) error {
	if nowUtc == "" {
		nowUtc = NowUtcIsoSeconds()
	}
	var t db.Transaction
	if err := tx.Select("id, kind, source_type, manual_override").Where("id = ?", transactionID).First(&t).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if t.SourceType == "manual" || t.Kind != "expense" {
		return nil
	}
	if t.ManualOverride == 1 {
		return tx.Model(&db.Transaction{}).Where("id = ?", transactionID).Update("updated_at_utc", nowUtc).Error
	}

	type sourceExt struct {
		Role          string  `gorm:"column:role"`
		AmountMinor   *int64  `gorm:"column:amount_minor"`
		Currency      *string `gorm:"column:currency"`
		OccurredAtUtc *string `gorm:"column:occurred_at_utc"`
		MerchantRaw   *string `gorm:"column:merchant_raw"`
		Category      *string `gorm:"column:category"`

		LlmAccountID *string `gorm:"column:llm_account_id"`
		SenderID     *string `gorm:"column:sender_id"`
	}
	var sources []sourceExt
	if err := tx.Raw(`
		SELECT
			ts.role,
			ee.amount_minor, ee.currency, ee.occurred_at_utc,
			ee.merchant_raw, ee.category,
			ee.llm_account_id,
			re.sender_id
		FROM transaction_sources ts
		JOIN raw_emails re ON re.id = ts.email_id
		LEFT JOIN email_extractions ee ON ee.email_id = ts.email_id
		WHERE ts.transaction_id = ?
		ORDER BY re.internal_date_ms ASC, ts.email_id ASC`, transactionID).Scan(&sources).Error; err != nil {
		return err
	}
	if len(sources) == 0 {
		return nil
	}

	var cardCharge, receipt *sourceExt
	for i := range sources {
		if sources[i].Role == "card_charge" && cardCharge == nil {
			cardCharge = &sources[i]
		}
		if sources[i].Role == "receipt" && receipt == nil {
			receipt = &sources[i]
		}
	}
	primary := &sources[0]
	if cardCharge != nil {
		primary = cardCharge
	}

	merchant := "Unknown Merchant"
	if receipt != nil && receipt.MerchantRaw != nil && *receipt.MerchantRaw != "" {
		merchant = *receipt.MerchantRaw
	} else if primary.MerchantRaw != nil && *primary.MerchantRaw != "" {
		merchant = *primary.MerchantRaw
	}

	category := DefaultCategory
	if receipt != nil && receipt.Category != nil && *receipt.Category != "" {
		category = *receipt.Category
	} else if primary.Category != nil && *primary.Category != "" {
		category = *primary.Category
	}

	var accountID *string
	if cardCharge != nil {
		accountID = MatchAccountForExtraction(tx, cardCharge.SenderID, cardCharge.LlmAccountID)
	}
	if accountID == nil {
		for i := range sources {
			if cardCharge != nil && &sources[i] == cardCharge {
				continue
			}
			accountID = MatchAccountForExtraction(tx, sources[i].SenderID, sources[i].LlmAccountID)
			if accountID != nil {
				break
			}
		}
	}

	if primary.AmountMinor != nil && primary.Currency != nil && primary.OccurredAtUtc != nil {
		resolvedCategory, err := resolveTransactionCategory(tx, category)
		if err != nil {
			return err
		}
		if err := tx.Exec(`
			UPDATE transactions
			SET account_id = COALESCE(?, account_id),
				amount_minor = ?,
				currency = ?,
				occurred_at_utc = ?,
				merchant = ?,
				category_id = ?,
				updated_at_utc = ?
			WHERE id = ?`,
			accountID, *primary.AmountMinor, *primary.Currency, *primary.OccurredAtUtc,
			merchant, resolvedCategory.ID, nowUtc, transactionID,
		).Error; err != nil {
			return err
		}
	}
	return nil
}

// ResolveEffectiveEmailRole resolves transaction source roles (`card_charge` or
// `receipt`) consistently across sync admission, Ghost Row promotion, and Settings recovery.
func ResolveEffectiveEmailRole(senderRoleHint *string, emailKind string, bankAccountHint *string) string {
	if senderRoleHint != nil {
		hint := strings.TrimSpace(*senderRoleHint)
		if hint == "card_charge" || hint == "receipt" {
			return hint
		}
	}
	if emailKind == "card_charge" || emailKind == "receipt" {
		return emailKind
	}
	if bankAccountHint != nil && strings.TrimSpace(*bankAccountHint) != "" {
		return "card_charge"
	}
	return "receipt"
}

func SaveExtractedTransaction(gdb *gorm.DB, input ExtractedTransactionInput) (string, error) {
	var txID string
	err := gdb.Transaction(func(tx *gorm.DB) error {
		var saveErr error
		txID, saveErr = SaveExtractedTransactionInTx(tx, input)
		return saveErr
	})
	return txID, err
}

// SaveExtractedTransactionInTx persists an extracted transaction and source link
// within an already-open transaction without starting a nested transaction.
func SaveExtractedTransactionInTx(tx *gorm.DB, input ExtractedTransactionInput) (string, error) {
	nowUtc := input.NowUtc
	if nowUtc == "" {
		nowUtc = NowUtcIsoSeconds()
	}
	if _, err := UtcIsoToEpochMs(input.OccurredAtUtc); err != nil {
		return "", err
	}
	category, err := resolveTransactionCategory(tx, input.Category)
	if err != nil {
		return "", err
	}

	merchantRawPtr := strPtrOrNil(input.MerchantRaw)

	if err := tx.Exec(`
		INSERT OR IGNORE INTO email_extractions (
			id, email_id, email_kind, amount_minor, currency, occurred_at_utc,
			merchant_raw, category, reference_code,
			extraction_model, raw_llm_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'sync', '{}')`,
		uuid.NewString(), input.EmailID, input.EmailKind, input.AmountMinor, input.Currency,
		input.OccurredAtUtc, merchantRawPtr, category.Name, input.ReferenceCode,
	).Error; err != nil {
		return "", err
	}

	resolvedAccount := input.ResolvedAccountID

	merchant := "Unknown Merchant"
	if merchantRawPtr != nil {
		merchant = *merchantRawPtr
	}
	txID := uuid.NewString()
	if err := tx.Create(&db.Transaction{
		ID:             txID,
		SourceType:     "gmail",
		AccountID:      resolvedAccount,
		AmountMinor:    input.AmountMinor,
		Currency:       input.Currency,
		OccurredAtUtc:  input.OccurredAtUtc,
		Merchant:       merchant,
		CategoryID:     category.ID,
		ManualOverride: 0,
		Reviewed:       0,
		CreatedAtUtc:   nowUtc,
		UpdatedAtUtc:   nowUtc,
	}).Error; err != nil {
		return "", err
	}

	if err := tx.Create(&db.TransactionSource{
		ID:            uuid.NewString(),
		TransactionID: txID,
		EmailID:       input.EmailID,
		Role:          input.EmailKind,
		MatchScore:    1.0,
		MatchState:    "single",
	}).Error; err != nil {
		return "", err
	}
	return txID, nil
}

func SplitTransactionSource(gdb *gorm.DB, transactionID, emailID string) (string, error) {
	nowUtc := NowUtcIsoSeconds()
	var newTxID string
	err := gdb.Transaction(func(tx *gorm.DB) error {
		var current db.Transaction
		if err := tx.Select("kind, ambiguous_source_amount").First(&current, "id = ?", transactionID).Error; err != nil {
			return err
		}
		if current.Kind != "expense" {
			return errors.New("Only expenses support source splitting.")
		}
		var sources []db.TransactionSource
		if err := tx.Where("transaction_id = ?", transactionID).Find(&sources).Error; err != nil {
			return err
		}
		if len(sources) < 2 {
			return fmt.Errorf("Transaction %s must have at least 2 sources to split", transactionID)
		}
		var detached *db.TransactionSource
		var remaining []db.TransactionSource
		for i := range sources {
			if sources[i].EmailID == emailID {
				detached = &sources[i]
			} else {
				remaining = append(remaining, sources[i])
			}
		}
		if detached == nil {
			return fmt.Errorf("Email %s is not attached to transaction %s", emailID, transactionID)
		}

		type extInfo struct {
			AmountMinor   *int64  `gorm:"column:amount_minor"`
			Currency      *string `gorm:"column:currency"`
			OccurredAtUtc *string `gorm:"column:occurred_at_utc"`
			MerchantRaw   *string `gorm:"column:merchant_raw"`
			Category      *string `gorm:"column:category"`

			LlmAccountID *string `gorm:"column:llm_account_id"`
			SenderID     *string `gorm:"column:sender_id"`
		}
		var ext extInfo
		if err := tx.Raw(`
			SELECT ee.amount_minor, ee.currency, ee.occurred_at_utc, ee.merchant_raw,
			       ee.category, ee.bank_account_hint, ee.payment_method_raw,
			       ee.llm_account_id, re.sender_id
			FROM raw_emails re
			LEFT JOIN email_extractions ee ON ee.email_id = re.id
			WHERE re.id = ?`, emailID).Scan(&ext).Error; err != nil {
			return err
		}

		newTxID = uuid.NewString()
		acctID := MatchAccountForExtraction(tx, ext.SenderID, ext.LlmAccountID)

		amountMinor := int64(1)
		if ext.AmountMinor != nil {
			amountMinor = *ext.AmountMinor
		}
		currency := "USD"
		if ext.Currency != nil && *ext.Currency != "" {
			currency = *ext.Currency
		}
		occurred := nowUtc
		if ext.OccurredAtUtc != nil && *ext.OccurredAtUtc != "" {
			occurred = *ext.OccurredAtUtc
		}
		merchant := "Unknown Merchant"
		if ext.MerchantRaw != nil && *ext.MerchantRaw != "" {
			merchant = *ext.MerchantRaw
		}
		category := DefaultCategory
		if ext.Category != nil && *ext.Category != "" {
			category = *ext.Category
		}

		resolvedCategory, err := resolveTransactionCategory(tx, category)
		if err != nil {
			return err
		}

		if err := tx.Create(&db.Transaction{
			ID:                    newTxID,
			SourceType:            "gmail",
			AccountID:             acctID,
			AmountMinor:           amountMinor,
			Currency:              currency,
			OccurredAtUtc:         occurred,
			Merchant:              merchant,
			CategoryID:            resolvedCategory.ID,
			ManualOverride:        0,
			AmbiguousSourceAmount: current.AmbiguousSourceAmount,
			Reviewed:              0,
			CreatedAtUtc:          nowUtc,
			UpdatedAtUtc:          nowUtc,
		}).Error; err != nil {
			return err
		}

		if err := tx.Model(&db.TransactionSource{}).Where("id = ?", detached.ID).Updates(map[string]any{
			"transaction_id": newTxID,
			"match_state":    "single",
			"match_score":    1.0,
		}).Error; err != nil {
			return err
		}

		if err := copyTransactionTags(tx, newTxID, []string{transactionID}, nowUtc); err != nil {
			return err
		}

		if len(remaining) == 1 {
			if err := tx.Model(&db.TransactionSource{}).Where("id = ?", remaining[0].ID).Updates(map[string]any{
				"match_state": "single",
				"match_score": 1.0,
			}).Error; err != nil {
				return err
			}
		}

		if err := RecomputeTransactionFromSources(tx, transactionID, nowUtc); err != nil {
			return err
		}
		if err := RecomputeTransactionFromSources(tx, newTxID, nowUtc); err != nil {
			return err
		}
		return nil
	})
	return newTxID, err
}
