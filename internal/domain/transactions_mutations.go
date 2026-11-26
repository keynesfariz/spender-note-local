package domain

import (
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"portspendernote/internal/db"
)

type CreateManualTransactionInput struct {
	Kind          string   `json:"kind"`
	AmountMinor   int64    `json:"amount_minor"`
	OccurredAtUtc string   `json:"occurred_at_utc"`
	Merchant      string   `json:"merchant"`
	Category      *string  `json:"category"`
	TagIDs        []string `json:"tag_ids"`
	AccountID     *string  `json:"account_id"`
	Note          *string  `json:"note"`
}

func validateTransactionCategoryID(tx *gorm.DB, rawCategory string) (string, error) {
	cat, err := lookupCategoryByName(tx, rawCategory)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", errors.New("Selected category does not exist.")
		}
		return "", err
	}
	return cat.ID, nil
}

func validateSelectedTagIDs(tx *gorm.DB, tagIDs []string) error {
	if len(tagIDs) == 0 {
		return nil
	}
	var existingCount int64
	if err := tx.Model(&db.Tag{}).Where("id IN ?", tagIDs).Count(&existingCount).Error; err != nil {
		return err
	}
	if int(existingCount) != len(tagIDs) {
		return errors.New("One or more selected tags do not exist.")
	}
	return nil
}

func setTransactionTags(tx *gorm.DB, transactionID string, tagIDs []string, nowUtc string) error {
	if err := tx.Where("transaction_id = ?", transactionID).Delete(&db.TransactionTag{}).Error; err != nil {
		return err
	}
	if len(tagIDs) == 0 {
		return nil
	}
	rows := make([]db.TransactionTag, len(tagIDs))
	for i, tagID := range tagIDs {
		rows[i] = db.TransactionTag{
			TransactionID: transactionID,
			TagID:         tagID,
			CreatedAtUtc:  nowUtc,
		}
	}
	return tx.Create(&rows).Error
}

func replaceTransactionTags(tx *gorm.DB, transactionID string, rawTagIDs []string, nowUtc string) error {
	tagIDs := normalizeTransactionIDs(rawTagIDs)
	if err := validateSelectedTagIDs(tx, tagIDs); err != nil {
		return err
	}
	return setTransactionTags(tx, transactionID, tagIDs, nowUtc)
}

func resolveAndReplaceTransactionTags(tx *gorm.DB, transactionID string, rawTagIDs []string, rawTagNames []string, nowUtc string) error {
	callerTagIDs := normalizeTransactionIDs(rawTagIDs)

	pending := make([]struct {
		name string
		norm string
	}, 0, len(rawTagNames))
	norms := make([]string, 0, len(rawTagNames))
	seenLower := make(map[string]struct{}, len(rawTagNames))
	for _, rawName := range rawTagNames {
		name := strings.TrimSpace(rawName)
		if name == "" {
			continue
		}
		norm := strings.ToLower(name)
		if _, seen := seenLower[norm]; seen {
			continue
		}
		seenLower[norm] = struct{}{}
		pending = append(pending, struct {
			name string
			norm string
		}{name: name, norm: norm})
		norms = append(norms, norm)
	}

	allTagIDs := make([]string, 0, len(callerTagIDs)+len(pending))
	allTagIDs = append(allTagIDs, callerTagIDs...)

	if len(norms) > 0 {
		var existingTags []db.Tag
		if err := tx.Select("id, normalized_name").Where("normalized_name IN ?", norms).Find(&existingTags).Error; err != nil {
			return err
		}
		idByNorm := make(map[string]string, len(pending))
		for _, t := range existingTags {
			idByNorm[t.NormalizedName] = t.ID
		}

		newTags := make([]db.Tag, 0, len(pending))
		for _, item := range pending {
			if existingID, ok := idByNorm[item.norm]; ok {
				allTagIDs = append(allTagIDs, existingID)
				continue
			}
			createdID := uuid.NewString()
			idByNorm[item.norm] = createdID
			newTags = append(newTags, db.Tag{
				ID:             createdID,
				Name:           item.name,
				NormalizedName: item.norm,
				CreatedAtUtc:   nowUtc,
			})
			allTagIDs = append(allTagIDs, createdID)
		}
		if len(newTags) > 0 {
			if err := tx.Create(&newTags).Error; err != nil {
				return err
			}
		}
	}

	if err := validateSelectedTagIDs(tx, callerTagIDs); err != nil {
		return err
	}
	return setTransactionTags(tx, transactionID, normalizeTransactionIDs(allTagIDs), nowUtc)
}

