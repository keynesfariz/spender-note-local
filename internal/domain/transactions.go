package domain

import (
	"errors"
	"sort"
	"strings"
	"time"

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
	Configured              bool               `json:"configured"`
	Month                   *string            `json:"month"`
	Timezone                *string            `json:"timezone"`
	Currency                *string            `json:"currency"`
	StartUtc                *string            `json:"startUtc"`
	EndUtc                  *string            `json:"endUtc"`
	TotalSpentMinor         int64              `json:"totalSpentMinor"`
	IncomeMinor             int64              `json:"incomeMinor"`
	NetSpentMinor           int64              `json:"netSpentMinor"`
	CardPaymentsMinor       int64              `json:"cardPaymentsMinor"`
	ForeignCardPaymentCount int64              `json:"foreignCardPaymentCount"`
	TransactionCount        int64              `json:"transactionCount"`
	ForeignCurrencyCount    int64              `json:"foreignCurrencyCount"`
	ByCategory              []SpendingGroupRow `json:"byCategory"`
	ByMerchant              []SpendingGroupRow `json:"byMerchant"`
	ByAccount               []SpendingGroupRow `json:"byAccount"`
}

type TransactionTagItem struct {
	ID   string `gorm:"column:id" json:"id"`
	Name string `gorm:"column:name" json:"name"`
}

type TransactionListItem struct {
	ID                     string               `json:"id"`
	SourceType             string               `json:"source_type"`
	Kind                   string               `json:"kind"`
	AccountID              *string              `json:"account_id"`
	AccountName            *string              `json:"account_name"`
	DestinationAccountID   *string              `json:"destination_account_id"`
	DestinationAccountName *string              `json:"destination_account_name"`
	DestinationAmountMinor *int64               `json:"destination_amount_minor"`
	DestinationCurrency    *string              `json:"destination_currency"`
	SourcePostedAtUtc      *string              `json:"source_posted_at_utc"`
	DestinationPostedAtUtc *string              `json:"destination_posted_at_utc"`
	PaymentMethodRaw       *string              `json:"payment_method_raw"`
	AmountMinor            int64                `json:"amount_minor"`
	Currency               string               `json:"currency"`
	OccurredAtUtc          string               `json:"occurred_at_utc"`
	Merchant               string               `json:"merchant"`
	Remark                 *string              `json:"remark"`
	RemarkUserEdited       bool                 `json:"remark_user_edited"`
	Category               string               `json:"category"`
	Tags                   []TransactionTagItem `json:"tags"`
	ManualOverride         int                  `json:"manual_override"`
	Reviewed               int                  `json:"reviewed"`
	Note                   *string              `json:"note"`
	CreatedAtUtc           string               `json:"created_at_utc"`
	UpdatedAtUtc           string               `json:"updated_at_utc"`
	SourceCount            int                  `json:"sourceCount"`
	SourceRoles            []string             `json:"sourceRoles"`
	IsForeignCurrency      bool                 `json:"isForeignCurrency"`
}

type transactionLedgerRow struct {
	ID                     string  `gorm:"column:id"`
	SourceType             string  `gorm:"column:source_type"`
	AccountID              *string `gorm:"column:account_id"`
	AccountName            *string `gorm:"column:account_name"`
	Kind                   string  `gorm:"column:kind"`
	DestinationAccountID   *string `gorm:"column:destination_account_id"`
	DestinationAccountName *string `gorm:"column:destination_account_name"`
	DestinationAmountMinor *int64  `gorm:"column:destination_amount_minor"`
	DestinationCurrency    *string `gorm:"column:destination_currency"`
	SourcePostedAtUtc      *string `gorm:"column:source_posted_at_utc"`
	DestinationPostedAtUtc *string `gorm:"column:destination_posted_at_utc"`
	PaymentMethodRaw       *string `gorm:"column:payment_method_raw"`
	AmountMinor            int64   `gorm:"column:amount_minor"`
	Currency               string  `gorm:"column:currency"`
	OccurredAtUtc          string  `gorm:"column:occurred_at_utc"`
	Merchant               string  `gorm:"column:merchant"`
	Remark                 *string `gorm:"column:remark"`
	RemarkUserEdited       bool    `gorm:"column:remark_user_edited"`
	Category               string  `gorm:"column:category"`
	ManualOverride         int     `gorm:"column:manual_override"`
	Reviewed               int     `gorm:"column:reviewed"`
	Note                   *string `gorm:"column:note"`
	CreatedAtUtc           string  `gorm:"column:created_at_utc"`
	UpdatedAtUtc           string  `gorm:"column:updated_at_utc"`
}

