package domain

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/glebarez/go-sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"portspendernote/internal/db"
	"portspendernote/internal/llm"
)

type ConflictError struct {
	Message string
}

func (e *ConflictError) Error() string { return e.Message }

type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string { return e.Message }

type UnprocessableError struct {
	Message string
}

func (e *UnprocessableError) Error() string { return e.Message }

type GhostRowItem struct {
	EmailID              string  `json:"email_id"`
	FromAddress          string  `json:"from_address"`
	SenderEmail          string  `json:"sender_email"`
	Subject              string  `json:"subject"`
	AmountMinor          int64   `json:"amount_minor"`
	Currency             string  `json:"currency"`
	OccurredAtUtc        string  `json:"occurred_at_utc"`
	MerchantRaw          *string `json:"merchant_raw"`
	Category             *string `json:"category"`
	AccountID            *string `json:"account_id"`
	AccountName          *string `json:"account_name"`
	PaymentMethodRaw     *string `json:"payment_method_raw"`
	TriageSignatureKey   string  `json:"triage_signature_key"`
	TriageSubjectPattern string  `json:"triage_subject_pattern"`
	TriageBodyCue        string  `json:"triage_body_cue"`
	IsForeignCurrency    bool    `json:"isForeignCurrency"`
}

type TriagePriorPolicyState struct {
	Existed bool   `json:"existed"`
	Action  string `json:"action"`
}

type TriageUndoPayload struct {
	Action                       string                 `json:"action"`
	SenderEmail                  string                 `json:"sender_email"`
	SignatureKey                 string                 `json:"signature_key"`
	SubjectPattern               string                 `json:"subject_pattern"`
	BodyCue                      string                 `json:"body_cue"`
	PriorPolicy                  TriagePriorPolicyState `json:"prior_policy"`
	AffectedEmailIDs             []string               `json:"affected_email_ids"`
	CreatedTransactionIDs        []string               `json:"created_transaction_ids"`
	CreatedProvisionalAccountIDs []string               `json:"created_provisional_account_ids"`
}

type ApplyTriageVerdictInput struct {
	EmailID   string `json:"emailId"`
	Action    string `json:"action"`
	StartDate string `json:"startDate,omitempty"`
	EndDate   string `json:"endDate,omitempty"`
}

type SenderTriagePolicyView struct {
	ID                 string `json:"id"`
	SenderEmail        string `json:"sender_email"`
	SignatureKey       string `json:"signature_key"`
	SubjectPattern     string `json:"subject_pattern"`
	BodyCue            string `json:"body_cue"`
	Action             string `json:"action"`
	IgnoredCount       int64  `json:"ignored_count"`
	GhostCount         int64  `json:"ghost_count"`
	RestoredGhostCount int64  `json:"restoredGhostCount,omitempty"`
	CreatedAtUtc       string `json:"created_at_utc"`
	UpdatedAtUtc       string `json:"updated_at_utc"`
}

type TriageVerdictResult struct {
	Policy                SenderTriagePolicyView `json:"policy"`
	AffectedEmailIDs      []string               `json:"affectedEmailIds"`
	CreatedTransactionIDs []string               `json:"createdTransactionIds"`
	AffectedCount         int                    `json:"affectedCount"`
	OutOfRangeCount       int                    `json:"outOfRangeCount"`
	UndoPayload           TriageUndoPayload      `json:"undoPayload"`
}

type TriageUndoInput struct {
	UndoPayload TriageUndoPayload `json:"undoPayload"`
}

type TriageUndoResult struct {
	Ok            bool `json:"ok"`
	RestoredCount int  `json:"restoredCount"`
}

type ListTriagePoliciesParams struct {
	SenderEmail string
	Search      string
}

type UpdateTriagePolicyInput struct {
	Action string `json:"action"`
}

type DeleteTriagePolicyResult struct {
	Ok                 bool  `json:"ok"`
	RestoredGhostCount int64 `json:"restoredGhostCount"`
}

type IgnoredEmailItem struct {
	EmailID              string  `json:"email_id"`
	FromAddress          string  `json:"from_address"`
	SenderEmail          string  `json:"sender_email"`
	Subject              string  `json:"subject"`
	CleanedText          *string `json:"cleaned_text,omitempty"`
	ReceivedAtUtc        string  `json:"received_at_utc"`
	AmountMinor          *int64  `json:"amount_minor"`
	Currency             *string `json:"currency"`
	OccurredAtUtc        *string `json:"occurred_at_utc"`
	MerchantRaw          *string `json:"merchant_raw"`
	Category             *string `json:"category"`
	ExtractionModel      string  `json:"extraction_model"`
	TriageSignatureKey   string  `json:"triage_signature_key"`
	TriageSubjectPattern string  `json:"triage_subject_pattern"`
	TriageBodyCue        string  `json:"triage_body_cue"`
	PreLlmSkipped        bool    `json:"preLlmSkipped"`
}

type ListIgnoredEmailsParams struct {
	SenderEmail string
	Search      string
	Page        int
	PageSize    int
	SummaryOnly bool
}

type PaginatedIgnoredEmailsResponse struct {
	Items            []IgnoredEmailItem `json:"items"`
	TotalCount       int64              `json:"totalCount"`
	Page             int                `json:"page"`
	PageSize         int                `json:"pageSize"`
	AvailableSenders []string           `json:"availableSenders"`
}

type PromoteIgnoredEmailResult struct {
	Transaction TransactionListItem `json:"transaction"`
	EmailID     string              `json:"emailId"`
	UsedLlm     bool                `json:"usedLlm"`
}

const unbackfilledCondition = `re.status = 'non_transaction' AND (ee.email_id IS NULL OR (COALESCE(ee.extraction_model, '') != 'rule:billing_statement' AND (ee.triage_signature_key IS NULL OR ee.triage_signature_key = '')))`

const triageCandidateColumns = `
	re.id AS email_id, re.sender_id, s.role_hint AS sender_role_hint,
	re.from_address, re.subject, re.cleaned_text, re.received_at_utc,
	re.status AS raw_status, ee.email_kind, ee.amount_minor, ee.currency,
	ee.occurred_at_utc, ee.merchant_raw, ee.category, ee.card_last4,
	ee.payment_method_raw, ee.bank_account_hint, ee.llm_account_id,
	ee.account_type_guess, ee.reference_code, ee.extraction_model,
	ee.triage_signature_key, ee.triage_subject_pattern, ee.triage_body_cue,
	ee.triage_state`

