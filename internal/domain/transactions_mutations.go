package domain

import (
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"portspendernote/internal/db"
)

type CreateManualTransactionInput struct {
	AmountMinor   int64    `json:"amount_minor"`
	OccurredAtUtc string   `json:"occurred_at_utc"`
	Merchant      string   `json:"merchant"`
	Category      *string  `json:"category"`
	TagIDs        []string `json:"tag_ids"`
	AccountID     *string  `json:"account_id"`
	Note          *string  `json:"note"`
}

func validateTransactionCategory(tx *gorm.DB, rawCategory string) (string, error) {
	cat, err := lookupCategoryByName(tx, rawCategory)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", errors.New("Selected category does not exist.")
		}
		return "", err
	}
	return cat.Name, nil
}

func replaceTransactionTags(tx *gorm.DB, transactionID string, rawTagIDs []string, nowUtc string) error {
	tagIDs := normalizeTransactionIDs(rawTagIDs)
	if len(tagIDs) > 0 {
		var existingCount int64
		if err := tx.Model(&db.Tag{}).Where("id IN ?", tagIDs).Count(&existingCount).Error; err != nil {
			return err
		}
		if int(existingCount) != len(tagIDs) {
			return errors.New("One or more selected tags do not exist.")
		}
	}

	if err := tx.Where("transaction_id = ?", transactionID).Delete(&db.TransactionTag{}).Error; err != nil {
		return err
	}
	for _, tagID := range tagIDs {
		if err := tx.Create(&db.TransactionTag{
			TransactionID: transactionID,
			TagID:         tagID,
			CreatedAtUtc:  nowUtc,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func CreateManualTransaction(gdb *gorm.DB, input CreateManualTransactionInput) (*TransactionListItem, error) {
	_, curPtr, err := getLocaleConfig(gdb)
	if err != nil {
		return nil, err
	}
	if curPtr == nil || *curPtr == "" {
		return nil, errors.New("Configure your country and currency in Settings before adding transactions.")
	}
	if input.AmountMinor <= 0 {
		return nil, errors.New("Amount must be greater than zero.")
	}
	occurred := strings.TrimSpace(input.OccurredAtUtc)
	if !IsValidUtcIsoSeconds(occurred) {
		return nil, errors.New("Transaction date must be a valid UTC ISO-8601 timestamp.")
	}
	merchant := strings.TrimSpace(input.Merchant)
	if merchant == "" {
		return nil, errors.New("Merchant name is required.")
	}
	category := DefaultCategory
	if input.Category != nil && strings.TrimSpace(*input.Category) != "" {
		category = strings.TrimSpace(*input.Category)
	}

	var note *string
	if input.Note != nil && strings.TrimSpace(*input.Note) != "" {
		trimmed := strings.TrimSpace(*input.Note)
		note = &trimmed
	}

	var created TransactionListItem
	err = gdb.Transaction(func(tx *gorm.DB) error {
		canonicalCategory, err := validateTransactionCategory(tx, category)
		if err != nil {
			return err
		}
		var accountID *string
		if input.AccountID != nil && strings.TrimSpace(*input.AccountID) != "" {
			trimmed := strings.TrimSpace(*input.AccountID)
			if err := AssertAccountExists(tx, trimmed, ""); err != nil {
				return err
			}
			accountID = &trimmed
		}

		id := uuid.NewString()
		nowUtc := NowUtcIsoSeconds()
		if err := tx.Create(&db.Transaction{
			ID:             id,
			SourceType:     "manual",
			AccountID:      accountID,
			AmountMinor:    input.AmountMinor,
			Currency:       *curPtr,
			OccurredAtUtc:  occurred,
			Merchant:       merchant,
			Category:       canonicalCategory,
			ManualOverride: 0,
			Reviewed:       1,
			Note:           note,
			CreatedAtUtc:   nowUtc,
			UpdatedAtUtc:   nowUtc,
		}).Error; err != nil {
			return err
		}
		if err := replaceTransactionTags(tx, id, input.TagIDs, nowUtc); err != nil {
			return err
		}
		detail, err := GetTransactionDetail(tx, id)
		if err != nil {
			return err
		}
		created = detail.Transaction
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &created, nil
}

type UpdateTransactionInput struct {
	AmountMinor            *int64   `json:"amount_minor"`
	OccurredAtUtc          *string  `json:"occurred_at_utc"`
	Merchant               *string  `json:"merchant"`
	Category               *string  `json:"category"`
	HasTagIDs              bool     `json:"-"`
	TagIDs                 []string `json:"tag_ids"`
	HasAccountID           bool     `json:"-"`
	AccountID              *string  `json:"account_id"`
	HasNote                bool     `json:"-"`
	Note                   *string  `json:"note"`
	ConvertToLocalCurrency *bool    `json:"convertToLocalCurrency"`
}

func UpdateTransaction(gdb *gorm.DB, id string, patch UpdateTransactionInput) (*TransactionListItem, error) {
	var item TransactionListItem
	err := gdb.Transaction(func(tx *gorm.DB) error {
		_, appCurrency, err := getLocaleConfig(tx)
		if err != nil {
			return err
		}
		var current db.Transaction
		if err := tx.Where("id = ?", id).First(&current).Error; err != nil {
			return errors.New("Transaction not found.")
		}

		amountMinor := current.AmountMinor
		if patch.AmountMinor != nil {
			amountMinor = *patch.AmountMinor
		}
		if amountMinor <= 0 {
			return errors.New("Amount must be greater than zero.")
		}

		occurred := current.OccurredAtUtc
		if patch.OccurredAtUtc != nil {
			occurred = strings.TrimSpace(*patch.OccurredAtUtc)
		}
		if !IsValidUtcIsoSeconds(occurred) {
			return errors.New("Transaction date must be a valid UTC ISO-8601 timestamp.")
		}

		merchant := current.Merchant
		if patch.Merchant != nil {
			merchant = strings.TrimSpace(*patch.Merchant)
		}
		if merchant == "" {
			return errors.New("Merchant name is required.")
		}

		category := current.Category
		if patch.Category != nil {
			rawCat := strings.TrimSpace(*patch.Category)
			if rawCat == "" {
				return errors.New("Category is required.")
			}
			canonicalCat, err := validateTransactionCategory(tx, rawCat)
			if err != nil {
				return err
			}
			category = canonicalCat
		}

		accountID := current.AccountID
		if patch.HasAccountID {
			if patch.AccountID != nil && strings.TrimSpace(*patch.AccountID) != "" {
				trimmed := strings.TrimSpace(*patch.AccountID)
				if err := AssertAccountExists(tx, trimmed, ""); err != nil {
					return err
				}
				accountID = &trimmed
			} else {
				accountID = nil
			}
		}

		note := current.Note
		if patch.HasNote {
			if patch.Note != nil && strings.TrimSpace(*patch.Note) != "" {
				trimmed := strings.TrimSpace(*patch.Note)
				note = &trimmed
			} else {
				note = nil
			}
		}

		shouldConvert := appCurrency != nil && *appCurrency != "" && current.Currency != *appCurrency &&
			((patch.ConvertToLocalCurrency != nil && *patch.ConvertToLocalCurrency) || patch.AmountMinor != nil)
		nextCurrency := current.Currency
		if shouldConvert {
			nextCurrency = *appCurrency
		}

		manualOverride := current.ManualOverride
		if current.SourceType == "gmail" {
			manualOverride = 1
		}

		nowUtc := NowUtcIsoSeconds()
		if err := tx.Model(&db.Transaction{}).Where("id = ?", id).Updates(map[string]any{
			"account_id":      accountID,
			"amount_minor":    amountMinor,
			"currency":        nextCurrency,
			"occurred_at_utc": occurred,
			"merchant":        merchant,
			"category":        category,
			"manual_override": manualOverride,
			"reviewed":        1,
			"note":            note,
			"updated_at_utc":  nowUtc,
		}).Error; err != nil {
			return err
		}

		if patch.HasTagIDs {
			if err := replaceTransactionTags(tx, id, patch.TagIDs, nowUtc); err != nil {
				return err
			}
		}

		detail, err := GetTransactionDetail(tx, id)
		if err != nil {
			return err
		}
		item = detail.Transaction
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func normalizeTransactionIDs(rawIDs []string) []string {
	seen := make(map[string]struct{}, len(rawIDs))
	ids := make([]string, 0, len(rawIDs))
	for _, raw := range rawIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return ids
}

func dismissEmailsAndDeleteTransactions(tx *gorm.DB, ids []string) (int64, error) {
	emailSubquery := tx.Model(&db.TransactionSource{}).Select("email_id").Where("transaction_id IN ?", ids)
	if err := tx.Model(&db.RawEmail{}).Where("id IN (?)", emailSubquery).Update("status", "dismissed").Error; err != nil {
		return 0, err
	}
	res := tx.Where("id IN ?", ids).Delete(&db.Transaction{})
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

func DeleteTransaction(gdb *gorm.DB, id string) error {
	return gdb.Transaction(func(tx *gorm.DB) error {
		deleted, err := dismissEmailsAndDeleteTransactions(tx, []string{id})
		if err != nil {
			return err
		}
		if deleted == 0 {
			return errors.New("Transaction not found.")
		}
		return nil
	})
}

func DeleteTransactions(gdb *gorm.DB, transactionIDs []string) (int64, error) {
	ids := normalizeTransactionIDs(transactionIDs)
	if len(ids) == 0 {
		return 0, errors.New("Select at least one transaction to delete.")
	}

	var deletedCount int64
	err := gdb.Transaction(func(tx *gorm.DB) error {
		deleted, err := dismissEmailsAndDeleteTransactions(tx, ids)
		if err != nil {
			return err
		}
		if int(deleted) != len(ids) {
			return errors.New("Cannot delete: one or more selected transactions no longer exist.")
		}
		deletedCount = deleted
		return nil
	})
	if err != nil {
		return 0, err
	}
	return deletedCount, nil
}

func nullableStringEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func MarkTransactionsReviewed(gdb *gorm.DB, transactionIDs []string) (int64, error) {
	ids := normalizeTransactionIDs(transactionIDs)
	if len(ids) == 0 {
		return 0, errors.New("Select at least one transaction to mark reviewed.")
	}

	res := gdb.Model(&db.Transaction{}).Where("id IN ?", ids).Updates(map[string]any{
		"reviewed":       1,
		"updated_at_utc": NowUtcIsoSeconds(),
	})
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

func DuplicateMergeTransactions(gdb *gorm.DB, keepID, dropID string) (*TransactionListItem, error) {
	keepID = strings.TrimSpace(keepID)
	dropID = strings.TrimSpace(dropID)
	if keepID == "" || dropID == "" || keepID == dropID {
		return nil, errors.New("Duplicate merge requires exactly 2 distinct transactions.")
	}

	var result TransactionListItem
	err := gdb.Transaction(func(tx *gorm.DB) error {
		var keepTx, dropTx db.Transaction
		if err := tx.Where("id = ?", keepID).First(&keepTx).Error; err != nil {
			return errors.New("Cannot merge: one or both transactions no longer exist.")
		}
		if err := tx.Where("id = ?", dropID).First(&dropTx).Error; err != nil {
			return errors.New("Cannot merge: one or both transactions no longer exist.")
		}
		if keepTx.Currency != dropTx.Currency || !nullableStringEqual(keepTx.AccountID, dropTx.AccountID) {
			return errors.New("Duplicate merge requires transactions with the same currency and account.")
		}
		if keepTx.AmountMinor != dropTx.AmountMinor {
			return errors.New("Duplicate merge requires transactions with equal amounts.")
		}

		// Re-point transaction_sources before deleting the absorbed transaction so
		// ON DELETE CASCADE does not drop linked emails and raw_emails stay extracted.
		if err := tx.Model(&db.TransactionSource{}).Where("transaction_id = ?", dropID).Updates(map[string]any{
			"transaction_id": keepID,
			"match_state":    "confirmed",
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&db.TransactionSource{}).Where("transaction_id = ?", keepID).Update("match_state", "confirmed").Error; err != nil {
			return err
		}

		// Preserve deduplicated tag union on the kept transaction before dropping the duplicate.
		nowUtc := NowUtcIsoSeconds()
		if err := tx.Exec(`
			INSERT OR IGNORE INTO transaction_tags (transaction_id, tag_id, created_at_utc)
			SELECT ?, tag_id, ? FROM transaction_tags WHERE transaction_id = ?`,
			keepID, nowUtc, dropID,
		).Error; err != nil {
			return err
		}

		if err := tx.Where("id = ?", dropID).Delete(&db.Transaction{}).Error; err != nil {
			return err
		}

		if err := tx.Model(&db.Transaction{}).Where("id = ?", keepID).Updates(map[string]any{
			"reviewed":       1,
			"updated_at_utc": nowUtc,
		}).Error; err != nil {
			return err
		}

		detail, err := GetTransactionDetail(tx, keepID)
		if err != nil {
			return err
		}
		result = detail.Transaction
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

type CombineTransactionsInput struct {
	TransactionIDs []string `json:"transactionIds"`
	Merchant       string   `json:"merchant"`
	Category       *string  `json:"category"`
	HasAccountID   bool     `json:"-"`
	AccountID      *string  `json:"account_id"`
	OccurredAtUtc  string   `json:"occurred_at_utc"`
	HasNote        bool     `json:"-"`
	Note           *string  `json:"note"`
}

func CombineTransactions(gdb *gorm.DB, input CombineTransactionsInput) (*TransactionListItem, error) {
	ids := normalizeTransactionIDs(input.TransactionIDs)
	if len(ids) < 2 {
		return nil, errors.New("Combine transactions requires at least 2 distinct transactions.")
	}

	merchant := strings.TrimSpace(input.Merchant)
	if merchant == "" {
		return nil, errors.New("Merchant name is required.")
	}
	occurred := strings.TrimSpace(input.OccurredAtUtc)
	if !IsValidUtcIsoSeconds(occurred) {
		return nil, errors.New("Transaction date must be a valid UTC ISO-8601 timestamp.")
	}
	category := DefaultCategory
	if input.Category != nil && strings.TrimSpace(*input.Category) != "" {
		category = strings.TrimSpace(*input.Category)
	}

	var result TransactionListItem
	err := gdb.Transaction(func(tx *gorm.DB) error {
		var rows []db.Transaction
		if err := tx.Where("id IN ?", ids).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(ids) {
			return errors.New("Cannot combine: one or more selected transactions no longer exist.")
		}

		baseCurrency := rows[0].Currency
		baseAccountID := rows[0].AccountID
		var totalAmount int64
		for _, r := range rows {
			if r.Currency != baseCurrency || !nullableStringEqual(r.AccountID, baseAccountID) {
				return errors.New("Combine transactions requires all selected transactions to share the same currency and account.")
			}
			totalAmount += r.AmountMinor
		}

		accountID := baseAccountID
		if input.HasAccountID {
			if input.AccountID != nil && strings.TrimSpace(*input.AccountID) != "" {
				trimmed := strings.TrimSpace(*input.AccountID)
				if err := AssertAccountExists(tx, trimmed, ""); err != nil {
					return err
				}
				accountID = &trimmed
			} else {
				accountID = nil
			}
		}

		var note *string
		if input.HasNote {
			if input.Note != nil && strings.TrimSpace(*input.Note) != "" {
				trimmed := strings.TrimSpace(*input.Note)
				note = &trimmed
			}
		}

		canonicalCategory, err := validateTransactionCategory(tx, category)
		if err != nil {
			return err
		}

		survivorID := ids[0]
		absorbedIDs := ids[1:]

		// Preserve deduplicated tag union across all combined transactions before deleting absorbed rows.
		nowUtc := NowUtcIsoSeconds()
		if err := tx.Exec(`
			INSERT OR IGNORE INTO transaction_tags (transaction_id, tag_id, created_at_utc)
			SELECT ?, tag_id, ? FROM transaction_tags WHERE transaction_id IN ?`,
			survivorID, nowUtc, absorbedIDs,
		).Error; err != nil {
			return err
		}

		// Re-point transaction_sources from absorbed transactions before deletion.
		if err := tx.Model(&db.TransactionSource{}).Where("transaction_id IN ?", absorbedIDs).Updates(map[string]any{
			"transaction_id": survivorID,
			"match_state":    "confirmed",
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&db.TransactionSource{}).Where("transaction_id = ?", survivorID).Update("match_state", "confirmed").Error; err != nil {
			return err
		}

		if err := tx.Where("id IN ?", absorbedIDs).Delete(&db.Transaction{}).Error; err != nil {
			return err
		}

		if err := tx.Model(&db.Transaction{}).Where("id = ?", survivorID).Updates(map[string]any{
			"account_id":      accountID,
			"amount_minor":    totalAmount,
			"occurred_at_utc": occurred,
			"merchant":        merchant,
			"category":        canonicalCategory,
			"manual_override": 1,
			"reviewed":        1,
			"note":            note,
			"updated_at_utc":  nowUtc,
		}).Error; err != nil {
			return err
		}

		detail, err := GetTransactionDetail(tx, survivorID)
		if err != nil {
			return err
		}
		result = detail.Transaction
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}