func (r transactionLedgerRow) listItem(paymentMethod *string, tags []TransactionTagItem, roles []string, isForeign bool) TransactionListItem {
	return TransactionListItem{
		ID:                     r.ID,
		SourceType:             r.SourceType,
		AccountID:              r.AccountID,
		AccountName:            r.AccountName,
		Kind:                   r.Kind,
		DestinationAccountID:   r.DestinationAccountID,
		DestinationAccountName: r.DestinationAccountName,
		DestinationAmountMinor: r.DestinationAmountMinor,
		DestinationCurrency:    r.DestinationCurrency,
		SourcePostedAtUtc:      r.SourcePostedAtUtc,
		DestinationPostedAtUtc: r.DestinationPostedAtUtc,
		PaymentMethodRaw:       paymentMethod,
		AmountMinor:            r.AmountMinor,
		Currency:               r.Currency,
		OccurredAtUtc:          r.OccurredAtUtc,
		Merchant:               r.Merchant,
		Remark:                 r.Remark,
		RemarkUserEdited:       r.RemarkUserEdited,
		Category:               r.Category,
		Tags:                   tags,
		ManualOverride:         r.ManualOverride,
		Reviewed:               r.Reviewed,
		Note:                   r.Note,
		CreatedAtUtc:           r.CreatedAtUtc,
		UpdatedAtUtc:           r.UpdatedAtUtc,
		SourceCount:            len(roles),
		SourceRoles:            roles,
		IsForeignCurrency:      isForeign,
	}
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
	return GetFilteredMonthlySpendingSummary(gdb, monthParam, "", "")
}

func GetFilteredMonthlySpendingSummary(gdb *gorm.DB, monthParam, category, tag string) (*MonthlySpendingSummary, error) {
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
		Kind        string  `gorm:"column:kind"`
		AmountMinor int64   `gorm:"column:amount_minor"`
		Currency    string  `gorm:"column:currency"`
		Category    string  `gorm:"column:category"`
		Merchant    string  `gorm:"column:merchant"`
		AccountID   *string `gorm:"column:account_id"`
		AccountName *string `gorm:"column:account_name"`
	}
	var monthRows []summaryTxRow
	conds, args := transactionTaxonomyFilters(category, tag)
	entryConditions := append([]string{"t.kind IN ('expense', 'income')"}, conds...)
	if err := gdb.Raw(`
		SELECT
			t.kind,
			CASE WHEN t.kind = 'transfer' THEN t.destination_amount_minor ELSE t.amount_minor END AS amount_minor,
			CASE WHEN t.kind = 'transfer' THEN t.destination_currency ELSE t.currency END AS currency,
			c.name AS category, t.merchant, t.account_id, a.name AS account_name
		FROM transactions t
		JOIN categories c ON c.id = t.category_id
		LEFT JOIN accounts a ON a.id = t.account_id
		LEFT JOIN accounts destination ON destination.id = t.destination_account_id
		WHERE t.occurred_at_utc >= ? AND t.occurred_at_utc < ?
		  AND ((`+strings.Join(entryConditions, " AND ")+`) OR (t.kind = 'transfer' AND destination.type = 'credit_card'))`,
		append([]any{startUtc, endUtc}, args...)...,
	).Scan(&monthRows).Error; err != nil {
		return nil, err
	}

	var expenses, income, cardPayments, localCount, foreignCount, foreignPayments int64
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
		if r.Kind == "transfer" {
			if r.Currency == currency {
				cardPayments += r.AmountMinor
			} else {
				foreignPayments++
			}
			continue
		}
		if r.Currency != currency {
			foreignCount++
			continue
		}
		localCount++
		if r.Kind == "income" {
			income += r.AmountMinor
			continue
		}
		expenses += r.AmountMinor

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
		Configured:              true,
		Month:                   &month,
		Timezone:                &tz,
		Currency:                &currency,
		StartUtc:                &startUtc,
		EndUtc:                  &endUtc,
		TotalSpentMinor:         expenses,
		IncomeMinor:             income,
		NetSpentMinor:           expenses - income,
		CardPaymentsMinor:       cardPayments,
		ForeignCardPaymentCount: foreignPayments,
		TransactionCount:        localCount + foreignCount,
		ForeignCurrencyCount:    foreignCount,
		ByCategory:              sortGroups(catMap, 0),
		ByMerchant:              sortGroups(merchMap, 10),
		ByAccount:               sortGroups(acctMap, 0),
	}, nil
}

func transactionTaxonomyFilters(category, tag string) ([]string, []any) {
	var conds []string
	var args []any
	if category != "" {
		conds = append(conds, "t.category_id = (SELECT id FROM categories WHERE name = ?)")
		args = append(args, category)
	}
	if t := strings.TrimSpace(tag); t != "" {
		conds = append(conds, `EXISTS (
			SELECT 1
			FROM transaction_tags tt
			JOIN tags tg ON tg.id = tt.tag_id
			WHERE tt.transaction_id = t.id
			  AND (tg.normalized_name = ? OR tg.id = ?)
		)`)
		args = append(args, strings.ToLower(t), t)
	}
	return conds, args
}

