package domain

import "gorm.io/gorm"

type TransactionInspectorSource struct {
	ID            string `json:"id"`
	EmailID       string `gorm:"column:email_id" json:"email_id"`
	Role          string `json:"role"`
	MatchState    string `json:"match_state"`
	FromAddress   string `json:"from_address"`
	Subject       string `json:"subject"`
	ReceivedAtUtc string `gorm:"column:received_at_utc" json:"received_at_utc"`
}

type TransactionInspectorView struct {
	Transaction TransactionListItem          `json:"transaction"`
	Sources     []TransactionInspectorSource `json:"sources"`
}

type TransactionSourceBody struct {
	CleanedText string `json:"cleaned_text"`
}

func GetTransactionInspector(gdb *gorm.DB, id string) (*TransactionInspectorView, error) {
	sources := []TransactionInspectorSource{}
	if err := gdb.Raw(`
		SELECT ts.id, ts.email_id, ts.role, ts.match_state, re.from_address, re.subject, re.received_at_utc
		FROM transaction_sources ts
		JOIN raw_emails re ON re.id = ts.email_id
		WHERE ts.transaction_id = ?
		ORDER BY re.internal_date_ms ASC`, id).Scan(&sources).Error; err != nil {
		return nil, err
	}
	roles := make([]string, 0, len(sources))
	for _, source := range sources {
		roles = append(roles, source.Role)
	}
	item, err := getTransactionItem(gdb, id, roles)
	if err != nil {
		return nil, err
	}
	return &TransactionInspectorView{Transaction: *item, Sources: sources}, nil
}

func GetTransactionSourceBody(gdb *gorm.DB, transactionID, emailID string) (*TransactionSourceBody, error) {
	var body TransactionSourceBody
	err := gdb.Table("transaction_sources ts").
		Select("re.cleaned_text").
		Joins("JOIN raw_emails re ON re.id = ts.email_id").
		Where("ts.transaction_id = ? AND ts.email_id = ?", transactionID, emailID).
		Take(&body).Error
	if err != nil {
		return nil, err
	}
	return &body, nil
}
