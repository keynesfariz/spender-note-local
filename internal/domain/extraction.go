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

const maxStackedPrefixPasses = 2

var (
	processorPrefixRe = regexp.MustCompile(`(?i)^(?:sq\s*\*|tst\s*\*|paypal\s*\*|pp\s*\*|apl\s*\*|apple\s*pay\s*\*|amzn\s*mktp|amzn\s*\*|stripe\s*\*|sp\s*\*|goog\s*\*|pos\s*\*)\s*`)
	storeIDRe         = regexp.MustCompile(`(?i)(?:#\s*\d+|\*\s*[a-z0-9]{3,}|\b(?:store|loc)\s*#?\s*\d+\b)`)
	legalSuffixRe     = regexp.MustCompile(`(?i)\b(?:inc|llc|ltd|corp|co|plc|pt|tbk|sdn\s+bhd|pte|gmbh|s\.a\.|sa)\b\.?`)
	nonAlnumRe        = regexp.MustCompile(`[^a-z0-9]+`)
)

type ExtractedTransactionInput struct {
	EmailID           string
	EmailKind         string
	AmountMinor       int64
	Currency          string
	OccurredAtUtc     string
	MerchantRaw       string
	MerchantNorm      string
	Category          string
	CardLast4         *string
	ReferenceCode     *string
	ResolvedAccountID *string
	NowUtc            string
}

func NormalizeMerchant(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	for i := 0; i < maxStackedPrefixPasses; i++ {
		s = processorPrefixRe.ReplaceAllString(s, "")
	}
	s = storeIDRe.ReplaceAllString(s, " ")
	s = legalSuffixRe.ReplaceAllString(s, " ")
	s = nonAlnumRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(multiSpaceRe.ReplaceAllString(s, " "))
}

