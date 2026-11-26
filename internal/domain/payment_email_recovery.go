package domain

import "gorm.io/gorm"

const paymentRecoveryBase = `
	FROM raw_emails re
	JOIN email_extractions ee ON ee.email_id = re.id
	WHERE re.status = 'non_transaction'
	  AND (ee.triage_state IS NULL OR ee.triage_state = 'ghost')
	  AND ee.extraction_model NOT IN ('rule:billing_statement', 'user:statement')
	  AND NOT EXISTS (SELECT 1 FROM transaction_sources ts WHERE ts.email_id = re.id)`
const withoutPaymentDraft = `
	  AND (ee.amount_minor IS NULL OR ee.amount_minor <= 0
	       OR ee.currency IS NULL OR LENGTH(TRIM(ee.currency)) != 3)`

func ListSkippedPaymentEmails(gdb *gorm.DB, page, pageSize int) (*PaginatedSkippedTransactionsResponse, error) {
	page = max(1, page)
	if pageSize <= 0 {
		pageSize = 25
	}
	pageSize = min(pageSize, maxGhostRowsPerPage)
	result := PaginatedSkippedTransactionsResponse{
		Items: []GhostRowItem{}, Page: page, PageSize: pageSize,
	}
	err := gdb.Transaction(func(tx *gorm.DB) error {
		where := paymentRecoveryBase + withoutPaymentDraft
		if err := tx.Raw("SELECT COUNT(*) " + where).Scan(&result.TotalCount).Error; err != nil {
			return err
		}
		if result.TotalCount == 0 || int64(page-1) > (result.TotalCount-1)/int64(pageSize) {
			return nil
		}
		return tx.Raw(`
			SELECT re.id AS email_id, re.from_address, re.from_address AS sender_email,
			       re.subject, re.received_at_utc AS occurred_at_utc
			`+where+`
			ORDER BY re.internal_date_ms DESC, re.id DESC LIMIT ? OFFSET ?`,
			pageSize, (page-1)*pageSize).Scan(&result.Items).Error
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func GetSkippedPaymentEmailBody(gdb *gorm.DB, emailID string) (string, error) {
	var result struct{ CleanedText string }
	query := gdb.Raw("SELECT re.cleaned_text "+paymentRecoveryBase+" AND re.id = ?", emailID).Scan(&result)
	if query.Error != nil {
		return "", query.Error
	}
	if query.RowsAffected == 0 {
		return "", &NotFoundError{Message: "This skipped email is no longer available."}
	}
	return result.CleanedText, nil
}