func strPtrOrNil(s string) *string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// BackfillSkippedEmailTriageData populates template signatures and draft fields
// for existing non_transaction emails lazily inside a single immediate write lock.
func BackfillSkippedEmailTriageData(gdb *gorm.DB) error {
	var needsWork int
	if err := gdb.Raw(`
		SELECT 1
		FROM raw_emails re
		LEFT JOIN email_extractions ee ON ee.email_id = re.id
		WHERE ` + unbackfilledCondition + `
		LIMIT 1`,
	).Scan(&needsWork).Error; err != nil {
		return err
	}
	if needsWork == 0 {
		return nil
	}

	return db.RunImmediateTx(gdb, func(tx *gorm.DB) error {
		type backfillRow struct {
			EmailID          string  `gorm:"column:email_id"`
			FromAddress      string  `gorm:"column:from_address"`
			Subject          string  `gorm:"column:subject"`
			CleanedText      string  `gorm:"column:cleaned_text"`
			ReceivedAtUtc    string  `gorm:"column:received_at_utc"`
			ExtractionID     *string `gorm:"column:extraction_id"`
			AmountMinor      *int64  `gorm:"column:amount_minor"`
			Currency         *string `gorm:"column:currency"`
			OccurredAtUtc    *string `gorm:"column:occurred_at_utc"`
			MerchantRaw      *string `gorm:"column:merchant_raw"`
			Category         *string `gorm:"column:category"`
			CardLast4        *string `gorm:"column:card_last4"`
			PaymentMethodRaw *string `gorm:"column:payment_method_raw"`
			BankAccountHint  *string `gorm:"column:bank_account_hint"`
			LlmAccountID     *string `gorm:"column:llm_account_id"`
			AccountTypeGuess *string `gorm:"column:account_type_guess"`
			ReferenceCode    *string `gorm:"column:reference_code"`
			ExtractionModel  *string `gorm:"column:extraction_model"`
			RawLlmJSON       *string `gorm:"column:raw_llm_json"`
			TriageState      *string `gorm:"column:triage_state"`
		}

		var candidates []backfillRow
		if err := tx.Raw(`
			SELECT
				re.id AS email_id, re.from_address, re.subject, re.cleaned_text, re.received_at_utc,
				ee.id AS extraction_id, ee.amount_minor, ee.currency, ee.occurred_at_utc,
				ee.merchant_raw, ee.category, ee.card_last4, ee.payment_method_raw,
				ee.bank_account_hint, ee.llm_account_id, ee.account_type_guess, ee.reference_code,
				ee.extraction_model, ee.raw_llm_json, ee.triage_state
			FROM raw_emails re
			LEFT JOIN email_extractions ee ON ee.email_id = re.id
			WHERE ` + unbackfilledCondition,
		).Scan(&candidates).Error; err != nil {
			return err
		}
		if len(candidates) == 0 {
			return nil
		}

		tzPtr, curPtr, err := getLocaleConfig(tx)
		if err != nil {
			return err
		}
		userTimezone := "UTC"
		if tzPtr != nil && strings.TrimSpace(*tzPtr) != "" && !strings.EqualFold(strings.TrimSpace(*tzPtr), "Local") {
			userTimezone = strings.TrimSpace(*tzPtr)
		}
		catNames, err := ListCategoryNames(tx)
		if err != nil {
			return err
		}

		for _, row := range candidates {
			sig := DeriveEmailTemplateSignature(row.FromAddress, row.Subject, row.CleanedText)

			amt := row.AmountMinor
			cur := row.Currency
			occ := row.OccurredAtUtc
			merchRaw := row.MerchantRaw
			var merchNorm *string
			if merchRaw != nil {
				if norm := strings.TrimSpace(NormalizeMerchant(*merchRaw)); norm != "" {
					merchNorm = &norm
				} else {
					merchRaw = nil
				}
			}
			cat := row.Category
			cardLast4 := row.CardLast4
			pmRaw := row.PaymentMethodRaw
			bankHint := row.BankAccountHint
			llmAcct := row.LlmAccountID
			acctType := row.AccountTypeGuess
			refCode := row.ReferenceCode

			rawJSON := "{}"
			if row.RawLlmJSON != nil && strings.TrimSpace(*row.RawLlmJSON) != "" {
				rawJSON = *row.RawLlmJSON
			}
			cleaned := llm.StripThinkBlocks(rawJSON)
			if strings.HasPrefix(cleaned, "{") {
				var parsed llm.RawExtraction
				if json.Unmarshal([]byte(cleaned), &parsed) == nil {
					parsed.EmailKind = "non_transaction"
					res, normErr := llm.NormalizeExtractedResult(&parsed, rawJSON, llm.ExtractInput{
						EmailID:       row.EmailID,
						FromAddress:   row.FromAddress,
						Subject:       row.Subject,
						CleanedText:   row.CleanedText,
						ReceivedAtUtc: row.ReceivedAtUtc,
						UserTimezone:  userTimezone,
						UserCurrency:  curPtr,
						Categories:    catNames,
					})
					if normErr == nil && res != nil {
						if res.AmountMinor > 0 && len(res.Currency) == 3 {
							a := res.AmountMinor
							c := res.Currency
							amt = &a
							cur = &c
							if res.OccurredAtUtc != "" {
								o := res.OccurredAtUtc
								occ = &o
							}
							resolvedCat := ResolveKnownCategory(tx, res.Category)
							cat = &resolvedCat
						}
						if merchRaw == nil {
							if p := strPtrOrNil(res.MerchantRaw); p != nil {
								merchRaw = p
								if norm := strings.TrimSpace(NormalizeMerchant(*p)); norm != "" {
									merchNorm = &norm
								}
							}
						}
						if cardLast4 == nil && res.CardLast4 != nil {
							cardLast4 = res.CardLast4
						}
						if pmRaw == nil && res.PaymentMethodRaw != nil {
							pmRaw = res.PaymentMethodRaw
						}
						if bankHint == nil && res.BankAccountHint != nil {
							bankHint = res.BankAccountHint
						}
						if llmAcct == nil && res.MatchedAccountID != nil {
							llmAcct = res.MatchedAccountID
						}
						if acctType == nil && res.AccountTypeGuess != nil {
							acctType = res.AccountTypeGuess
						}
						if refCode == nil && res.ReferenceCode != nil {
							refCode = res.ReferenceCode
						}
					}
				}
			}

			var triageState *string
			if row.TriageState != nil && *row.TriageState != "" {
				s := *row.TriageState
				triageState = &s
			} else if amt != nil && *amt > 0 && cur != nil && len(strings.TrimSpace(*cur)) == 3 {
				s := "ghost"
				triageState = &s
			}

			if row.ExtractionID == nil {
				if err := tx.Exec(`
					INSERT INTO email_extractions (
						id, email_id, email_kind, amount_minor, currency, occurred_at_utc,
						merchant_raw, merchant_norm, category, card_last4, payment_method_raw,
						bank_account_hint, llm_account_id, reference_code, extraction_model, raw_llm_json,
						account_type_guess, triage_signature_key, triage_subject_pattern, triage_body_cue, triage_state
					) VALUES (?, ?, 'non_transaction', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'legacy_backfill', ?, ?, ?, ?, ?, ?)`,
					uuid.NewString(), row.EmailID, amt, cur, occ,
					merchRaw, merchNorm, cat, cardLast4, pmRaw,
					bankHint, llmAcct, refCode, rawJSON,
					acctType, sig.SignatureKey, sig.SubjectPattern, sig.BodyCue, triageState,
				).Error; err != nil {
					return err
				}
			} else {
				if err := tx.Exec(`
					UPDATE email_extractions
					SET amount_minor = ?,
						currency = ?,
						occurred_at_utc = ?,
						merchant_raw = ?,
						merchant_norm = ?,
						category = ?,
						card_last4 = ?,
						payment_method_raw = ?,
						bank_account_hint = ?,
						llm_account_id = ?,
						reference_code = ?,
						account_type_guess = ?,
						triage_signature_key = ?,
						triage_subject_pattern = ?,
						triage_body_cue = ?,
						triage_state = ?
					WHERE id = ?`,
					amt, cur, occ,
					merchRaw, merchNorm, cat, cardLast4, pmRaw,
					bankHint, llmAcct, refCode,
					acctType, sig.SignatureKey, sig.SubjectPattern, sig.BodyCue, triageState,
					*row.ExtractionID,
				).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

const maxGhostRowsPerPage = 100

type ghostListQueryResult struct {
	Items              []GhostRowItem
	GhostInRangeCount  int64
	TotalGhostCount    int64
	GhostRowsTruncated bool
}

const eligibleGhostBase = `
		FROM raw_emails re
		JOIN email_extractions ee ON ee.email_id = re.id
		WHERE re.status = 'non_transaction'
		  AND ee.triage_state = 'ghost'
		  AND COALESCE(ee.extraction_model, '') != 'rule:billing_statement'
		  AND ee.amount_minor > 0
		  AND ee.currency IS NOT NULL
		  AND LENGTH(TRIM(ee.currency)) = 3`

func queryGhostRowsForTransactions(
	gdb *gorm.DB,
	params ListTransactionsParams,
	appCurrency string,
	rangeStartUtc string,
	rangeEndUtc string,
	totalTxCount int64,
	offset int,
	pageRowsCount int,
	prevPageOldestUtc string,
	currPageOldestUtc string,
) (*ghostListQueryResult, error) {
	const dateBounds = "COALESCE(ee.occurred_at_utc, re.received_at_utc) >= ? AND COALESCE(ee.occurred_at_utc, re.received_at_utc) < ?"
	inRangeCount := "COUNT(*)"
	var conds []string
	var args []any
	if rangeStartUtc != "" && rangeEndUtc != "" {
		inRangeCount += " FILTER (WHERE " + dateBounds + ")"
		conds = append(conds, dateBounds)
		args = append(args, rangeStartUtc, rangeEndUtc)
	}

	var counts struct {
		Total   int64
		InRange int64
	}
	if err := gdb.Raw(`SELECT COUNT(*) AS total, `+inRangeCount+` AS in_range `+eligibleGhostBase, args...).Scan(&counts).Error; err != nil {
		return nil, err
	}

	result := &ghostListQueryResult{
		Items:             []GhostRowItem{},
		GhostInRangeCount: counts.InRange,
		TotalGhostCount:   counts.Total,
	}
	showSkipped := params.ShowSkipped == nil || *params.ShowSkipped
	outsideLedgerPage := (totalTxCount > 0 && pageRowsCount == 0) || (totalTxCount == 0 && params.Page > 1)
	if !showSkipped || strings.TrimSpace(params.Tag) != "" || outsideLedgerPage {
		return result, nil
	}

	if params.Category != "" {
		conds = append(conds, "ee.category = ?")
		args = append(args, params.Category)
	}
	if q := strings.TrimSpace(params.Search); q != "" {
		like := "%" + strings.ToLower(q) + "%"
		conds = append(conds, `(
			LOWER(COALESCE(ee.merchant_raw, '')) LIKE ?
			OR LOWER(re.subject) LIKE ?
			OR LOWER(re.from_address) LIKE ?
			OR LOWER(COALESCE(ee.payment_method_raw, '')) LIKE ?
		)`)
		args = append(args, like, like, like, like)
	}

	if totalTxCount > 0 {
		if params.Page > 1 && prevPageOldestUtc != "" {
			conds = append(conds, "COALESCE(ee.occurred_at_utc, re.received_at_utc) < ?")
			args = append(args, prevPageOldestUtc)
		}
		isLastPage := int64(offset+pageRowsCount) >= totalTxCount
		if !isLastPage && currPageOldestUtc != "" {
			conds = append(conds, "COALESCE(ee.occurred_at_utc, re.received_at_utc) >= ?")
			args = append(args, currPageOldestUtc)
		}
	}

	extraWhere := ""
	if len(conds) > 0 {
		extraWhere = " AND " + strings.Join(conds, " AND ")
	}

	rawRows, err := readGhostRows(gdb, extraWhere, args, 0, 0)
	if err != nil {
		return nil, err
	}
	out, truncated, err := mapGhostRows(gdb, rawRows, params.AccountID, appCurrency)
	if err != nil {
		return nil, err
	}
	result.Items = out
	result.GhostRowsTruncated = truncated
	return result, nil
}

type rawGhostRow struct {
	EmailID              string  `gorm:"column:email_id"`
	SenderID             *string `gorm:"column:sender_id"`
	FromAddress          string  `gorm:"column:from_address"`
	Subject              string  `gorm:"column:subject"`
	AmountMinor          int64   `gorm:"column:amount_minor"`
	Currency             string  `gorm:"column:currency"`
	OccurredAtUtc        string  `gorm:"column:effective_occurred_at_utc"`
	MerchantRaw          *string `gorm:"column:merchant_raw"`
	Category             *string `gorm:"column:category"`
	CardLast4            *string `gorm:"column:card_last4"`
	BankAccountHint      *string `gorm:"column:bank_account_hint"`
	PaymentMethodRaw     *string `gorm:"column:payment_method_raw"`
	LlmAccountID         *string `gorm:"column:llm_account_id"`
	TriageSignatureKey   string  `gorm:"column:triage_signature_key"`
	TriageSubjectPattern string  `gorm:"column:triage_subject_pattern"`
	TriageBodyCue        string  `gorm:"column:triage_body_cue"`
}

func readGhostRows(gdb *gorm.DB, extraWhere string, args []any, limit, offset int) ([]rawGhostRow, error) {
	pageClause := ""
	if limit > 0 {
		pageClause = " LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	}

	var rawRows []rawGhostRow
	if err := gdb.Raw(`
		SELECT
			re.id AS email_id,
			re.sender_id,
			re.from_address,
			re.subject,
			ee.amount_minor,
			UPPER(TRIM(ee.currency)) AS currency,
			COALESCE(ee.occurred_at_utc, re.received_at_utc) AS effective_occurred_at_utc,
			NULLIF(TRIM(COALESCE(ee.merchant_raw, '')), '') AS merchant_raw,
			ee.category,
			ee.card_last4,
			ee.bank_account_hint,
			ee.payment_method_raw,
			ee.llm_account_id,
			COALESCE(ee.triage_signature_key, '') AS triage_signature_key,
			COALESCE(ee.triage_subject_pattern, '') AS triage_subject_pattern,
			COALESCE(ee.triage_body_cue, '') AS triage_body_cue
		`+eligibleGhostBase+extraWhere+`
		ORDER BY effective_occurred_at_utc DESC, re.internal_date_ms DESC, re.id DESC`+pageClause,
		args...,
	).Scan(&rawRows).Error; err != nil {
		return nil, err
	}

	return rawRows, nil
}

func mapGhostRows(gdb *gorm.DB, rawRows []rawGhostRow, accountFilter, appCurrency string) ([]GhostRowItem, bool, error) {
	if len(rawRows) == 0 {
		return []GhostRowItem{}, false, nil
	}

	type acctNameRow struct {
		ID   string `gorm:"column:id"`
		Name string `gorm:"column:name"`
	}
	var acctRows []acctNameRow
	if err := gdb.Raw(`SELECT id, name FROM accounts`).Scan(&acctRows).Error; err != nil {
		return nil, false, err
	}
	acctNameByID := make(map[string]string, len(acctRows))
	for _, a := range acctRows {
		acctNameByID[a.ID] = a.Name
	}

	out := make([]GhostRowItem, 0, len(rawRows))
	truncated := false
	for _, r := range rawRows {
		resolvedAcctID := MatchAccountForExtraction(gdb, MatchAccountParams{
			SenderID:        r.SenderID,
			CardLast4:       r.CardLast4,
			BankAccountHint: r.BankAccountHint,
			PaymentMethod:   r.PaymentMethodRaw,
			LlmAccountID:    r.LlmAccountID,
		})
		if accountFilter != "" && accountFilter != "all" {
			if accountFilter == "unassigned" {
				if resolvedAcctID != nil {
					continue
				}
			} else if resolvedAcctID == nil || *resolvedAcctID != accountFilter {
				continue
			}
		}

		if len(out) >= maxGhostRowsPerPage {
			truncated = true
			break
		}

		var resolvedAcctName *string
		if resolvedAcctID != nil {
			if name, ok := acctNameByID[*resolvedAcctID]; ok {
				n := name
				resolvedAcctName = &n
			}
		}

		out = append(out, GhostRowItem{
			EmailID:              r.EmailID,
			FromAddress:          r.FromAddress,
			SenderEmail:          NormalizeSenderEmail(r.FromAddress),
			Subject:              r.Subject,
			AmountMinor:          r.AmountMinor,
			Currency:             r.Currency,
			OccurredAtUtc:        r.OccurredAtUtc,
			MerchantRaw:          r.MerchantRaw,
			Category:             r.Category,
			AccountID:            resolvedAcctID,
			AccountName:          resolvedAcctName,
			PaymentMethodRaw:     r.PaymentMethodRaw,
			TriageSignatureKey:   r.TriageSignatureKey,
			TriageSubjectPattern: r.TriageSubjectPattern,
			TriageBodyCue:        r.TriageBodyCue,
			IsForeignCurrency:    appCurrency != "" && r.Currency != appCurrency,
		})
	}

	return out, truncated, nil
}

type PaginatedSkippedTransactionsResponse struct {
	Items      []GhostRowItem `json:"items"`
	TotalCount int64          `json:"totalCount"`
	Page       int            `json:"page"`
	PageSize   int            `json:"pageSize"`
}

func ListSkippedTransactions(gdb *gorm.DB, page, pageSize int) (*PaginatedSkippedTransactionsResponse, error) {
	return ListSkippedTransactionsWithBackfill(gdb, page, pageSize, nil)
}

func ListSkippedTransactionsWithBackfill(gdb *gorm.DB, page, pageSize int, backfill *TriageBackfill) (*PaginatedSkippedTransactionsResponse, error) {
	page = max(1, page)
	if pageSize <= 0 {
		pageSize = 25
	}
	pageSize = min(pageSize, maxGhostRowsPerPage)

	result := PaginatedSkippedTransactionsResponse{
		Items: []GhostRowItem{}, Page: page, PageSize: pageSize,
	}
	timezone, currency, err := getLocaleConfig(gdb)
	if err != nil {
		return nil, err
	}
	if timezone == nil || *timezone == "" || currency == nil || *currency == "" {
		return &result, nil
	}
	if err := backfill.ensure(gdb); err != nil {
		return nil, err
	}

	var rows []rawGhostRow
	err = gdb.Transaction(func(tx *gorm.DB) error {
		if err := tx.Raw(`SELECT COUNT(*) ` + eligibleGhostBase).Scan(&result.TotalCount).Error; err != nil {
			return err
		}
		if result.TotalCount == 0 || int64(page-1) > (result.TotalCount-1)/int64(pageSize) {
			return nil
		}
		var err error
		rows, err = readGhostRows(tx, "", nil, pageSize, (page-1)*pageSize)
		return err
	})
	if err != nil {
		return nil, err
	}
	result.Items, _, err = mapGhostRows(gdb, rows, "", *currency)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

type triageCandidateRow struct {
	EmailID              string  `gorm:"column:email_id"`
	SenderID             *string `gorm:"column:sender_id"`
	SenderRoleHint       *string `gorm:"column:sender_role_hint"`
	FromAddress          string  `gorm:"column:from_address"`
	Subject              string  `gorm:"column:subject"`
	CleanedText          string  `gorm:"column:cleaned_text"`
	ReceivedAtUtc        string  `gorm:"column:received_at_utc"`
	RawStatus            string  `gorm:"column:raw_status"`
	EmailKind            string  `gorm:"column:email_kind"`
	AmountMinor          *int64  `gorm:"column:amount_minor"`
	Currency             *string `gorm:"column:currency"`
	OccurredAtUtc        *string `gorm:"column:occurred_at_utc"`
	MerchantRaw          *string `gorm:"column:merchant_raw"`
	Category             *string `gorm:"column:category"`
	CardLast4            *string `gorm:"column:card_last4"`
	PaymentMethodRaw     *string `gorm:"column:payment_method_raw"`
	BankAccountHint      *string `gorm:"column:bank_account_hint"`
	LlmAccountID         *string `gorm:"column:llm_account_id"`
	AccountTypeGuess     *string `gorm:"column:account_type_guess"`
	ReferenceCode        *string `gorm:"column:reference_code"`
	ExtractionModel      string  `gorm:"column:extraction_model"`
	TriageSignatureKey   *string `gorm:"column:triage_signature_key"`
	TriageSubjectPattern *string `gorm:"column:triage_subject_pattern"`
	TriageBodyCue        *string `gorm:"column:triage_body_cue"`
	TriageState          *string `gorm:"column:triage_state"`
}

func isWithinRangeUtc(tsUtc, startUtc, endUtc string) bool {
	if startUtc == "" || endUtc == "" {
		return true
	}
	return tsUtc >= startUtc && tsUtc < endUtc
}

func admitPromotedTransactionInTx(
	tx *gorm.DB,
	emailID string,
	senderID *string,
	fromAddress string,
	effectiveRole string,
	amountMinor int64,
	currency string,
	occurredAt string,
	merchant string,
	category string,
	cardLast4 *string,
	bankHint *string,
	pmRaw *string,
	llmAcctID *string,
	acctTypeGuess *string,
	refCode *string,
	nowUtc string,
) (txID string, createdProvAccountID *string, err error) {
	acctID := MatchAccountForExtraction(tx, MatchAccountParams{
		SenderID:        senderID,
		CardLast4:       cardLast4,
		BankAccountHint: bankHint,
		PaymentMethod:   pmRaw,
		LlmAccountID:    llmAcctID,
	})
	if acctID == nil {
		prov, provErr := CreateProvisionalAccountForExtractionInTx(tx, ProvisionalAccountParams{
			FromAddress:      fromAddress,
			CardLast4:        cardLast4,
			BankAccountHint:  bankHint,
			PaymentMethod:    pmRaw,
			AccountTypeGuess: acctTypeGuess,
			Currency:         &currency,
		})
		if provErr != nil {
			return "", nil, provErr
		}
		if prov != nil {
			id := prov.ID
			acctID = &id
			createdProvAccountID = &id
		}
	}

	txID, err = SaveExtractedTransactionInTx(tx, ExtractedTransactionInput{
		EmailID:           emailID,
		EmailKind:         effectiveRole,
		AmountMinor:       amountMinor,
		Currency:          currency,
		OccurredAtUtc:     occurredAt,
		MerchantRaw:       merchant,
		Category:          category,
		CardLast4:         cardLast4,
		ReferenceCode:     refCode,
		ResolvedAccountID: acctID,
		NowUtc:            nowUtc,
	})
	if err != nil {
		return "", nil, err
	}

	if err := tx.Model(&db.RawEmail{}).
		Where("id = ?", emailID).
		Updates(map[string]any{"status": "extracted", "error_message": nil}).Error; err != nil {
		return "", nil, err
	}
	return txID, createdProvAccountID, nil
}

func ApplyTriageVerdict(gdb *gorm.DB, input ApplyTriageVerdictInput) (*TriageVerdictResult, error) {
	emailID := strings.TrimSpace(input.EmailID)
	if emailID == "" {
		return nil, &ValidationError{Message: "emailId is required."}
	}
	var targetPolicyAction string
	switch strings.TrimSpace(input.Action) {
	case "promote":
		targetPolicyAction = "admit"
	case "ignore":
		targetPolicyAction = "ignore"
	default:
		return nil, &ValidationError{Message: "action must be 'promote' or 'ignore'."}
	}

	tzPtr, _, err := getLocaleConfig(gdb)
	if err != nil {
		return nil, err
	}
	tz := "UTC"
	if tzPtr != nil && strings.TrimSpace(*tzPtr) != "" {
		tz = strings.TrimSpace(*tzPtr)
	}
	var rangeStartUtc, rangeEndUtc string
	if strings.TrimSpace(input.StartDate) != "" || strings.TrimSpace(input.EndDate) != "" {
		rangeStartUtc, rangeEndUtc, err = GetDateRangeUtcBounds(input.StartDate, input.EndDate, tz)
		if err != nil {
			return nil, &ValidationError{Message: err.Error()}
		}
	}

	var result *TriageVerdictResult
	err = db.RunImmediateTx(gdb, func(tx *gorm.DB) error {
		var clicked []triageCandidateRow
		if err := tx.Raw(`
			SELECT`+triageCandidateColumns+`
			FROM raw_emails re
			LEFT JOIN senders s ON s.id = re.sender_id
			LEFT JOIN email_extractions ee ON ee.email_id = re.id
			WHERE re.id = ?
			LIMIT 1`, emailID,
		).Scan(&clicked).Error; err != nil {
			return err
		}
		if len(clicked) == 0 {
			return &NotFoundError{Message: "Email not found."}
		}
		target := clicked[0]
		if target.RawStatus != "non_transaction" ||
			target.TriageState == nil ||
			*target.TriageState != "ghost" ||
			target.ExtractionModel == "rule:billing_statement" ||
			target.AmountMinor == nil ||
			*target.AmountMinor <= 0 ||
			target.Currency == nil ||
			len(strings.TrimSpace(*target.Currency)) != 3 {
			return &ConflictError{Message: "Email is no longer an eligible unjudged skipped item."}
		}

		sig := DeriveEmailTemplateSignature(target.FromAddress, target.Subject, target.CleanedText)
		if target.TriageSignatureKey != nil && strings.TrimSpace(*target.TriageSignatureKey) != "" {
			sig.SignatureKey = strings.TrimSpace(*target.TriageSignatureKey)
		}
		if target.TriageSubjectPattern != nil && strings.TrimSpace(*target.TriageSubjectPattern) != "" {
			sig.SubjectPattern = strings.TrimSpace(*target.TriageSubjectPattern)
		}
		if target.TriageBodyCue != nil && *target.TriageBodyCue != "" {
			sig.BodyCue = *target.TriageBodyCue
		}

		nowUtc := NowUtcIsoSeconds()
		var existingPolicy db.SenderTriagePolicy
		prior := TriagePriorPolicyState{}
		policyErr := tx.Where("sender_email = ? AND signature_key = ?", sig.SenderEmail, sig.SignatureKey).First(&existingPolicy).Error
		if policyErr == nil {
			prior.Existed = true
			prior.Action = existingPolicy.Action
			if err := tx.Model(&db.SenderTriagePolicy{}).Where("id = ?", existingPolicy.ID).Updates(map[string]any{
				"subject_pattern": sig.SubjectPattern,
				"body_cue":        sig.BodyCue,
				"action":          targetPolicyAction,
				"updated_at_utc":  nowUtc,
			}).Error; err != nil {
				return err
			}
			existingPolicy.SubjectPattern = sig.SubjectPattern
			existingPolicy.BodyCue = sig.BodyCue
			existingPolicy.Action = targetPolicyAction
			existingPolicy.UpdatedAtUtc = nowUtc
		} else if errors.Is(policyErr, gorm.ErrRecordNotFound) {
			existingPolicy = db.SenderTriagePolicy{
				ID:             uuid.NewString(),
				SenderEmail:    sig.SenderEmail,
				SignatureKey:   sig.SignatureKey,
				SubjectPattern: sig.SubjectPattern,
				BodyCue:        sig.BodyCue,
				Action:         targetPolicyAction,
				CreatedAtUtc:   nowUtc,
				UpdatedAtUtc:   nowUtc,
			}
			if err := tx.Create(&existingPolicy).Error; err != nil {
				return err
			}
		} else {
			return policyErr
		}

		var rawSiblings []triageCandidateRow
		if err := tx.Raw(`
			SELECT`+triageCandidateColumns+`
			FROM raw_emails re
			LEFT JOIN senders s ON s.id = re.sender_id
			JOIN email_extractions ee ON ee.email_id = re.id
			WHERE re.status = 'non_transaction'
			  AND ee.triage_signature_key = ?
			  AND ee.triage_state = 'ghost'
			  AND COALESCE(ee.extraction_model, '') != 'rule:billing_statement'
			  AND ee.amount_minor > 0
			  AND ee.currency IS NOT NULL
			  AND LENGTH(TRIM(ee.currency)) = 3
			ORDER BY re.internal_date_ms ASC, re.id ASC`,
			sig.SignatureKey,
		).Scan(&rawSiblings).Error; err != nil {
			return err
		}

		affectedEmailIDs := make([]string, 0, len(rawSiblings))
		createdTxIDs := make([]string, 0, len(rawSiblings))
		createdProvAccountIDs := make([]string, 0)
		outOfRangeCount := 0

		for _, sib := range rawSiblings {
			if NormalizeSenderEmail(sib.FromAddress) != sig.SenderEmail {
				continue
			}
			affectedEmailIDs = append(affectedEmailIDs, sib.EmailID)

			if input.Action == "ignore" {
				if err := tx.Model(&db.EmailExtraction{}).
					Where("email_id = ?", sib.EmailID).
					Update("triage_state", "ignored").Error; err != nil {
					return err
				}
				effOcc := sib.ReceivedAtUtc
				if sib.OccurredAtUtc != nil && IsValidUtcIsoSeconds(strings.TrimSpace(*sib.OccurredAtUtc)) {
					effOcc = strings.TrimSpace(*sib.OccurredAtUtc)
				}
				if !isWithinRangeUtc(effOcc, rangeStartUtc, rangeEndUtc) {
					outOfRangeCount++
				}
				continue
			}

			occurredAt := sib.ReceivedAtUtc
			if sib.OccurredAtUtc != nil && IsValidUtcIsoSeconds(strings.TrimSpace(*sib.OccurredAtUtc)) {
				occurredAt = strings.TrimSpace(*sib.OccurredAtUtc)
			}
			currency := strings.ToUpper(strings.TrimSpace(*sib.Currency))
			merchant := ""
			if sib.MerchantRaw != nil {
				merchant = strings.TrimSpace(*sib.MerchantRaw)
			}
			if merchant == "" {
				merchant = strings.TrimSpace(sib.Subject)
			}
			if merchant == "" {
				merchant = "Unknown Merchant"
			}
			rawCat := ""
			if sib.Category != nil {
				rawCat = *sib.Category
			}
			category := ResolveKnownCategory(tx, rawCat)
			effectiveRole := ResolveEffectiveEmailRole(sib.SenderRoleHint, sib.EmailKind, sib.CardLast4, sib.BankAccountHint)

			txID, createdProvID, saveErr := admitPromotedTransactionInTx(
				tx, sib.EmailID, sib.SenderID, sib.FromAddress, effectiveRole,
				*sib.AmountMinor, currency, occurredAt, merchant, category,
				sib.CardLast4, sib.BankAccountHint, sib.PaymentMethodRaw, sib.LlmAccountID, sib.AccountTypeGuess, sib.ReferenceCode, nowUtc,
			)
			if saveErr != nil {
				return saveErr
			}
			if createdProvID != nil {
				createdProvAccountIDs = append(createdProvAccountIDs, *createdProvID)
			}
			createdTxIDs = append(createdTxIDs, txID)

			if err := tx.Model(&db.EmailExtraction{}).
				Where("email_id = ?", sib.EmailID).
				Update("triage_state", "promoted").Error; err != nil {
				return err
			}
			if !isWithinRangeUtc(occurredAt, rangeStartUtc, rangeEndUtc) {
				outOfRangeCount++
			}
		}

		policyView, err := buildSenderTriagePolicyView(tx, existingPolicy)
		if err != nil {
			return err
		}

		result = &TriageVerdictResult{
			Policy:                policyView,
			AffectedEmailIDs:      affectedEmailIDs,
			CreatedTransactionIDs: createdTxIDs,
			AffectedCount:         len(affectedEmailIDs),
			OutOfRangeCount:       outOfRangeCount,
			UndoPayload: TriageUndoPayload{
				Action:                       input.Action,
				SenderEmail:                  sig.SenderEmail,
				SignatureKey:                 sig.SignatureKey,
				SubjectPattern:               sig.SubjectPattern,
				BodyCue:                      sig.BodyCue,
				PriorPolicy:                  prior,
				AffectedEmailIDs:             affectedEmailIDs,
				CreatedTransactionIDs:        createdTxIDs,
				CreatedProvisionalAccountIDs: createdProvAccountIDs,
			},
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func UndoTriageVerdict(gdb *gorm.DB, input TriageUndoInput) (*TriageUndoResult, error) {
	payload := input.UndoPayload
	if payload.Action != "promote" && payload.Action != "ignore" {
		return nil, &ValidationError{Message: "Invalid undo action."}
	}
	senderEmail := strings.ToLower(strings.TrimSpace(payload.SenderEmail))
	sigKey := strings.TrimSpace(payload.SignatureKey)
	if senderEmail == "" || sigKey == "" || len(payload.AffectedEmailIDs) == 0 {
		return nil, &ValidationError{Message: "Incomplete triage undo payload."}
	}

	expectedPolicyAction := "admit"
	expectedEmailStatus := "extracted"
	expectedTriageState := "promoted"
	if payload.Action == "ignore" {
		expectedPolicyAction = "ignore"
		expectedEmailStatus = "non_transaction"
		expectedTriageState = "ignored"
	}

	var restoredCount int
	err := db.RunImmediateTx(gdb, func(tx *gorm.DB) error {
		var policy db.SenderTriagePolicy
		if err := tx.Where("sender_email = ? AND signature_key = ?", senderEmail, sigKey).First(&policy).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return &ConflictError{Message: "Triage policy was removed or changed; cannot undo."}
			}
			return err
		}
		if policy.Action != expectedPolicyAction {
			return &ConflictError{Message: "Triage policy was changed after this verdict; cannot undo."}
		}

		type emailStateRow struct {
			EmailID     string  `gorm:"column:email_id"`
			Status      string  `gorm:"column:status"`
			TriageState *string `gorm:"column:triage_state"`
		}
		var emailStates []emailStateRow
		if err := tx.Raw(`
			SELECT re.id AS email_id, re.status, ee.triage_state
			FROM raw_emails re
			JOIN email_extractions ee ON ee.email_id = re.id
			WHERE re.id IN ?`, payload.AffectedEmailIDs,
		).Scan(&emailStates).Error; err != nil {
			return err
		}
		if len(emailStates) != len(payload.AffectedEmailIDs) {
			return &ConflictError{Message: "One or more affected emails no longer exist; cannot undo."}
		}
		for _, es := range emailStates {
			if es.Status != expectedEmailStatus || es.TriageState == nil || *es.TriageState != expectedTriageState {
				return &ConflictError{Message: "One or more affected emails were modified after this verdict; cannot undo."}
			}
		}

		if payload.Action == "promote" {
			if len(payload.CreatedTransactionIDs) != len(payload.AffectedEmailIDs) {
				return &ConflictError{Message: "Promoted transaction list does not match affected emails; cannot undo."}
			}
			var txs []db.Transaction
			if err := tx.Where("id IN ?", payload.CreatedTransactionIDs).Find(&txs).Error; err != nil {
				return err
			}
			if len(txs) != len(payload.CreatedTransactionIDs) {
				return &ConflictError{Message: "One or more promoted transactions were already deleted; cannot undo."}
			}
			for _, tRow := range txs {
				if tRow.ManualOverride != 0 || tRow.Reviewed != 0 || tRow.RemarkUserEdited {
					return &ConflictError{Message: "Promoted transactions were edited or reviewed; cannot undo."}
				}
			}

			var srcRows []db.TransactionSource
			if err := tx.Where("transaction_id IN ?", payload.CreatedTransactionIDs).Find(&srcRows).Error; err != nil {
				return err
			}
			if len(srcRows) != len(payload.CreatedTransactionIDs) {
				return &ConflictError{Message: "Promoted transactions were merged or split; cannot undo."}
			}
			allowedEmails := make(map[string]struct{}, len(payload.AffectedEmailIDs))
			for _, id := range payload.AffectedEmailIDs {
				allowedEmails[id] = struct{}{}
			}
			txSeen := make(map[string]int, len(payload.CreatedTransactionIDs))
			for _, src := range srcRows {
				if _, ok := allowedEmails[src.EmailID]; !ok {
					return &ConflictError{Message: "Promoted transaction sources were modified; cannot undo."}
				}
				txSeen[src.TransactionID]++
				if txSeen[src.TransactionID] > 1 {
					return &ConflictError{Message: "Promoted transaction was merged with another email; cannot undo."}
				}
			}

			if err := tx.Where("id IN ?", payload.CreatedTransactionIDs).Delete(&db.Transaction{}).Error; err != nil {
				return err
			}
		}

		if err := tx.Model(&db.RawEmail{}).
			Where("id IN ?", payload.AffectedEmailIDs).
			Updates(map[string]any{"status": "non_transaction", "error_message": nil}).Error; err != nil {
			return err
		}
		if err := tx.Model(&db.EmailExtraction{}).
			Where("email_id IN ?", payload.AffectedEmailIDs).
			Update("triage_state", "ghost").Error; err != nil {
			return err
		}

		for _, acctID := range payload.CreatedProvisionalAccountIDs {
			var acct db.Account
			if err := tx.Select("id, status").Where("id = ?", acctID).First(&acct).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					continue
				}
				return err
			}
			if acct.Status != "provisional" {
				continue
			}
			var txRefCnt, senderRefCnt int64
			if err := tx.Model(&db.Transaction{}).Where("account_id = ?", acctID).Count(&txRefCnt).Error; err != nil {
				return err
			}
			if err := tx.Model(&db.Sender{}).Where("default_account_id = ?", acctID).Count(&senderRefCnt).Error; err != nil {
				return err
			}
			if txRefCnt == 0 && senderRefCnt == 0 {
				if err := tx.Where("id = ?", acctID).Delete(&db.Account{}).Error; err != nil {
					return err
				}
			}
		}

		if !payload.PriorPolicy.Existed {
			if err := tx.Where("id = ?", policy.ID).Delete(&db.SenderTriagePolicy{}).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&db.SenderTriagePolicy{}).Where("id = ?", policy.ID).Updates(map[string]any{
				"action":         payload.PriorPolicy.Action,
				"updated_at_utc": NowUtcIsoSeconds(),
			}).Error; err != nil {
				return err
			}
		}

		restoredCount = len(payload.AffectedEmailIDs)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &TriageUndoResult{Ok: true, RestoredCount: restoredCount}, nil
}

func buildSenderTriagePolicyView(tx *gorm.DB, pol db.SenderTriagePolicy) (SenderTriagePolicyView, error) {
	type stateCountRow struct {
		FromAddress string  `gorm:"column:from_address"`
		TriageState *string `gorm:"column:triage_state"`
	}
	var rows []stateCountRow
	if err := tx.Raw(`
		SELECT re.from_address, ee.triage_state
		FROM raw_emails re
		JOIN email_extractions ee ON ee.email_id = re.id
		WHERE re.status = 'non_transaction'
		  AND ee.triage_signature_key = ?
		  AND ee.triage_state IN ('ignored', 'ghost')`,
		pol.SignatureKey,
	).Scan(&rows).Error; err != nil {
		return SenderTriagePolicyView{}, err
	}
	var ignoredCnt, ghostCnt int64
	for _, r := range rows {
		if NormalizeSenderEmail(r.FromAddress) != pol.SenderEmail || r.TriageState == nil {
			continue
		}
		if *r.TriageState == "ignored" {
			ignoredCnt++
		} else if *r.TriageState == "ghost" {
			ghostCnt++
		}
	}
	return SenderTriagePolicyView{
		ID:             pol.ID,
		SenderEmail:    pol.SenderEmail,
		SignatureKey:   pol.SignatureKey,
		SubjectPattern: pol.SubjectPattern,
		BodyCue:        pol.BodyCue,
		Action:         pol.Action,
		IgnoredCount:   ignoredCnt,
		GhostCount:     ghostCnt,
		CreatedAtUtc:   pol.CreatedAtUtc,
		UpdatedAtUtc:   pol.UpdatedAtUtc,
	}, nil
}

func ListTriagePolicies(gdb *gorm.DB, params ListTriagePoliciesParams) ([]SenderTriagePolicyView, error) {
	var policies []db.SenderTriagePolicy
	q := gdb.Model(&db.SenderTriagePolicy{})
	if s := strings.TrimSpace(params.SenderEmail); s != "" {
		q = q.Where("sender_email = ?", NormalizeSenderEmail(s))
	}
	if search := strings.TrimSpace(params.Search); search != "" {
		like := "%" + strings.ToLower(search) + "%"
		q = q.Where("(LOWER(sender_email) LIKE ? OR LOWER(subject_pattern) LIKE ? OR LOWER(body_cue) LIKE ?)", like, like, like)
	}
	if err := q.Order("updated_at_utc DESC, sender_email ASC").Find(&policies).Error; err != nil {
		return nil, err
	}
	if len(policies) == 0 {
		return []SenderTriagePolicyView{}, nil
	}

	sigKeys := make([]string, 0, len(policies))
	seenSig := make(map[string]struct{}, len(policies))
	for _, pol := range policies {
		if _, ok := seenSig[pol.SignatureKey]; !ok {
			seenSig[pol.SignatureKey] = struct{}{}
			sigKeys = append(sigKeys, pol.SignatureKey)
		}
	}

	type batchStateRow struct {
		SignatureKey string  `gorm:"column:triage_signature_key"`
		FromAddress  string  `gorm:"column:from_address"`
		TriageState  *string `gorm:"column:triage_state"`
	}
	var stateRows []batchStateRow
	if err := gdb.Raw(`
		SELECT ee.triage_signature_key, re.from_address, ee.triage_state
		FROM raw_emails re
		JOIN email_extractions ee ON ee.email_id = re.id
		WHERE re.status = 'non_transaction'
		  AND ee.triage_signature_key IN ?
		  AND ee.triage_state IN ('ignored', 'ghost')`,
		sigKeys,
	).Scan(&stateRows).Error; err != nil {
		return nil, err
	}

	type policyStateCounts struct {
		ignored int64
		ghost   int64
	}
	countsByKey := make(map[string]policyStateCounts, len(policies))
	for _, r := range stateRows {
		if r.TriageState == nil {
			continue
		}
		k := NormalizeSenderEmail(r.FromAddress) + "\n" + r.SignatureKey
		c := countsByKey[k]
		if *r.TriageState == "ignored" {
			c.ignored++
		} else if *r.TriageState == "ghost" {
			c.ghost++
		}
		countsByKey[k] = c
	}

	out := make([]SenderTriagePolicyView, 0, len(policies))
	for _, pol := range policies {
		c := countsByKey[pol.SenderEmail+"\n"+pol.SignatureKey]
		out = append(out, SenderTriagePolicyView{
			ID:             pol.ID,
			SenderEmail:    pol.SenderEmail,
			SignatureKey:   pol.SignatureKey,
			SubjectPattern: pol.SubjectPattern,
			BodyCue:        pol.BodyCue,
			Action:         pol.Action,
			IgnoredCount:   c.ignored,
			GhostCount:     c.ghost,
			CreatedAtUtc:   pol.CreatedAtUtc,
			UpdatedAtUtc:   pol.UpdatedAtUtc,
		})
	}
	return out, nil
}

func restorePostLlmIgnoredToGhostInTx(tx *gorm.DB, senderEmail, signatureKey string) (int64, error) {
	type restoreRow struct {
		EmailID     string `gorm:"column:email_id"`
		FromAddress string `gorm:"column:from_address"`
	}
	var candidates []restoreRow
	if err := tx.Raw(`
		SELECT re.id AS email_id, re.from_address
		FROM raw_emails re
		JOIN email_extractions ee ON ee.email_id = re.id
		WHERE re.status = 'non_transaction'
		  AND ee.triage_signature_key = ?
		  AND ee.triage_state = 'ignored'
		  AND COALESCE(ee.extraction_model, '') != 'policy:ignore'
		  AND ee.amount_minor > 0
		  AND ee.currency IS NOT NULL
		  AND LENGTH(TRIM(ee.currency)) = 3`,
		signatureKey,
	).Scan(&candidates).Error; err != nil {
		return 0, err
	}

	matchingIDs := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if NormalizeSenderEmail(c.FromAddress) == senderEmail {
			matchingIDs = append(matchingIDs, c.EmailID)
		}
	}
	if len(matchingIDs) == 0 {
		return 0, nil
	}
	res := tx.Model(&db.EmailExtraction{}).Where("email_id IN ?", matchingIDs).Update("triage_state", "ghost")
	return res.RowsAffected, res.Error
}

func UpdateTriagePolicy(gdb *gorm.DB, id string, input UpdateTriagePolicyInput) (*SenderTriagePolicyView, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, &ValidationError{Message: "Policy ID is required."}
	}
	action := strings.TrimSpace(input.Action)
	if action != "admit" && action != "ignore" {
		return nil, &ValidationError{Message: "action must be 'admit' or 'ignore'."}
	}

	var result SenderTriagePolicyView
	err := db.RunImmediateTx(gdb, func(tx *gorm.DB) error {
		var pol db.SenderTriagePolicy
		if err := tx.Where("id = ?", id).First(&pol).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return &NotFoundError{Message: "Triage policy not found."}
			}
			return err
		}
		var restored int64
		if pol.Action == "ignore" && action == "admit" {
			var rErr error
			restored, rErr = restorePostLlmIgnoredToGhostInTx(tx, pol.SenderEmail, pol.SignatureKey)
			if rErr != nil {
				return rErr
			}
		}
		nowUtc := NowUtcIsoSeconds()
		if err := tx.Model(&db.SenderTriagePolicy{}).Where("id = ?", id).Updates(map[string]any{
			"action":         action,
			"updated_at_utc": nowUtc,
		}).Error; err != nil {
			return err
		}
		pol.Action = action
		pol.UpdatedAtUtc = nowUtc
		view, err := buildSenderTriagePolicyView(tx, pol)
		if err != nil {
			return err
		}
		view.RestoredGhostCount = restored
		result = view
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func DeleteTriagePolicy(gdb *gorm.DB, id string) (*DeleteTriagePolicyResult, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, &ValidationError{Message: "Policy ID is required."}
	}
	var restored int64
	err := db.RunImmediateTx(gdb, func(tx *gorm.DB) error {
		var pol db.SenderTriagePolicy
		if err := tx.Where("id = ?", id).First(&pol).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return &NotFoundError{Message: "Triage policy not found."}
			}
			return err
		}
		if pol.Action == "ignore" {
			var rErr error
			restored, rErr = restorePostLlmIgnoredToGhostInTx(tx, pol.SenderEmail, pol.SignatureKey)
			if rErr != nil {
				return rErr
			}
		}
		return tx.Where("id = ?", id).Delete(&db.SenderTriagePolicy{}).Error
	})
	if err != nil {
		return nil, err
	}
	return &DeleteTriagePolicyResult{Ok: true, RestoredGhostCount: restored}, nil
}

// SQLite's built-in lower() folds ASCII only. Use the domain's sender
// normalization and Go's Unicode case folding for the existing search rules.
func init() {
	sqlite.MustRegisterDeterministicScalarFunction("spender_normalized_sender", 1,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			return NormalizeSenderEmail(args[0].(string)), nil
		})
	sqlite.MustRegisterDeterministicScalarFunction("spender_unicode_lower", 1,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			return strings.ToLower(args[0].(string)), nil
		})
}

const ignoredEmailBase = `
	FROM raw_emails re
	JOIN email_extractions ee ON ee.email_id = re.id
	WHERE re.status = 'non_transaction'
	  AND ee.triage_state = 'ignored'`

func ListIgnoredEmails(gdb *gorm.DB, params ListIgnoredEmailsParams) (*PaginatedIgnoredEmailsResponse, error) {
	pageSize := params.PageSize
	if pageSize <= 0 {
		pageSize = 25
	}

	result := PaginatedIgnoredEmailsResponse{
		Items:            []IgnoredEmailItem{},
		Page:             max(1, params.Page),
		PageSize:         min(100, pageSize),
		AvailableSenders: []string{},
	}
	base := ignoredEmailBase
	var args []any
	if sender := NormalizeSenderEmail(params.SenderEmail); sender != "" {
		base += " AND spender_normalized_sender(re.from_address) = ?"
		args = append(args, sender)
	}
	if search := strings.ToLower(strings.TrimSpace(params.Search)); search != "" {
		base += ` AND instr(spender_unicode_lower(
			re.subject || char(10) || re.from_address || char(10) ||
			TRIM(COALESCE(ee.merchant_raw, '')) || char(10) ||
			COALESCE(ee.triage_subject_pattern, '') || char(10) ||
			COALESCE(ee.triage_body_cue, '')), ?) > 0`
		args = append(args, search)
	}

	err := gdb.Transaction(func(tx *gorm.DB) error {
		var senders []string
		if err := tx.Raw(`
			SELECT DISTINCT spender_normalized_sender(from_address) AS email
			FROM (SELECT DISTINCT re.from_address` + ignoredEmailBase + `)
			ORDER BY email`).Scan(&senders).Error; err != nil {
			return err
		}
		for _, sender := range senders {
			if sender != "" {
				result.AvailableSenders = append(result.AvailableSenders, sender)
			}
		}

		if err := tx.Raw("SELECT COUNT(*) "+base, args...).Scan(&result.TotalCount).Error; err != nil {
			return err
		}
		if result.TotalCount == 0 || int64(result.Page-1) > (result.TotalCount-1)/int64(result.PageSize) {
			return nil
		}

		offset := int64(result.Page-1) * int64(result.PageSize)
		if err := tx.Raw(`
			SELECT re.id AS email_id, re.from_address,
				spender_normalized_sender(re.from_address) AS sender_email,
				re.subject, re.received_at_utc, ee.amount_minor, ee.currency, ee.occurred_at_utc,
				NULLIF(TRIM(COALESCE(ee.merchant_raw, '')), '') AS merchant_raw,
				ee.category, ee.extraction_model,
				COALESCE(ee.triage_signature_key, '') AS triage_signature_key,
				COALESCE(ee.triage_subject_pattern, '') AS triage_subject_pattern,
				COALESCE(ee.triage_body_cue, '') AS triage_body_cue,
				ee.extraction_model = 'policy:ignore' AS pre_llm_skipped
			`+base+`
			ORDER BY COALESCE(ee.occurred_at_utc, re.received_at_utc) DESC, re.internal_date_ms DESC, re.id DESC
			LIMIT ? OFFSET ?`, append(args, result.PageSize, offset)...).Scan(&result.Items).Error; err != nil {
			return err
		}
		if params.SummaryOnly || len(result.Items) == 0 {
			return nil
		}

		emailIDs := make([]string, len(result.Items))
		for i, item := range result.Items {
			emailIDs[i] = item.EmailID
		}
		var bodies []struct {
			ID          string
			CleanedText string
		}
		if err := tx.Raw("SELECT id, cleaned_text FROM raw_emails WHERE id IN ?", emailIDs).Scan(&bodies).Error; err != nil {
			return err
		}
		byEmailID := make(map[string]*string, len(bodies))
		for i := range bodies {
			byEmailID[bodies[i].ID] = &bodies[i].CleanedText
		}
		for i := range result.Items {
			result.Items[i].CleanedText = byEmailID[result.Items[i].EmailID]
		}
		return nil
	}, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func GetIgnoredEmailBody(gdb *gorm.DB, emailID string) (string, error) {
	var body string
	err := gdb.Raw("SELECT re.cleaned_text "+ignoredEmailBase+" AND re.id = ?", emailID).Row().Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return "", &NotFoundError{Message: "Ignored email not found."}
	}
	return body, err
}

func PromoteIgnoredEmail(gdb *gorm.DB, ctx context.Context, emailID string) (*PromoteIgnoredEmailResult, error) {
	emailID = strings.TrimSpace(emailID)
	if emailID == "" {
		return nil, &ValidationError{Message: "emailId is required."}
	}

	var rows []triageCandidateRow
	if err := gdb.Raw(`
		SELECT`+triageCandidateColumns+`
		FROM raw_emails re
		LEFT JOIN senders s ON s.id = re.sender_id
		LEFT JOIN email_extractions ee ON ee.email_id = re.id
		WHERE re.id = ?
		LIMIT 1`, emailID,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, &NotFoundError{Message: "Email not found."}
	}
	target := rows[0]
	if target.RawStatus != "non_transaction" || target.TriageState == nil || *target.TriageState != "ignored" {
		return nil, &ConflictError{Message: "Email is no longer in the ignored state."}
	}

	hasCompleteDraft := target.ExtractionModel != "policy:ignore" &&
		target.AmountMinor != nil &&
		*target.AmountMinor > 0 &&
		target.Currency != nil &&
		len(strings.TrimSpace(*target.Currency)) == 3

	var extracted *llm.ExtractResult
	usedLlm := false
	llmModelName := target.ExtractionModel

	if !hasCompleteDraft {
		var settings db.AppSetting
		if err := gdb.First(&settings, 1).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, &ValidationError{Message: "Application settings row is missing."}
			}
			return nil, err
		}
		if settings.LlmModel == nil || strings.TrimSpace(*settings.LlmModel) == "" {
			return nil, &ValidationError{Message: "Select a model from your local LLM server in Settings before recovering skipped emails."}
		}
		llmModelName = strings.TrimSpace(*settings.LlmModel)
		client, err := llm.NewClient(settings.LlmBaseURL, time.Duration(settings.LlmTimeoutMs)*time.Millisecond)
		if err != nil {
			return nil, &ValidationError{Message: err.Error()}
		}

		userTimezone := "UTC"
		if settings.Timezone != nil && strings.TrimSpace(*settings.Timezone) != "" && !strings.EqualFold(strings.TrimSpace(*settings.Timezone), "Local") {
			userTimezone = strings.TrimSpace(*settings.Timezone)
		}
		var countryRules []string
		if settings.CountryCode != nil && strings.TrimSpace(*settings.CountryCode) != "" {
			if pack := FindCountryPackByCode(*settings.CountryCode); pack != nil {
				countryRules = pack.LlmPromptRules
			}
		}
		catNames, err := ListCategoryNames(gdb)
		if err != nil {
			return nil, err
		}
		acctContexts := ListExtractionAccountContexts(gdb)
		llmAccounts := make([]llm.AccountContext, 0, len(acctContexts))
		for _, a := range acctContexts {
			llmAccounts = append(llmAccounts, llm.AccountContext{
				ID:          a.ID,
				Name:        a.Name,
				Type:        a.Type,
				Identifiers: a.Identifiers,
			})
		}

		extracted, err = client.ExtractTransaction(ctx, llmModelName, llm.ExtractInput{
			EmailID:            target.EmailID,
			FromAddress:        target.FromAddress,
			Subject:            target.Subject,
			CleanedText:        target.CleanedText,
			ReceivedAtUtc:      target.ReceivedAtUtc,
			UserCountry:        settings.CountryCode,
			CountryPromptRules: countryRules,
			UserTimezone:       userTimezone,
			UserCurrency:       settings.Currency,
			Categories:         catNames,
			Accounts:           llmAccounts,
		})
		if err != nil {
			return nil, err
		}
		usedLlm = true
		if extracted.AmountMinor <= 0 || len(strings.TrimSpace(extracted.Currency)) != 3 {
			return nil, &UnprocessableError{Message: "Local LLM could not extract a positive monetary amount from this email."}
		}
	}

	var txID string
	err := db.RunImmediateTx(gdb, func(tx *gorm.DB) error {
		var current []triageCandidateRow
		if err := tx.Raw(`
			SELECT`+triageCandidateColumns+`
			FROM raw_emails re
			LEFT JOIN senders s ON s.id = re.sender_id
			LEFT JOIN email_extractions ee ON ee.email_id = re.id
			WHERE re.id = ?
			LIMIT 1`, emailID,
		).Scan(&current).Error; err != nil {
			return err
		}
		if len(current) == 0 || current[0].RawStatus != "non_transaction" || current[0].TriageState == nil || *current[0].TriageState != "ignored" {
			return &ConflictError{Message: "Email was already promoted or changed."}
		}
		curRow := current[0]

		var amountMinor int64
		var currency, occurredAt, merchant, category, rawKind string
		var cardLast4, bankHint, pmRaw, llmAcctID, acctTypeGuess, refCode *string

		if usedLlm {
			amountMinor = extracted.AmountMinor
			currency = strings.ToUpper(strings.TrimSpace(extracted.Currency))
			occurredAt = extracted.OccurredAtUtc
			merchant = strings.TrimSpace(extracted.MerchantRaw)
			category = ResolveKnownCategory(tx, extracted.Category)
			rawKind = extracted.EmailKind
			cardLast4 = extracted.CardLast4
			bankHint = extracted.BankAccountHint
			pmRaw = extracted.PaymentMethodRaw
			llmAcctID = extracted.MatchedAccountID
			acctTypeGuess = extracted.AccountTypeGuess
			refCode = extracted.ReferenceCode
		} else {
			amountMinor = *curRow.AmountMinor
			currency = strings.ToUpper(strings.TrimSpace(*curRow.Currency))
			occurredAt = curRow.ReceivedAtUtc
			if curRow.OccurredAtUtc != nil && IsValidUtcIsoSeconds(strings.TrimSpace(*curRow.OccurredAtUtc)) {
				occurredAt = strings.TrimSpace(*curRow.OccurredAtUtc)
			}
			if curRow.MerchantRaw != nil {
				merchant = strings.TrimSpace(*curRow.MerchantRaw)
			}
			rawCat := ""
			if curRow.Category != nil {
				rawCat = *curRow.Category
			}
			category = ResolveKnownCategory(tx, rawCat)
			rawKind = curRow.EmailKind
			cardLast4 = curRow.CardLast4
			bankHint = curRow.BankAccountHint
			pmRaw = curRow.PaymentMethodRaw
			llmAcctID = curRow.LlmAccountID
			acctTypeGuess = curRow.AccountTypeGuess
			refCode = curRow.ReferenceCode
		}

		if merchant == "" {
			merchant = strings.TrimSpace(curRow.Subject)
		}
		if merchant == "" {
			merchant = "Unknown Merchant"
		}
		effectiveRole := ResolveEffectiveEmailRole(curRow.SenderRoleHint, rawKind, cardLast4, bankHint)

		var merchantRawPtr, merchantNormPtr *string
		if usedLlm {
			if p := strPtrOrNil(extracted.MerchantRaw); p != nil {
				merchantRawPtr = p
				if norm := strings.TrimSpace(NormalizeMerchant(*p)); norm != "" {
					merchantNormPtr = &norm
				}
			}
			sig := DeriveEmailTemplateSignature(curRow.FromAddress, curRow.Subject, curRow.CleanedText)
			if err := tx.Exec(`
				INSERT OR REPLACE INTO email_extractions (
					id, email_id, email_kind, amount_minor, currency, occurred_at_utc,
					merchant_raw, merchant_norm, category, card_last4,
					payment_method_raw, bank_account_hint, llm_account_id, reference_code,
					extraction_model, raw_llm_json,
					account_type_guess, triage_signature_key, triage_subject_pattern, triage_body_cue, triage_state
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'promoted')`,
				uuid.NewString(), emailID, effectiveRole, amountMinor, currency, occurredAt,
				merchantRawPtr, merchantNormPtr, category, cardLast4,
				pmRaw, bankHint, llmAcctID, refCode, llmModelName, extracted.RawJSON,
				acctTypeGuess, sig.SignatureKey, sig.SubjectPattern, sig.BodyCue,
			).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&db.EmailExtraction{}).
				Where("email_id = ?", emailID).
				Update("triage_state", "promoted").Error; err != nil {
				return err
			}
		}

		nowUtc := NowUtcIsoSeconds()
		var saveErr error
		txID, _, saveErr = admitPromotedTransactionInTx(
			tx, emailID, curRow.SenderID, curRow.FromAddress, effectiveRole,
			amountMinor, currency, occurredAt, merchant, category,
			cardLast4, bankHint, pmRaw, llmAcctID, acctTypeGuess, refCode, nowUtc,
		)
		return saveErr
	})
	if err != nil {
		return nil, err
	}

	detail, err := GetTransactionDetail(gdb, txID)
	if err != nil {
		return nil, err
	}
	return &PromoteIgnoredEmailResult{
		Transaction: detail.Transaction,
		EmailID:     emailID,
		UsedLlm:     usedLlm,
	}, nil
}