type ListTransactionsParams struct {
	LedgerRange    string
	Month          string
	StartDate      string
	EndDate        string
	Category       string
	Tag            string
	AccountID      string
	Kind           string
	Search         string
	UnreviewedOnly bool
	ShowSkipped    *bool
	Page           int
	PageSize       int
}

type LedgerScope struct {
	Configured bool   `json:"configured"`
	Timezone   string `json:"timezone"`
	Today      string `json:"today"`
	NextDayUtc string `json:"nextDayUtc"`
	StartDate  string `json:"startDate"`
	EndDate    string `json:"endDate"`
}

type PaginatedTransactionsResponse struct {
	Ledger                 *LedgerScope          `json:"ledger,omitempty"`
	Items                  []TransactionListItem `json:"items"`
	TotalCount             int64                 `json:"totalCount"`
	Page                   int                   `json:"page"`
	PageSize               int                   `json:"pageSize"`
	TotalInRangeCount      int64                 `json:"totalInRangeCount"`
	UnreviewedInRangeCount int64                 `json:"unreviewedInRangeCount"`
	GhostRows              []GhostRowItem        `json:"ghostRows"`
	GhostInRangeCount      int64                 `json:"ghostInRangeCount"`
	TotalGhostCount        int64                 `json:"totalGhostCount"`
	GhostRowsTruncated     bool                  `json:"ghostRowsTruncated"`
}

func ListTransactions(gdb *gorm.DB, params ListTransactionsParams) (*PaginatedTransactionsResponse, error) {
	return listTransactionsAt(gdb, params, time.Now())
}

