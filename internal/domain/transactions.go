package domain

import (
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"portspendernote/internal/db"
)

const paymentMethodSubquery = `(
	SELECT ee.payment_method_raw
	FROM transaction_sources ts
	JOIN email_extractions ee ON ee.email_id = ts.email_id
	WHERE ts.transaction_id = t.id
	  AND ee.payment_method_raw IS NOT NULL
	ORDER BY CASE WHEN ts.role = 'receipt' THEN 0 ELSE 1 END, ts.rowid ASC
	LIMIT 1
)`

type SpendingGroupRow struct {
	Key        string `gorm:"column:key" json:"key"`
	Label      string `gorm:"column:label" json:"label"`
	TotalMinor int64  `gorm:"column:total_minor" json:"total_minor"`
	Count      int64  `gorm:"column:count" json:"count"`
}

type MonthlySpendingSummary struct {
	Configured           bool               `json:"configured"`
	Month                *string            `json:"month"`
	Timezone             *string            `json:"timezone"`
	Currency             *string            `json:"currency"`
	StartUtc             *string            `json:"startUtc"`
	EndUtc               *string            `json:"endUtc"`
	TotalSpentMinor      int64              `json:"totalSpentMinor"`
	TransactionCount     int64              `json:"transactionCount"`
	ForeignCurrencyCount int64              `json:"foreignCurrencyCount"`
	ByCategory           []SpendingGroupRow `json:"byCategory"`
	ByMerchant           []SpendingGroupRow `json:"byMerchant"`
	ByAccount            []SpendingGroupRow `json:"byAccount"`
}

type TransactionListItem struct {
	ID                string   `json:"id"`
	SourceType        string   `json:"source_type"`
	AccountID         *string  `json:"account_id"`
	AccountName       *string  `json:"account_name"`
	PaymentMethodRaw  *string  `json:"payment_method_raw"`
	AmountMinor       int64    `json:"amount_minor"`
	Currency          string   `json:"currency"`
	OccurredAtUtc     string   `json:"occurred_at_utc"`
	Merchant          string   `json:"merchant"`
	Category          string   `json:"category"`
	ManualOverride    int      `json:"manual_override"`
	Reviewed          int      `json:"reviewed"`
	Note              *string  `json:"note"`
	CreatedAtUtc      string   `json:"created_at_utc"`
	UpdatedAtUtc      string   `json:"updated_at_utc"`
	SourceCount       int      `json:"sourceCount"`
	SourceRoles       []string `json:"sourceRoles"`
	IsForeignCurrency bool     `json:"isForeignCurrency"`
}

type TransactionSourceView struct {
	ID              string  `gorm:"column:id" json:"id"`
	EmailID         string  `gorm:"column:email_id" json:"email_id"`
	Role            string  `gorm:"column:role" json:"role"`
	MatchScore      float64 `gorm:"column:match_score" json:"match_score"`
	MatchState      string  `gorm:"column:match_state" json:"match_state"`
	FromAddress     string  `gorm:"column:from_address" json:"from_address"`
	Subject         string  `gorm:"column:subject" json:"subject"`
	CleanedText     string  `gorm:"column:cleaned_text" json:"cleaned_text"`
	ReceivedAtUtc   string  `gorm:"column:received_at_utc" json:"received_at_utc"`
	ExtractionModel *string `gorm:"column:extraction_model" json:"extraction_model"`
	RawLlmJSON      *string `gorm:"column:raw_llm_json" json:"raw_llm_json"`
}

type TransactionDetailView struct {
	Transaction TransactionListItem     `json:"transaction"`
	Sources     []TransactionSourceView `json:"sources"`
}

func getLocaleConfig(gdb *gorm.DB) (tz *string, cur *string) {
	var s db.AppSetting
	if err := gdb.Select("timezone, currency").First(&s, 1).Error; err == nil {
		return s.Timezone, s.Currency
	}
	return nil, nil
}

