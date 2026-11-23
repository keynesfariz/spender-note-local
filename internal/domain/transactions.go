package domain

import (
	"errors"
	"sort"
	"strings"

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

type TransactionTagItem struct {
	ID   string `gorm:"column:id" json:"id"`
	Name string `gorm:"column:name" json:"name"`
}

type TransactionListItem struct {
	ID                string               `json:"id"`
	SourceType        string               `json:"source_type"`
	AccountID         *string              `json:"account_id"`
	AccountName       *string              `json:"account_name"`
	PaymentMethodRaw  *string              `json:"payment_method_raw"`
	AmountMinor       int64                `json:"amount_minor"`
	Currency          string               `json:"currency"`
	OccurredAtUtc     string               `json:"occurred_at_utc"`
	Merchant          string               `json:"merchant"`
	Category          string               `json:"category"`
	Tags              []TransactionTagItem `json:"tags"`
	ManualOverride    int                  `json:"manual_override"`
	Reviewed          int                  `json:"reviewed"`
	Note              *string              `json:"note"`
	CreatedAtUtc      string               `json:"created_at_utc"`
	UpdatedAtUtc      string               `json:"updated_at_utc"`
	SourceCount       int                  `json:"sourceCount"`
	SourceRoles       []string             `json:"sourceRoles"`
	IsForeignCurrency bool                 `json:"isForeignCurrency"`
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

func getLocaleConfig(gdb *gorm.DB) (tz *string, cur *string, err error) {
	var s db.AppSetting
	err = gdb.Select("timezone, currency").First(&s, 1).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	return s.Timezone, s.Currency, nil
}

func GetMonthlySpendingSummary(gdb *gorm.DB, monthParam string) (*MonthlySpendingSummary, error) {
	tzPtr, curPtr, err := getLocaleConfig(gdb)
	if err != nil {
		return nil, err
	}
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

	type summaryTxRow struct {
		AmountMinor int64   `gorm:"column:amount_minor"`
		Currency    string  `gorm:"column:currency"`
		Category    string  `gorm:"column:category"`
		Merchant    string  `gorm:"column:merchant"`
		AccountID   *string `gorm:"column:account_id"`
		AccountName *string `gorm:"column:account_name"`
	}
	var monthRows []summaryTxRow
	if err := gdb.Raw(`
		SELECT t.amount_minor, t.currency, t.category, t.merchant, t.account_id, a.name AS account_name
		FROM transactions t
		LEFT JOIN accounts a ON a.id = t.account_id
		WHERE t.occurred_at_utc >= ? AND t.occurred_at_utc < ?`,
		startUtc, endUtc,
	).Scan(&monthRows).Error; err != nil {
		return nil, err
	}

	var localTotal, localCount, foreignCount int64
	catMap := map[string]*SpendingGroupRow{}
	merchMap := map[string]*SpendingGroupRow{}
	acctMap := map[string]*SpendingGroupRow{}

	addGroup := func(m map[string]*SpendingGroupRow, key, label string, amount int64, keepMinLabel bool) {
		if g, ok := m[key]; ok {
			g.TotalMinor += amount
			g.Count++
			if keepMinLabel && label < g.Label {
				g.Label = label
			}
			return
		}
		m[key] = &SpendingGroupRow{Key: key, Label: label, TotalMinor: amount, Count: 1}
	}

	for _, r := range monthRows {
		if r.Currency != currency {
			foreignCount++
			continue
		}
		localTotal += r.AmountMinor
		localCount++

		addGroup(catMap, r.Category, r.Category, r.AmountMinor, false)
		addGroup(merchMap, strings.ToLower(strings.TrimSpace(r.Merchant)), r.Merchant, r.AmountMinor, true)

		aKey := "unassigned"
		if r.AccountID != nil && *r.AccountID != "" {
			aKey = *r.AccountID
		}
		aLabel := "Unassigned"
		if r.AccountName != nil && *r.AccountName != "" {
			aLabel = *r.AccountName
		}
		addGroup(acctMap, aKey, aLabel, r.AmountMinor, false)
	}

	sortGroups := func(m map[string]*SpendingGroupRow, limit int) []SpendingGroupRow {
		res := make([]SpendingGroupRow, 0, len(m))
		for _, g := range m {
			res = append(res, *g)
		}
		sort.Slice(res, func(i, j int) bool {
			if res[i].TotalMinor != res[j].TotalMinor {
				return res[i].TotalMinor > res[j].TotalMinor
			}
			return res[i].Label < res[j].Label
		})
		if limit > 0 && len(res) > limit {
			res = res[:limit]
		}
		return res
	}

	return &MonthlySpendingSummary{
		Configured:           true,
		Month:                &month,
		Timezone:             &tz,
		Currency:             &currency,
		StartUtc:             &startUtc,
		EndUtc:               &endUtc,
		TotalSpentMinor:      localTotal,
		TransactionCount:     localCount + foreignCount,
		ForeignCurrencyCount: foreignCount,
		ByCategory:           sortGroups(catMap, 0),
		ByMerchant:           sortGroups(merchMap, 10),
		ByAccount:            sortGroups(acctMap, 0),
	}, nil
}

type ListTransactionsParams struct {
	Month          string
	StartDate      string
	EndDate        string
	Category       string
	Tag            string
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

	tzPtr, curPtr, err := getLocaleConfig(gdb)
	if err != nil {
		return nil, err
	}
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
	if t := strings.TrimSpace(params.Tag); t != "" && t != "all" {
		conds = append(conds, `EXISTS (
			SELECT 1
			FROM transaction_tags tt
			JOIN tags tg ON tg.id = tt.tag_id
			WHERE tt.transaction_id = t.id
			  AND (tg.name = ? OR tg.normalized_name = ? OR tg.id = ?)
		)`)
		args = append(args, t, strings.ToLower(t), t)
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

	totalCount := rangeStats.TotalInRange
	if whereClause != rangeWhere {
		if err := gdb.Raw(`SELECT COUNT(*) FROM transactions t `+whereClause, args...).Scan(&totalCount).Error; err != nil {
			return nil, err
		}
	}

	type rawTxRow struct {
		ID             string  `gorm:"column:id"`
		SourceType     string  `gorm:"column:source_type"`
		AccountID      *string `gorm:"column:account_id"`
		AccountName    *string `gorm:"column:account_name"`
		AmountMinor    int64   `gorm:"column:amount_minor"`
		Currency       string  `gorm:"column:currency"`
		OccurredAtUtc  string  `gorm:"column:occurred_at_utc"`
		Merchant       string  `gorm:"column:merchant"`
		Category       string  `gorm:"column:category"`
		ManualOverride int     `gorm:"column:manual_override"`
		Reviewed       int     `gorm:"column:reviewed"`
		Note           *string `gorm:"column:note"`
		CreatedAtUtc   string  `gorm:"column:created_at_utc"`
		UpdatedAtUtc   string  `gorm:"column:updated_at_utc"`
	}

	offset := (page - 1) * pageSize
	pageArgs := append(append([]any{}, args...), pageSize, offset)

	var rows []rawTxRow
	query := `
		SELECT
			t.*,
			a.name AS account_name
		FROM transactions t
		LEFT JOIN accounts a ON a.id = t.account_id
		` + whereClause + `
		ORDER BY t.occurred_at_utc DESC, t.created_at_utc DESC, t.id DESC
		LIMIT ? OFFSET ?`

	if err := gdb.Raw(query, pageArgs...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	rolesByTx := map[string][]string{}
	pmByTx := map[string]*string{}
	pmIsReceiptByTx := map[string]bool{}
	tagsByTx := map[string][]TransactionTagItem{}
	if len(rows) > 0 {
		txIDs := make([]string, len(rows))
		for i, r := range rows {
			txIDs[i] = r.ID
		}
		type pageSourceRow struct {
			TransactionID    string  `gorm:"column:transaction_id"`
			Role             string  `gorm:"column:role"`
			PaymentMethodRaw *string `gorm:"column:payment_method_raw"`
		}
		var srcRows []pageSourceRow
		if err := gdb.Raw(`
			SELECT ts.transaction_id, ts.role, ee.payment_method_raw
			FROM transaction_sources ts
			LEFT JOIN email_extractions ee ON ee.email_id = ts.email_id
			WHERE ts.transaction_id IN ?
			ORDER BY ts.rowid ASC`, txIDs).Scan(&srcRows).Error; err != nil {
			return nil, err
		}
		for _, s := range srcRows {
			rolesByTx[s.TransactionID] = append(rolesByTx[s.TransactionID], s.Role)
			if s.PaymentMethodRaw != nil {
				if s.Role == "receipt" && !pmIsReceiptByTx[s.TransactionID] {
					v := *s.PaymentMethodRaw
					pmByTx[s.TransactionID] = &v
					pmIsReceiptByTx[s.TransactionID] = true
				} else if _, exists := pmByTx[s.TransactionID]; !exists {
					v := *s.PaymentMethodRaw
					pmByTx[s.TransactionID] = &v
				}
			}
		}

		type pageTagRow struct {
			TransactionID string `gorm:"column:transaction_id"`
			TagID         string `gorm:"column:tag_id"`
			TagName       string `gorm:"column:tag_name"`
		}
		var tagRows []pageTagRow
		if err := gdb.Raw(`
			SELECT tt.transaction_id, tg.id AS tag_id, tg.name AS tag_name
			FROM transaction_tags tt
			JOIN tags tg ON tg.id = tt.tag_id
			WHERE tt.transaction_id IN ?
			ORDER BY LOWER(tg.name) ASC, tg.id ASC`, txIDs).Scan(&tagRows).Error; err != nil {
			return nil, err
		}
		for _, tr := range tagRows {
			tagsByTx[tr.TransactionID] = append(tagsByTx[tr.TransactionID], TransactionTagItem{
				ID:   tr.TagID,
				Name: tr.TagName,
			})
		}
	}

	out := make([]TransactionListItem, 0, len(rows))
	for _, r := range rows {
		roles := rolesByTx[r.ID]
		if roles == nil {
			roles = []string{}
		}
		tags := tagsByTx[r.ID]
		if tags == nil {
			tags = []TransactionTagItem{}
		}
		out = append(out, TransactionListItem{
			ID:                r.ID,
			SourceType:        r.SourceType,
			AccountID:         r.AccountID,
			AccountName:       r.AccountName,
			PaymentMethodRaw:  pmByTx[r.ID],
			AmountMinor:       r.AmountMinor,
			Currency:          r.Currency,
			OccurredAtUtc:     r.OccurredAtUtc,
			Merchant:          r.Merchant,
			Category:          r.Category,
			Tags:              tags,
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
	_, curPtr, err := getLocaleConfig(gdb)
	if err != nil {
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
	}
	var rows []rawTxRow
	if err := gdb.Raw(`
		SELECT t.*, a.name AS account_name, `+paymentMethodSubquery+` AS payment_method_raw
		FROM transactions t
		LEFT JOIN accounts a ON a.id = t.account_id
		WHERE t.id = ?`, id).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("Transaction not found.")
	}
	txRow := rows[0]

	sources := []TransactionSourceView{}
	if err := gdb.Raw(`
		SELECT
			ts.id, ts.email_id, ts.role, ts.match_score, ts.match_state,
			re.from_address, re.subject, re.cleaned_text, re.received_at_utc,
			ee.extraction_model, ee.raw_llm_json
		FROM transaction_sources ts
		JOIN raw_emails re ON re.id = ts.email_id
		LEFT JOIN email_extractions ee ON ee.email_id = ts.email_id
		WHERE ts.transaction_id = ?
		ORDER BY re.internal_date_ms ASC`, id).Scan(&sources).Error; err != nil {
		return nil, err
	}

	roles := make([]string, 0, len(sources))
	for _, s := range sources {
		roles = append(roles, s.Role)
	}
	tags := []TransactionTagItem{}
	if err := gdb.Raw(`
		SELECT tg.id, tg.name
		FROM transaction_tags tt
		JOIN tags tg ON tg.id = tt.tag_id
		WHERE tt.transaction_id = ?
		ORDER BY LOWER(tg.name) ASC, tg.id ASC`, id).Scan(&tags).Error; err != nil {
		return nil, err
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
			Tags:              tags,
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