func appendTransactionTagNames(tx *gorm.DB, transactionID string, names []string, nowUtc string) error {
	var currentIDs []string
	if err := tx.Model(&db.TransactionTag{}).Where("transaction_id = ?", transactionID).Pluck("tag_id", &currentIDs).Error; err != nil {
		return err
	}
	return resolveAndReplaceTransactionTags(tx, transactionID, currentIDs, names, nowUtc)
}

func copyTransactionTags(tx *gorm.DB, targetTxID string, sourceTxIDs []string, nowUtc string) error {
	if len(sourceTxIDs) == 0 {
		return nil
	}
	return tx.Exec(`
		INSERT OR IGNORE INTO transaction_tags (transaction_id, tag_id, created_at_utc)
		SELECT ?, tag_id, ? FROM transaction_tags WHERE transaction_id IN ?`,
		targetTxID, nowUtc, sourceTxIDs,
	).Error
}

func CreateManualTransaction(gdb *gorm.DB, input CreateManualTransactionInput) (*TransactionListItem, error) {
	_, curPtr, err := getLocaleConfig(gdb)
	if err != nil {
		return nil, err
	}
	if curPtr == nil || *curPtr == "" {
		return nil, errors.New("Configure your country and currency in Settings before adding transactions.")
	}
	kind := strings.TrimSpace(input.Kind)
	if kind == "" {
		kind = "expense"
	}
	if kind != "expense" && kind != "income" {
		return nil, errors.New("Manual transactions must be an expense or income.")
	}
	if input.AmountMinor <= 0 {
		return nil, errors.New("Amount must be greater than zero.")
	}
	occurred := strings.TrimSpace(input.OccurredAtUtc)
	if !IsValidUtcIsoSeconds(occurred) {
		return nil, errors.New("Transaction date must be a valid UTC ISO-8601 timestamp.")
	}
	merchant := strings.TrimSpace(input.Merchant)
	if kind == "income" && merchant == "" {
		merchant = "Income"
	}
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
		categoryID, err := validateTransactionCategoryID(tx, category)
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

		currency := *curPtr
		if kind == "income" {
			if accountID == nil {
				return errors.New("Select a receiving account for income.")
			}
			var account db.Account
			if err := tx.Select("currency").First(&account, "id = ?", *accountID).Error; err != nil {
				return err
			}
			currency = account.Currency
		}
		id := uuid.NewString()
		nowUtc := NowUtcIsoSeconds()
		if err := tx.Create(&db.Transaction{
			ID:             id,
			SourceType:     "manual",
			Kind:           kind,
			AccountID:      accountID,
			AmountMinor:    input.AmountMinor,
			Currency:       currency,
			OccurredAtUtc:  occurred,
			Merchant:       merchant,
			CategoryID:     categoryID,
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
	Remark                 *string  `json:"remark"`
	RemarkUserEdited       *bool    `json:"remark_user_edited"`
	AmountMinor            *int64   `json:"amount_minor"`
	OccurredAtUtc          *string  `json:"occurred_at_utc"`
	Merchant               *string  `json:"merchant"`
	Category               *string  `json:"category"`
	HasTagIDs              bool     `json:"-"`
	TagIDs                 []string `json:"tag_ids"`
	TagNames               []string `json:"tag_names"`
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

		if current.Kind == "transfer" {
			return errors.New("Open the transfer editor to change this transfer.")
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
		if current.Kind == "income" && merchant == "" {
			merchant = "Income"
		}
		if merchant == "" {
			return errors.New("Merchant name is required.")
		}

		categoryID := current.CategoryID
		if patch.Category != nil {
			rawCat := strings.TrimSpace(*patch.Category)
			if rawCat == "" {
				return errors.New("Category is required.")
			}
			selectedID, err := validateTransactionCategoryID(tx, rawCat)
			if err != nil {
				return err
			}
			categoryID = selectedID
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

		convertToLocal := patch.AmountMinor != nil
		if patch.ConvertToLocalCurrency != nil {
			convertToLocal = *patch.ConvertToLocalCurrency
		}
		nextCurrency := current.Currency
		if convertToLocal && appCurrency != nil && *appCurrency != "" {
			nextCurrency = *appCurrency
		}

		if current.Kind == "income" {
			if accountID == nil {
				return errors.New("Select a receiving account for income.")
			}
			var account db.Account
			if err := tx.Select("currency").First(&account, "id = ?", *accountID).Error; err != nil {
				return err
			}
			if account.Currency != current.Currency && patch.AmountMinor == nil {
				return errors.New("Enter an amount in the receiving account's currency.")
			}
			nextCurrency = account.Currency
		}
		remark, remarkUserEdited := applyTransactionRemark(current, merchant, patch.Remark, patch.RemarkUserEdited)
		manualOverride := current.ManualOverride
		if current.SourceType == "gmail" {
			manualOverride = 1
		}

		nowUtc := NowUtcIsoSeconds()
		if err := tx.Model(&db.Transaction{}).Where("id = ?", id).Updates(map[string]any{
			"account_id":         accountID,
			"amount_minor":       amountMinor,
			"currency":           nextCurrency,
			"occurred_at_utc":    occurred,
			"merchant":           merchant,
			"remark":             remark,
			"remark_user_edited": remarkUserEdited,
			"category_id":        categoryID,
			"manual_override":    manualOverride,
			"reviewed":           1,
			"note":               note,
			"updated_at_utc":     nowUtc,
		}).Error; err != nil {
			return err
		}

		if patch.HasTagIDs {
			if err := resolveAndReplaceTransactionTags(tx, id, patch.TagIDs, patch.TagNames, nowUtc); err != nil {
				return err
			}
		} else if len(patch.TagNames) > 0 {
			if err := appendTransactionTagNames(tx, id, patch.TagNames, nowUtc); err != nil {
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
		if keepTx.Kind != "expense" || dropTx.Kind != "expense" {
			return errors.New("Only expenses can be merged as duplicates.")
		}
		if keepTx.Currency != dropTx.Currency || !nullableStringEqual(keepTx.AccountID, dropTx.AccountID) {
			return errors.New("Duplicate merge requires transactions with the same currency and account.")
		}
		if keepTx.AmountMinor != dropTx.AmountMinor {
			return errors.New("Duplicate merge requires transactions with equal amounts.")
		}

		if err := tx.Model(&db.TransactionSource{}).Where("transaction_id = ?", dropID).Updates(map[string]any{
			"transaction_id": keepID,
			"match_state":    "confirmed",
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&db.TransactionSource{}).Where("transaction_id = ?", keepID).Update("match_state", "confirmed").Error; err != nil {
			return err
		}

		nowUtc := NowUtcIsoSeconds()
		if err := copyTransactionTags(tx, keepID, []string{dropID}, nowUtc); err != nil {
			return err
		}

		if err := tx.Where("id = ?", dropID).Delete(&db.Transaction{}).Error; err != nil {
			return err
		}

		if err := tx.Model(&db.Transaction{}).Where("id = ?", keepID).Updates(map[string]any{
			"ambiguous_source_amount": keepTx.AmbiguousSourceAmount || dropTx.AmbiguousSourceAmount,
			"reviewed":                1,
			"updated_at_utc":          nowUtc,
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
	Remark           *string  `json:"remark"`
	RemarkUserEdited *bool    `json:"remark_user_edited"`
	TransactionIDs   []string `json:"transactionIds"`
	Merchant         string   `json:"merchant"`
	Category         *string  `json:"category"`
	HasTagIDs        bool     `json:"-"`
	TagIDs           []string `json:"tag_ids"`
	TagNames         []string `json:"tag_names"`
	HasAccountID     bool     `json:"-"`
	AccountID        *string  `json:"account_id"`
	OccurredAtUtc    string   `json:"occurred_at_utc"`
	HasNote          bool     `json:"-"`
	Note             *string  `json:"note"`
}

// applyTransactionRemark keeps provenance on an unchanged title and clears it
// when the user returns to the merchant fallback.
func applyTransactionRemark(current db.Transaction, nextMerchant string, input *string, userEdited *bool) (*string, bool) {
	if input == nil {
		return current.Remark, current.RemarkUserEdited
	}
	remark := strings.TrimSpace(*input)
	if remark == "" {
		return nil, false
	}
	if current.Remark != nil && remark == *current.Remark {
		return current.Remark, current.RemarkUserEdited
	}
	if current.Remark == nil && remark == strings.TrimSpace(nextMerchant) {
		return nil, false
	}
	return &remark, userEdited == nil || *userEdited
}

func CombineTransactions(gdb *gorm.DB, input CombineTransactionsInput) (*TransactionListItem, error) {
	ids := normalizeTransactionIDs(input.TransactionIDs)
	if len(ids) < 2 {
		return nil, errors.New("Combine transactions requires at least 2 distinct transactions.")
	}

	merchant := strings.TrimSpace(input.Merchant)
	if merchant == "" && input.Remark == nil {
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
		for _, row := range rows {
			if row.Kind != "expense" {
				return errors.New("Only expenses can be combined.")
			}
		}
		if len(rows) != len(ids) {
			return errors.New("Cannot combine: one or more selected transactions no longer exist.")
		}

		survivorID := ids[0]
		baseCurrency := rows[0].Currency
		baseAccountID := rows[0].AccountID
		var survivor db.Transaction
		var totalAmount int64
		for _, row := range rows {
			if row.Currency != baseCurrency || !nullableStringEqual(row.AccountID, baseAccountID) {
				return errors.New("Combine transactions requires all selected transactions to share the same currency and account.")
			}
			if row.ID == survivorID {
				survivor = row
			}
			totalAmount += row.AmountMinor
		}

		if merchant == "" {
			merchant = survivor.Merchant
		}
		remark, remarkUserEdited := applyTransactionRemark(survivor, merchant, input.Remark, input.RemarkUserEdited)

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

		categoryID, err := validateTransactionCategoryID(tx, category)
		if err != nil {
			return err
		}

		absorbedIDs := ids[1:]

		nowUtc := NowUtcIsoSeconds()
		if input.HasTagIDs {
			if err := resolveAndReplaceTransactionTags(tx, survivorID, input.TagIDs, input.TagNames, nowUtc); err != nil {
				return err
			}
		} else {
			if err := copyTransactionTags(tx, survivorID, absorbedIDs, nowUtc); err != nil {
				return err
			}
			if len(input.TagNames) > 0 {
				if err := appendTransactionTagNames(tx, survivorID, input.TagNames, nowUtc); err != nil {
					return err
				}
			}
		}

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
			"account_id":              accountID,
			"amount_minor":            totalAmount,
			"occurred_at_utc":         occurred,
			"merchant":                merchant,
			"remark":                  remark,
			"remark_user_edited":      remarkUserEdited,
			"category_id":             categoryID,
			"manual_override":         1,
			"ambiguous_source_amount": true,
			"reviewed":                1,
			"note":                    note,
			"updated_at_utc":          nowUtc,
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