func GetMonthlySpendingSummary(gdb *gorm.DB, monthParam string) (*MonthlySpendingSummary, error) {
	tzPtr, curPtr := getLocaleConfig(gdb)
	if tzPtr == nil || *tzPtr == "" || curPtr == nil || *curPtr == "" {
		return &MonthlySpendingSummary{
			Configured: false,
			Timezone:   tzPtr,
			Currency:   curPtr,
			ByCategory: []SpendingGroupRow{},
			ByMerchant: []SpendingGroupRow{},
			ByAccount:  []SpendingGroupRow{},
		}, nil
	}
	tz := *tzPtr
	currency := *curPtr

	month := strings.TrimSpace(monthParam)
	if month == "" {
		month = GetCurrentMonthInTimezone(tz)
	}
	startUtc, endUtc, err := GetMonthUtcBounds(month, tz)
	if err != nil {
		return nil, err
	}

	type totalsRow struct {
		LocalTotal   int64 `gorm:"column:local_total"`
		LocalCount   int64 `gorm:"column:local_count"`
		ForeignCount int64 `gorm:"column:foreign_count"`
	}
	var totals totalsRow
	gdb.Raw(`
		SELECT
			COALESCE(SUM(CASE WHEN currency = ? THEN amount_minor ELSE 0 END), 0) AS local_total,
			COALESCE(SUM(CASE WHEN currency = ? THEN 1 ELSE 0 END), 0) AS local_count,
			COALESCE(SUM(CASE WHEN currency != ? THEN 1 ELSE 0 END), 0) AS foreign_count
		FROM transactions
		WHERE occurred_at_utc >= ? AND occurred_at_utc < ?`,
		currency, currency, currency, startUtc, endUtc,
	).Scan(&totals)

	byCategory := []SpendingGroupRow{}
	gdb.Raw(`
		SELECT category AS key, category AS label, SUM(amount_minor) AS total_minor, COUNT(*) AS count
		FROM transactions
		WHERE currency = ? AND occurred_at_utc >= ? AND occurred_at_utc < ?
		GROUP BY category
		ORDER BY total_minor DESC, label ASC`,
		currency, startUtc, endUtc,
	).Scan(&byCategory)

	byMerchant := []SpendingGroupRow{}
	gdb.Raw(`
		SELECT LOWER(TRIM(merchant)) AS key, MIN(merchant) AS label, SUM(amount_minor) AS total_minor, COUNT(*) AS count
		FROM transactions
		WHERE currency = ? AND occurred_at_utc >= ? AND occurred_at_utc < ?
		GROUP BY LOWER(TRIM(merchant))
		ORDER BY total_minor DESC, label ASC
		LIMIT 10`,
		currency, startUtc, endUtc,
	).Scan(&byMerchant)

	byAccount := []SpendingGroupRow{}
	gdb.Raw(`
		SELECT
			COALESCE(t.account_id, 'unassigned') AS key,
			COALESCE(a.name, 'Unassigned') AS label,
			SUM(t.amount_minor) AS total_minor,
			COUNT(*) AS count
		FROM transactions t
		LEFT JOIN accounts a ON a.id = t.account_id
		WHERE t.currency = ? AND t.occurred_at_utc >= ? AND t.occurred_at_utc < ?
		GROUP BY COALESCE(t.account_id, 'unassigned'), COALESCE(a.name, 'Unassigned')
		ORDER BY total_minor DESC, label ASC`,
		currency, startUtc, endUtc,
	).Scan(&byAccount)

	return &MonthlySpendingSummary{
		Configured:           true,
		Month:                &month,
		Timezone:             &tz,
		Currency:             &currency,
		StartUtc:             &startUtc,
		EndUtc:               &endUtc,
		TotalSpentMinor:      totals.LocalTotal,
		TransactionCount:     totals.LocalCount + totals.ForeignCount,
		ForeignCurrencyCount: totals.ForeignCount,
		ByCategory:           byCategory,
		ByMerchant:           byMerchant,
		ByAccount:            byAccount,
	}, nil
}

