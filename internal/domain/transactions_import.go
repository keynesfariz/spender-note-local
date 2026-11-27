package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"portspendernote/internal/db"
)

type ImportManualTransactionRow struct {
	Kind          string   `json:"kind"`
	AmountMinor   int64    `json:"amount_minor"`
	Currency      *string  `json:"currency,omitempty"`
	OccurredAtUtc string   `json:"occurred_at_utc"`
	Merchant      string   `json:"merchant"`
	Category      *string  `json:"category,omitempty"`
	TagNames      []string `json:"tag_names,omitempty"`
	Note          *string  `json:"note,omitempty"`
}

type ImportManualTransactionsInput struct {
	AccountID    string                       `json:"account_id"`
	Transactions []ImportManualTransactionRow `json:"transactions"`
}

type ImportManualTransactionsResult struct {
	ImportedCount int      `json:"importedCount"`
	IDs           []string `json:"ids"`
}

func normalizeImportTimestamp(raw string, loc *time.Location) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("Transaction date is required.")
	}
	if IsValidUtcIsoSeconds(trimmed) {
		return trimmed, nil
	}
	if len(trimmed) == len(time.DateOnly) {
		parsedDate, err := time.ParseInLocation(time.DateOnly, trimmed, loc)
		if err == nil {
			noonLocal := time.Date(parsedDate.Year(), parsedDate.Month(), parsedDate.Day(), 12, 0, 0, 0, loc)
			return ToUtcIsoSeconds(noonLocal), nil
		}
	}
	return "", errors.New("Transaction date must be a valid UTC ISO-8601 timestamp or YYYY-MM-DD date.")
}

func ImportManualTransactions(gdb *gorm.DB, input ImportManualTransactionsInput) (*ImportManualTransactionsResult, error) {
	tzPtr, curPtr, err := getLocaleConfig(gdb)
	if err != nil {
		return nil, err
	}
	if tzPtr == nil || *tzPtr == "" || curPtr == nil || *curPtr == "" {
		return nil, errors.New("Configure your country, timezone, and currency in Settings before importing transactions.")
	}
	loc, err := LoadTimezone(*tzPtr)
	if err != nil {
		return nil, err
	}

	accountID := strings.TrimSpace(input.AccountID)
	if accountID == "" {
		return nil, errors.New("Select an account for this import.")
	}
	if len(input.Transactions) == 0 {
		return nil, errors.New("Select at least one transaction to import.")
	}

	var result ImportManualTransactionsResult
	err = gdb.Transaction(func(tx *gorm.DB) error {
		var account db.Account
		if err := tx.First(&account, "id = ?", accountID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("Selected account does not exist.")
			}
			return err
		}

		var categories []db.Category
		if err := tx.Select("id, normalized_name").Find(&categories).Error; err != nil {
			return err
		}
		categoryIDByNorm := make(map[string]string, len(categories))
		for _, c := range categories {
			categoryIDByNorm[c.NormalizedName] = c.ID
		}
		defaultCategoryID, ok := categoryIDByNorm[strings.ToLower(DefaultCategory)]
		if !ok {
			var err error
			defaultCategoryID, err = validateTransactionCategoryID(tx, DefaultCategory)
			if err != nil {
				return err
			}
		}

		var tags []db.Tag
		if err := tx.Select("id, normalized_name").Find(&tags).Error; err != nil {
			return err
		}
		tagIDByNorm := make(map[string]string, len(tags))
		for _, t := range tags {
			tagIDByNorm[t.NormalizedName] = t.ID
		}

		nowUtc := NowUtcIsoSeconds()
		createdIDs := make([]string, 0, len(input.Transactions))
		txRows := make([]db.Transaction, 0, len(input.Transactions))
		tagRows := make([]db.TransactionTag, 0, len(input.Transactions))

		for i, row := range input.Transactions {
			kind := strings.TrimSpace(row.Kind)
			if kind == "" {
				kind = "expense"
			}
			if kind != "expense" && kind != "income" {
				return fmt.Errorf("Row %d: manual import only supports expense or income transactions.", i+1)
			}
			if row.AmountMinor <= 0 {
				return fmt.Errorf("Row %d: amount must be greater than zero.", i+1)
			}
			occurredAtUtc, err := normalizeImportTimestamp(row.OccurredAtUtc, loc)
			if err != nil {
				return fmt.Errorf("Row %d: %s", i+1, err.Error())
			}
			merchant := strings.TrimSpace(row.Merchant)
			if kind == "income" && merchant == "" {
				merchant = "Income"
			}
			if merchant == "" {
				return fmt.Errorf("Row %d: merchant name is required.", i+1)
			}

			currency := *curPtr
			if row.Currency != nil && strings.TrimSpace(*row.Currency) != "" {
				normalizedCurrency, err := NormalizeCurrencyCode(*row.Currency)
				if err != nil {
					return fmt.Errorf("Row %d: currency must be a 3-letter ISO code.", i+1)
				}
				currency = normalizedCurrency
			} else if kind == "income" && account.Currency != "" {
				currency = account.Currency
			}

			categoryID := defaultCategoryID
			if row.Category != nil && strings.TrimSpace(*row.Category) != "" {
				if matchedID, found := categoryIDByNorm[strings.ToLower(strings.TrimSpace(*row.Category))]; found {
					categoryID = matchedID
				}
			}

			var note *string
			if row.Note != nil {
				note = strPtrOrNil(*row.Note)
			}

			txID := uuid.NewString()
			seenTagID := make(map[string]struct{}, len(row.TagNames))
			for _, rawTag := range row.TagNames {
				norm := strings.ToLower(strings.TrimSpace(rawTag))
				if norm == "" {
					continue
				}
				if tagID, found := tagIDByNorm[norm]; found {
					if _, dup := seenTagID[tagID]; !dup {
						seenTagID[tagID] = struct{}{}
						tagRows = append(tagRows, db.TransactionTag{
							TransactionID: txID,
							TagID:         tagID,
							CreatedAtUtc:  nowUtc,
						})
					}
				}
			}

			acctIDCopy := account.ID
			txRows = append(txRows, db.Transaction{
				ID:             txID,
				SourceType:     "manual",
				Kind:           kind,
				AccountID:      &acctIDCopy,
				AmountMinor:    row.AmountMinor,
				Currency:       currency,
				OccurredAtUtc:  occurredAtUtc,
				Merchant:       merchant,
				CategoryID:     categoryID,
				ManualOverride: 0,
				Reviewed:       0,
				Note:           note,
				CreatedAtUtc:   nowUtc,
				UpdatedAtUtc:   nowUtc,
			})
			createdIDs = append(createdIDs, txID)
		}

		if err := tx.CreateInBatches(&txRows, 100).Error; err != nil {
			return err
		}
		if len(tagRows) > 0 {
			if err := tx.CreateInBatches(&tagRows, 200).Error; err != nil {
				return err
			}
		}

		result = ImportManualTransactionsResult{
			ImportedCount: len(createdIDs),
			IDs:           createdIDs,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}