func RecomputeTransactionFromSources(tx *gorm.DB, transactionID string, nowUtc string) error {
	if nowUtc == "" {
		nowUtc = NowUtcIsoSeconds()
	}
	var t db.Transaction
	if err := tx.Select("id, source_type, manual_override").Where("id = ?", transactionID).First(&t).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if t.SourceType == "manual" {
		return nil
	}
	if t.ManualOverride == 1 {
		return tx.Model(&db.Transaction{}).Where("id = ?", transactionID).Update("updated_at_utc", nowUtc).Error
	}

	type sourceExt struct {
		EmailID          string  `gorm:"column:email_id"`
		Role             string  `gorm:"column:role"`
		AmountMinor      *int64  `gorm:"column:amount_minor"`
		Currency         *string `gorm:"column:currency"`
		OccurredAtUtc    *string `gorm:"column:occurred_at_utc"`
		MerchantRaw      *string `gorm:"column:merchant_raw"`
		Category         *string `gorm:"column:category"`
		CardLast4        *string `gorm:"column:card_last4"`
		BankAccountHint  *string `gorm:"column:bank_account_hint"`
		PaymentMethodRaw *string `gorm:"column:payment_method_raw"`
		LlmAccountID     *string `gorm:"column:llm_account_id"`
		SenderID         *string `gorm:"column:sender_id"`
	}
	var sources []sourceExt
	if err := tx.Raw(`
		SELECT
			ts.email_id, ts.role,
			ee.amount_minor, ee.currency, ee.occurred_at_utc,
			ee.merchant_raw, ee.category, ee.card_last4,
			ee.bank_account_hint, ee.payment_method_raw, ee.llm_account_id,
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
		category = ResolveKnownCategory(tx, *receipt.Category)
	} else if primary.Category != nil && *primary.Category != "" {
		category = ResolveKnownCategory(tx, *primary.Category)
	}

	var accountID *string
	if cardCharge != nil {
		accountID = MatchAccountForExtraction(tx, MatchAccountParams{
			SenderID:        cardCharge.SenderID,
			CardLast4:       cardCharge.CardLast4,
			BankAccountHint: cardCharge.BankAccountHint,
			PaymentMethod:   cardCharge.PaymentMethodRaw,
			LlmAccountID:    cardCharge.LlmAccountID,
		})
	}
	if accountID == nil {
		for i := range sources {
			if cardCharge != nil && &sources[i] == cardCharge {
				continue
			}
			accountID = MatchAccountForExtraction(tx, MatchAccountParams{
				SenderID:        sources[i].SenderID,
				CardLast4:       sources[i].CardLast4,
				BankAccountHint: sources[i].BankAccountHint,
				PaymentMethod:   sources[i].PaymentMethodRaw,
				LlmAccountID:    sources[i].LlmAccountID,
			})
			if accountID != nil {
				break
			}
		}
	}

	if primary.AmountMinor != nil && primary.Currency != nil && primary.OccurredAtUtc != nil {
		if err := tx.Exec(`
			UPDATE transactions
			SET account_id = COALESCE(?, account_id),
				amount_minor = ?,
				currency = ?,
				occurred_at_utc = ?,
				merchant = ?,
				category = ?,
				updated_at_utc = ?
			WHERE id = ?`,
			accountID, *primary.AmountMinor, *primary.Currency, *primary.OccurredAtUtc,
			merchant, category, nowUtc, transactionID,
		).Error; err != nil {
			return err
		}
	}
	return nil
}

func SaveExtractedTransaction(gdb *gorm.DB, input ExtractedTransactionInput) (string, error) {
	nowUtc := input.NowUtc
	if nowUtc == "" {
		nowUtc = NowUtcIsoSeconds()
	}
	merchantNorm := input.MerchantNorm
	if merchantNorm == "" {
		merchantNorm = NormalizeMerchant(input.MerchantRaw)
	}
	if _, err := UtcIsoToEpochMs(input.OccurredAtUtc); err != nil {
		return "", err
	}

	var txID string
	err := gdb.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`
			INSERT OR IGNORE INTO email_extractions (
				id, email_id, email_kind, amount_minor, currency, occurred_at_utc,
				merchant_raw, merchant_norm, category, card_last4, reference_code,
				extraction_model, raw_llm_json
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'sync', '{}')`,
			uuid.NewString(), input.EmailID, input.EmailKind, input.AmountMinor, input.Currency,
			input.OccurredAtUtc, input.MerchantRaw, merchantNorm, input.Category, input.CardLast4, input.ReferenceCode,
		).Error; err != nil {
			return err
		}

		resolvedAccount := input.ResolvedAccountID
		if resolvedAccount == nil {
			resolvedAccount = MatchAccountForExtraction(tx, MatchAccountParams{CardLast4: input.CardLast4})
		}

		merchant := strings.TrimSpace(input.MerchantRaw)
		if merchant == "" {
			merchant = "Unknown Merchant"
		}
		category := ResolveKnownCategory(tx, input.Category)

		txID = uuid.NewString()
		if err := tx.Create(&db.Transaction{
			ID:             txID,
			SourceType:     "gmail",
			AccountID:      resolvedAccount,
			AmountMinor:    input.AmountMinor,
			Currency:       input.Currency,
			OccurredAtUtc:  input.OccurredAtUtc,
			Merchant:       merchant,
			Category:       category,
			ManualOverride: 0,
			Reviewed:       0,
			CreatedAtUtc:   nowUtc,
			UpdatedAtUtc:   nowUtc,
		}).Error; err != nil {
			return err
		}

		if err := tx.Create(&db.TransactionSource{
			ID:            uuid.NewString(),
			TransactionID: txID,
			EmailID:       input.EmailID,
			Role:          input.EmailKind,
			MatchScore:    1.0,
			MatchState:    "single",
		}).Error; err != nil {
			return err
		}
		return nil
	})
	return txID, err
}

func SplitTransactionSource(gdb *gorm.DB, transactionID, emailID string) (string, error) {
	nowUtc := NowUtcIsoSeconds()
	var newTxID string
	err := gdb.Transaction(func(tx *gorm.DB) error {
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
			AmountMinor      *int64  `gorm:"column:amount_minor"`
			Currency         *string `gorm:"column:currency"`
			OccurredAtUtc    *string `gorm:"column:occurred_at_utc"`
			MerchantRaw      *string `gorm:"column:merchant_raw"`
			Category         *string `gorm:"column:category"`
			CardLast4        *string `gorm:"column:card_last4"`
			BankAccountHint  *string `gorm:"column:bank_account_hint"`
			PaymentMethodRaw *string `gorm:"column:payment_method_raw"`
			LlmAccountID     *string `gorm:"column:llm_account_id"`
			SenderID         *string `gorm:"column:sender_id"`
		}
		var ext extInfo
		if err := tx.Raw(`
			SELECT ee.amount_minor, ee.currency, ee.occurred_at_utc, ee.merchant_raw,
			       ee.category, ee.card_last4, ee.bank_account_hint, ee.payment_method_raw,
			       ee.llm_account_id, re.sender_id
			FROM raw_emails re
			LEFT JOIN email_extractions ee ON ee.email_id = re.id
			WHERE re.id = ?`, emailID).Scan(&ext).Error; err != nil {
			return err
		}

		newTxID = uuid.NewString()
		acctID := MatchAccountForExtraction(tx, MatchAccountParams{
			SenderID:        ext.SenderID,
			CardLast4:       ext.CardLast4,
			BankAccountHint: ext.BankAccountHint,
			PaymentMethod:   ext.PaymentMethodRaw,
			LlmAccountID:    ext.LlmAccountID,
		})

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
			category = ResolveKnownCategory(tx, *ext.Category)
		}

		if err := tx.Create(&db.Transaction{
			ID:             newTxID,
			SourceType:     "gmail",
			AccountID:      acctID,
			AmountMinor:    amountMinor,
			Currency:       currency,
			OccurredAtUtc:  occurred,
			Merchant:       merchant,
			Category:       category,
			ManualOverride: 0,
			Reviewed:       0,
			CreatedAtUtc:   nowUtc,
			UpdatedAtUtc:   nowUtc,
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

		// Preserve manual tags from the parent transaction on the newly split transaction.
		if err := tx.Exec(`
			INSERT OR IGNORE INTO transaction_tags (transaction_id, tag_id, created_at_utc)
			SELECT ?, tag_id, ? FROM transaction_tags WHERE transaction_id = ?`,
			newTxID, nowUtc, transactionID,
		).Error; err != nil {
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