func listTransactionsAt(gdb *gorm.DB, params ListTransactionsParams, now time.Time) (*PaginatedTransactionsResponse, error) {
	var ledger *LedgerScope
	if params.LedgerRange != "" {
		ledger = &LedgerScope{}
	}
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
			Ledger:    ledger,
			Items:     []TransactionListItem{},
			GhostRows: []GhostRowItem{},
			Page:      page,
			PageSize:  pageSize,
		}, nil
	}
	tz := *tzPtr
	currency := *curPtr
	if ledger != nil {
		loc, err := LoadTimezone(tz)
		if err != nil {
			return nil, err
		}
		local := now.In(loc)
		nextDay := localDayStart(local.Year(), local.Month(), local.Day()+1, loc)
		ledger.Configured = true
		ledger.Timezone = tz
		ledger.Today = local.Format(time.DateOnly)
		ledger.NextDayUtc = ToUtcIsoSeconds(nextDay)
		if params.LedgerRange == "dates" {
			if params.StartDate == "" {
				params.StartDate = ledger.Today
			}
			if params.EndDate == "" {
				params.EndDate = params.StartDate
			}
			ledger.StartDate, ledger.EndDate = params.StartDate, params.EndDate
		}
	}

	var rangeConds []string
	var rangeArgs []any
	var rangeStartUtc, rangeEndUtc string

	if strings.TrimSpace(params.StartDate) != "" || strings.TrimSpace(params.EndDate) != "" {
		startUtc, endUtc, err := GetDateRangeUtcBounds(params.StartDate, params.EndDate, tz)
		if err != nil {
			return nil, err
		}
		rangeStartUtc, rangeEndUtc = startUtc, endUtc
		rangeConds = append(rangeConds, "t.occurred_at_utc >= ? AND t.occurred_at_utc < ?")
		rangeArgs = append(rangeArgs, startUtc, endUtc)
	} else if strings.TrimSpace(params.Month) != "" {
		startUtc, endUtc, err := GetMonthUtcBounds(params.Month, tz)
		if err != nil {
			return nil, err
		}
		rangeStartUtc, rangeEndUtc = startUtc, endUtc
		rangeConds = append(rangeConds, "t.occurred_at_utc >= ? AND t.occurred_at_utc < ?")
		rangeArgs = append(rangeArgs, startUtc, endUtc)
	}

	conds := append([]string{}, rangeConds...)
	args := append([]any{}, rangeArgs...)

	taxonomyConds, taxonomyArgs := transactionTaxonomyFilters(params.Category, params.Tag)
	conds = append(conds, taxonomyConds...)
	args = append(args, taxonomyArgs...)
	if params.Kind != "" {
		switch params.Kind {
		case "expense", "income", "transfer":
			conds = append(conds, "t.kind = ?")
			args = append(args, params.Kind)
			if params.Kind != "expense" {
				showSkipped := false
				params.ShowSkipped = &showSkipped
			}
		default:
			return nil, errors.New("Invalid transaction kind.")
		}
	}
	if params.AccountID != "" && params.AccountID != "all" {
		if params.AccountID == "unassigned" {
			conds = append(conds, "t.account_id IS NULL")
		} else {
			conds = append(conds, "(t.account_id = ? OR t.destination_account_id = ?)")
			args = append(args, params.AccountID, params.AccountID)
		}
	}
	if q := strings.TrimSpace(params.Search); q != "" {
		like := "%" + strings.ToLower(q) + "%"
		conds = append(conds, "(LOWER(t.merchant) LIKE ? OR LOWER(COALESCE(t.remark, '')) LIKE ? OR LOWER(COALESCE(t.note, '')) LIKE ?)")
		args = append(args, like, like, like)
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

	offset := (page - 1) * pageSize
	pageArgs := append(append([]any{}, args...), pageSize, offset)

	var rows []transactionLedgerRow
	query := `
		SELECT
			t.*, c.name AS category,
			a.name AS account_name, destination.name AS destination_account_name
		FROM transactions t
		JOIN categories c ON c.id = t.category_id
		LEFT JOIN accounts a ON a.id = t.account_id
		LEFT JOIN accounts destination ON destination.id = t.destination_account_id
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
		out = append(out, r.listItem(pmByTx[r.ID], tags, roles, r.Currency != currency))
	}
	prevPageOldestUtc := ""
	if (params.ShowSkipped == nil || *params.ShowSkipped) && page > 1 && totalCount > 0 && len(rows) > 0 {
		prevArgs := append(append([]any{}, args...), offset-1)
		_ = gdb.Raw(`
			SELECT t.occurred_at_utc
			FROM transactions t
			`+whereClause+`
			ORDER BY t.occurred_at_utc DESC, t.created_at_utc DESC, t.id DESC
			LIMIT 1 OFFSET ?`,
			prevArgs...,
		).Scan(&prevPageOldestUtc).Error
	}
	currPageOldestUtc := ""
	if len(rows) > 0 {
		currPageOldestUtc = rows[len(rows)-1].OccurredAtUtc
	}

	ghostRes, err := queryGhostRowsForTransactions(
		gdb, params, currency, rangeStartUtc, rangeEndUtc,
		totalCount, offset, len(rows), prevPageOldestUtc, currPageOldestUtc,
	)
	if err != nil {
		return nil, err
	}

	return &PaginatedTransactionsResponse{
		Ledger:                 ledger,
		Items:                  out,
		TotalCount:             totalCount,
		Page:                   page,
		PageSize:               pageSize,
		TotalInRangeCount:      rangeStats.TotalInRange,
		UnreviewedInRangeCount: rangeStats.UnreviewedInRange,
		GhostRows:              ghostRes.Items,
		GhostInRangeCount:      ghostRes.GhostInRangeCount,
		TotalGhostCount:        ghostRes.TotalGhostCount,
		GhostRowsTruncated:     ghostRes.GhostRowsTruncated,
	}, nil
}

func GetTransactionDetail(gdb *gorm.DB, id string) (*TransactionDetailView, error) {
	return getTransactionDetail(gdb, id, true)
}

func getTransactionDetail(gdb *gorm.DB, id string, includeSourceContent bool) (*TransactionDetailView, error) {
	_, curPtr, err := getLocaleConfig(gdb)
	if err != nil {
		return nil, err
	}

	var rows []transactionLedgerRow
	if err := gdb.Raw(`
		SELECT t.*, c.name AS category, a.name AS account_name,
			destination.name AS destination_account_name, `+paymentMethodSubquery+` AS payment_method_raw
		FROM transactions t
		JOIN categories c ON c.id = t.category_id
		LEFT JOIN accounts a ON a.id = t.account_id
		LEFT JOIN accounts destination ON destination.id = t.destination_account_id
		WHERE t.id = ?`, id).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("Transaction not found.")
	}
	txRow := rows[0]

	const sourceColumns = `ts.id, ts.email_id, ts.role, ts.match_state, re.from_address, re.subject, re.received_at_utc`
	columns, extractionJoin := sourceColumns, ""
	if includeSourceContent {
		columns += ", ts.match_score, re.cleaned_text, ee.extraction_model, ee.raw_llm_json"
		extractionJoin = "LEFT JOIN email_extractions ee ON ee.email_id = ts.email_id"
	}
	sources := []TransactionSourceView{}
	if err := gdb.Raw(`
		SELECT `+columns+`
		FROM transaction_sources ts
		JOIN raw_emails re ON re.id = ts.email_id
		`+extractionJoin+`
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
		Transaction: txRow.listItem(txRow.PaymentMethodRaw, tags, roles, isForeign),
		Sources:     sources,
	}, nil
}
