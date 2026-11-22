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
	Month          string
	StartDate      string
	EndDate        string
	Category       string
	AccountID      string
	Search         string
	UnreviewedOnly bool
	Page           int
	PageSize       int
}

type PaginatedTransactionsResponse struct {
	Items                  []TransactionListItem `json:"items"`
	TotalCount             int64                 `json:"totalCount"`
	Page                   int                   `json:"page"`
	PageSize               int                   `json:"pageSize"`
	TotalInRangeCount      int64                 `json:"totalInRangeCount"`
	UnreviewedInRangeCount int64                 `json:"unreviewedInRangeCount"`
}

func ListTransactions(gdb *gorm.DB, params ListTransactionsParams) (*PaginatedTransactionsResponse, error) {
	page := params.Page
	if page < 1 {
		page = 1
	}
	pageSize := params.PageSize
	if pageSize <= 0 {
		pageSize = 25
	}
	if pageSize > 100 {
		pageSize = 100
	}

	tzPtr, curPtr := getLocaleConfig(gdb)
	if tzPtr == nil || *tzPtr == "" || curPtr == nil || *curPtr == "" {
		return &PaginatedTransactionsResponse{
			Items:    []TransactionListItem{},
			Page:     page,
			PageSize: pageSize,
		}, nil
	}
	tz := *tzPtr
	currency := *curPtr

	var rangeConds []string
	var rangeArgs []any

	if strings.TrimSpace(params.StartDate) != "" || strings.TrimSpace(params.EndDate) != "" {
		startUtc, endUtc, err := GetDateRangeUtcBounds(params.StartDate, params.EndDate, tz)
		if err != nil {
			return nil, err
		}
		rangeConds = append(rangeConds, "t.occurred_at_utc >= ? AND t.occurred_at_utc < ?")
		rangeArgs = append(rangeArgs, startUtc, endUtc)
	} else if strings.TrimSpace(params.Month) != "" {
		startUtc, endUtc, err := GetMonthUtcBounds(params.Month, tz)
		if err != nil {
			return nil, err
		}
		rangeConds = append(rangeConds, "t.occurred_at_utc >= ? AND t.occurred_at_utc < ?")
		rangeArgs = append(rangeArgs, startUtc, endUtc)
	}

	conds := append([]string{}, rangeConds...)
	args := append([]any{}, rangeArgs...)

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
	if params.UnreviewedOnly {
		conds = append(conds, "t.reviewed = 0")
	}

	rangeWhere := ""
	if len(rangeConds) > 0 {
		rangeWhere = "WHERE " + strings.Join(rangeConds, " AND ")
	}
	whereClause := ""
	if len(conds) > 0 {
		whereClause = "WHERE " + strings.Join(conds, " AND ")
	}

	type rangeStatsRow struct {
		TotalInRange      int64 `gorm:"column:total_in_range"`
		UnreviewedInRange int64 `gorm:"column:unreviewed_in_range"`
	}
	var rangeStats rangeStatsRow
	if err := gdb.Raw(`
		SELECT
			COUNT(*) AS total_in_range,
			COALESCE(SUM(CASE WHEN t.reviewed = 0 THEN 1 ELSE 0 END), 0) AS unreviewed_in_range
		FROM transactions t
		`+rangeWhere, rangeArgs...,
	).Scan(&rangeStats).Error; err != nil {
		return nil, err
	}

	var totalCount int64
	if err := gdb.Raw(`SELECT COUNT(*) FROM transactions t `+whereClause, args...).Scan(&totalCount).Error; err != nil {
		return nil, err
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

	offset := (page - 1) * pageSize
	pageArgs := append(append([]any{}, args...), pageSize, offset)

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
		ORDER BY t.occurred_at_utc DESC, t.created_at_utc DESC, t.id DESC
		LIMIT ? OFFSET ?`

	if err := gdb.Raw(query, pageArgs...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]TransactionListItem, 0, len(rows))
	for _, r := range rows {
		roles := []string{}
		if r.RolesCSV != nil && *r.RolesCSV != "" {
			roles = strings.Split(*r.RolesCSV, ",")
		}
		out = append(out, TransactionListItem{
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
		})
	}
	return &PaginatedTransactionsResponse{
		Items:                  out,
		TotalCount:             totalCount,
		Page:                   page,
		PageSize:               pageSize,
		TotalInRangeCount:      rangeStats.TotalInRange,
		UnreviewedInRangeCount: rangeStats.UnreviewedInRange,
	}, nil
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
		Reviewed:       1,
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
			"reviewed":        1,
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

		if err := tx.Where("id = ?", dropID).Delete(&db.Transaction{}).Error; err != nil {
			return err
		}

		if err := tx.Model(&db.Transaction{}).Where("id = ?", keepID).Updates(map[string]any{
			"reviewed":       1,
			"updated_at_utc": NowUtcIsoSeconds(),
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

		survivorID := ids[0]
		absorbedIDs := ids[1:]

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
			"category":        category,
			"manual_override": 1,
			"reviewed":        1,
			"note":            note,
			"updated_at_utc":  NowUtcIsoSeconds(),
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