type ListTransactionsParams struct {
	Month        string
	Category     string
	AccountID    string
	Search       string
	UnmergedOnly bool
}

func ListTransactions(gdb *gorm.DB, params ListTransactionsParams) ([]TransactionListItem, error) {
	tzPtr, curPtr := getLocaleConfig(gdb)
	if tzPtr == nil || *tzPtr == "" || curPtr == nil || *curPtr == "" {
		return []TransactionListItem{}, nil
	}
	tz := *tzPtr
	currency := *curPtr

	var conds []string
	var args []any

	if strings.TrimSpace(params.Month) != "" {
		startUtc, endUtc, err := GetMonthUtcBounds(params.Month, tz)
		if err != nil {
			return nil, err
		}
		conds = append(conds, "t.occurred_at_utc >= ? AND t.occurred_at_utc < ?")
		args = append(args, startUtc, endUtc)
	}
	if params.Category != "" && params.Category != "all" {
		conds = append(conds, "t.category = ?")
		args = append(args, params.Category)
	}
	if params.AccountID != "" && params.AccountID != "all" {
		if params.AccountID == "unassigned" {
			conds = append(conds, "t.account_id IS NULL")
		} else {
			conds = append(conds, "t.account_id = ?")
			args = append(args, params.AccountID)
		}
	}
	if q := strings.TrimSpace(params.Search); q != "" {
		like := "%" + strings.ToLower(q) + "%"
		conds = append(conds, "(LOWER(t.merchant) LIKE ? OR LOWER(COALESCE(t.note, '')) LIKE ?)")
		args = append(args, like, like)
	}

	whereClause := ""
	if len(conds) > 0 {
		whereClause = "WHERE " + strings.Join(conds, " AND ")
	}

	type rawTxRow struct {
		ID               string  `gorm:"column:id"`
		SourceType       string  `gorm:"column:source_type"`
		AccountID        *string `gorm:"column:account_id"`
		AccountName      *string `gorm:"column:account_name"`
		PaymentMethodRaw *string `gorm:"column:payment_method_raw"`
		AmountMinor      int64   `gorm:"column:amount_minor"`
		Currency         string  `gorm:"column:currency"`
		OccurredAtUtc    string  `gorm:"column:occurred_at_utc"`
		Merchant         string  `gorm:"column:merchant"`
		Category         string  `gorm:"column:category"`
		ManualOverride   int     `gorm:"column:manual_override"`
		Reviewed         int     `gorm:"column:reviewed"`
		Note             *string `gorm:"column:note"`
		CreatedAtUtc     string  `gorm:"column:created_at_utc"`
		UpdatedAtUtc     string  `gorm:"column:updated_at_utc"`
		RolesCSV         *string `gorm:"column:roles_csv"`
	}

	var rows []rawTxRow
	query := `
		SELECT
			t.*,
			a.name AS account_name,
			` + paymentMethodSubquery + ` AS payment_method_raw,
			(SELECT GROUP_CONCAT(ts.role) FROM transaction_sources ts WHERE ts.transaction_id = t.id) AS roles_csv
		FROM transactions t
		LEFT JOIN accounts a ON a.id = t.account_id
		` + whereClause + `
		ORDER BY t.occurred_at_utc DESC, t.created_at_utc DESC`

	if err := gdb.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]TransactionListItem, 0, len(rows))
	for _, r := range rows {
		roles := []string{}
		if r.RolesCSV != nil && *r.RolesCSV != "" {
			roles = strings.Split(*r.RolesCSV, ",")
		}
		item := TransactionListItem{
			ID:                r.ID,
			SourceType:        r.SourceType,
			AccountID:         r.AccountID,
			AccountName:       r.AccountName,
			PaymentMethodRaw:  r.PaymentMethodRaw,
			AmountMinor:       r.AmountMinor,
			Currency:          r.Currency,
			OccurredAtUtc:     r.OccurredAtUtc,
			Merchant:          r.Merchant,
			Category:          r.Category,
			ManualOverride:    r.ManualOverride,
			Reviewed:          r.Reviewed,
			Note:              r.Note,
			CreatedAtUtc:      r.CreatedAtUtc,
			UpdatedAtUtc:      r.UpdatedAtUtc,
			SourceCount:       len(roles),
			SourceRoles:       roles,
			IsForeignCurrency: r.Currency != currency,
		}
		if params.UnmergedOnly && item.SourceCount != 1 {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func GetTransactionDetail(gdb *gorm.DB, id string) (*TransactionDetailView, error) {
	_, curPtr := getLocaleConfig(gdb)

	type rawTxRow struct {
		ID               string  `gorm:"column:id"`
		SourceType       string  `gorm:"column:source_type"`
		AccountID        *string `gorm:"column:account_id"`
		AccountName      *string `gorm:"column:account_name"`
		PaymentMethodRaw *string `gorm:"column:payment_method_raw"`
		AmountMinor      int64   `gorm:"column:amount_minor"`
		Currency         string  `gorm:"column:currency"`
		OccurredAtUtc    string  `gorm:"column:occurred_at_utc"`
		Merchant         string  `gorm:"column:merchant"`
		Category         string  `gorm:"column:category"`
		ManualOverride   int     `gorm:"column:manual_override"`
		Reviewed         int     `gorm:"column:reviewed"`
		Note             *string `gorm:"column:note"`
		CreatedAtUtc     string  `gorm:"column:created_at_utc"`
		UpdatedAtUtc     string  `gorm:"column:updated_at_utc"`
	}
	var rows []rawTxRow
	gdb.Raw(`
		SELECT t.*, a.name AS account_name, `+paymentMethodSubquery+` AS payment_method_raw
		FROM transactions t
		LEFT JOIN accounts a ON a.id = t.account_id
		WHERE t.id = ?`, id).Scan(&rows)
	if len(rows) == 0 {
		return nil, errors.New("Transaction not found.")
	}
	txRow := rows[0]

	sources := []TransactionSourceView{}
	gdb.Raw(`
		SELECT
			ts.id, ts.email_id, ts.role, ts.match_score, ts.match_state,
			re.from_address, re.subject, re.cleaned_text, re.received_at_utc,
			ee.extraction_model, ee.raw_llm_json
		FROM transaction_sources ts
		JOIN raw_emails re ON re.id = ts.email_id
		LEFT JOIN email_extractions ee ON ee.email_id = ts.email_id
		WHERE ts.transaction_id = ?
		ORDER BY re.internal_date_ms ASC`, id).Scan(&sources)

	roles := make([]string, 0, len(sources))
	for _, s := range sources {
		roles = append(roles, s.Role)
	}
	isForeign := false
	if curPtr != nil && *curPtr != "" {
		isForeign = txRow.Currency != *curPtr
	}

	return &TransactionDetailView{
		Transaction: TransactionListItem{
			ID:                txRow.ID,
			SourceType:        txRow.SourceType,
			AccountID:         txRow.AccountID,
			AccountName:       txRow.AccountName,
			PaymentMethodRaw:  txRow.PaymentMethodRaw,
			AmountMinor:       txRow.AmountMinor,
			Currency:          txRow.Currency,
			OccurredAtUtc:     txRow.OccurredAtUtc,
			Merchant:          txRow.Merchant,
			Category:          txRow.Category,
			ManualOverride:    txRow.ManualOverride,
			Reviewed:          txRow.Reviewed,
			Note:              txRow.Note,
			CreatedAtUtc:      txRow.CreatedAtUtc,
			UpdatedAtUtc:      txRow.UpdatedAtUtc,
			SourceCount:       len(sources),
			SourceRoles:       roles,
			IsForeignCurrency: isForeign,
		},
		Sources: sources,
	}, nil
}

type CreateManualTransactionInput struct {
	AmountMinor   int64   `json:"amount_minor"`
	OccurredAtUtc string  `json:"occurred_at_utc"`
	Merchant      string  `json:"merchant"`
	Category      *string `json:"category"`
	AccountID     *string `json:"account_id"`
	Note          *string `json:"note"`
}

func CreateManualTransaction(gdb *gorm.DB, input CreateManualTransactionInput) (*TransactionListItem, error) {
	_, curPtr := getLocaleConfig(gdb)
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
	var accountID *string
	if input.AccountID != nil && strings.TrimSpace(*input.AccountID) != "" {
		trimmed := strings.TrimSpace(*input.AccountID)
		if err := AssertAccountExists(gdb, trimmed, ""); err != nil {
			return nil, err
		}
		accountID = &trimmed
	}
	var note *string
	if input.Note != nil && strings.TrimSpace(*input.Note) != "" {
		trimmed := strings.TrimSpace(*input.Note)
		note = &trimmed
	}

	id := uuid.NewString()
	nowUtc := NowUtcIsoSeconds()
	if err := gdb.Create(&db.Transaction{
		ID:             id,
		SourceType:     "manual",
		AccountID:      accountID,
		AmountMinor:    input.AmountMinor,
		Currency:       *curPtr,
		OccurredAtUtc:  occurred,
		Merchant:       merchant,
		Category:       category,
		ManualOverride: 0,
		Note:           note,
		CreatedAtUtc:   nowUtc,
		UpdatedAtUtc:   nowUtc,
	}).Error; err != nil {
		return nil, err
	}
	detail, err := GetTransactionDetail(gdb, id)
	if err != nil {
		return nil, err
	}
	return &detail.Transaction, nil
}

type UpdateTransactionInput struct {
	AmountMinor            *int64  `json:"amount_minor"`
	OccurredAtUtc          *string `json:"occurred_at_utc"`
	Merchant               *string `json:"merchant"`
	Category               *string `json:"category"`
	HasAccountID           bool    `json:"-"`
	AccountID              *string `json:"account_id"`
	HasNote                bool    `json:"-"`
	Note                   *string `json:"note"`
	ConvertToLocalCurrency *bool   `json:"convertToLocalCurrency"`
}

func UpdateTransaction(gdb *gorm.DB, id string, patch UpdateTransactionInput) (*TransactionListItem, error) {
	var item TransactionListItem
	err := gdb.Transaction(func(tx *gorm.DB) error {
		_, appCurrency := getLocaleConfig(tx)
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
			category = strings.TrimSpace(*patch.Category)
		}
		if category == "" {
			return errors.New("Category is required.")
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

		if err := tx.Model(&db.Transaction{}).Where("id = ?", id).Updates(map[string]any{
			"account_id":      accountID,
			"amount_minor":    amountMinor,
			"currency":        nextCurrency,
			"occurred_at_utc": occurred,
			"merchant":        merchant,
			"category":        category,
			"manual_override": manualOverride,
			"note":            note,
			"updated_at_utc":  NowUtcIsoSeconds(),
		}).Error; err != nil {
			return err
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

func DeleteTransaction(gdb *gorm.DB, id string) error {
	return gdb.Transaction(func(tx *gorm.DB) error {
		var existing db.Transaction
		if err := tx.Select("id").Where("id = ?", id).First(&existing).Error; err != nil {
			return errors.New("Transaction not found.")
		}
		var emailIDs []string
		tx.Model(&db.TransactionSource{}).Where("transaction_id = ?", id).Pluck("email_id", &emailIDs)
		if len(emailIDs) > 0 {
			tx.Model(&db.RawEmail{}).Where("id IN ?", emailIDs).Update("status", "dismissed")
		}
		return tx.Where("id = ?", id).Delete(&db.Transaction{}).Error
	})
}
