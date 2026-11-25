package domain

import "gorm.io/gorm"

type TransactionInspectorSource struct {
	ID            string `json:"id"`
	EmailID       string `json:"email_id"`
	Role          string `json:"role"`
	MatchState    string `json:"match_state"`
	FromAddress   string `json:"from_address"`
	Subject       string `json:"subject"`
	ReceivedAtUtc string `json:"received_at_utc"`
}

type TransactionInspectorView struct {
	Transaction TransactionListItem          `json:"transaction"`
	Sources     []TransactionInspectorSource `json:"sources"`
}

type TransactionSourceBody struct {
	CleanedText string `json:"cleaned_text"`
}

func GetTransactionInspector(gdb *gorm.DB, id string) (*TransactionInspectorView, error) {
	detail, err := getTransactionDetail(gdb, id, false)
	if err != nil {
		return nil, err
	}
	sources := make([]TransactionInspectorSource, 0, len(detail.Sources))
	for _, source := range detail.Sources {
		sources = append(sources, TransactionInspectorSource{
			ID: source.ID, EmailID: source.EmailID, Role: source.Role, MatchState: source.MatchState,
			FromAddress: source.FromAddress, Subject: source.Subject, ReceivedAtUtc: source.ReceivedAtUtc,
		})
	}
	return &TransactionInspectorView{Transaction: detail.Transaction, Sources: sources}, nil
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
