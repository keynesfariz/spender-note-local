package domain

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"portspendernote/internal/db"
)

type TransferItemInput struct {
	Action                 string   `json:"action"`
	ID                     string   `json:"id"`
	RequestID              string   `json:"request_id"`
	AccountID              string   `json:"account_id"`
	DestinationAccountID   string   `json:"destination_account_id"`
	AmountMinor            int64    `json:"amount_minor"`
	DestinationAmountMinor int64    `json:"destination_amount_minor"`
	OccurredAtUtc          string   `json:"occurred_at_utc"`
	SourcePostedAtUtc      string   `json:"source_posted_at_utc"`
	DestinationPostedAtUtc string   `json:"destination_posted_at_utc"`
	Note                   *string  `json:"note"`
	EmailIDs               []string `json:"email_ids"`
	ExpectedTransactionID  string   `json:"expected_transaction_id"`
	MatchTransactionID     string   `json:"match_transaction_id"`
	CardPayment            bool     `json:"card_payment"`
}

type TransferInput struct {
	Action string              `json:"action"`
	Items  []TransferItemInput `json:"items"`
}

type TransferResult struct {
	Items []TransactionListItem `json:"items"`
}

const userStatementExtraction = "user:statement"

func ApplyTransfers(gdb *gorm.DB, input TransferInput) (*TransferResult, error) {
	if len(input.Items) == 0 || len(input.Items) > 100 {
		return nil, errors.New("Select between 1 and 100 entries.")
	}
	result := &TransferResult{Items: []TransactionListItem{}}
	err := db.RunImmediateTx(gdb, func(tx *gorm.DB) error {
		seenEmails := make(map[string]bool)
		for i, item := range input.Items {
			if item.Action == "" {
				item.Action = input.Action
			}
			item.EmailIDs = normalizeTransactionIDs(item.EmailIDs)
			for _, id := range item.EmailIDs {
				if seenEmails[id] {
					return errors.New("Select each email only once.")
				}
				seenEmails[id] = true
			}
			id, err := applyTransferItem(tx, item)
			if err != nil {
				return fmt.Errorf("Entry %d: %w", i+1, err)
			}
			if id != "" {
				detail, err := getTransactionDetail(tx, id, false)
				if err != nil {
					return err
				}
				result.Items = append(result.Items, detail.Transaction)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

type transferEmail struct {
	ID              string
	Status          string
	ExtractionModel string
	TriageState     string
	TransactionID   string
}

func loadTransferEmails(tx *gorm.DB, ids []string) ([]transferEmail, string, error) {
	if len(ids) == 0 {
		return nil, "", nil
	}
	if len(ids) > 100 {
		return nil, "", errors.New("Select at most 100 emails per entry.")
	}
	var rows []transferEmail
	if err := tx.Raw(`
		SELECT re.id, re.status, COALESCE(ee.extraction_model, '') AS extraction_model,
		       COALESCE(ee.triage_state, '') AS triage_state,
		       COALESCE(ts.transaction_id, '') AS transaction_id
		FROM raw_emails re
		LEFT JOIN email_extractions ee ON ee.email_id = re.id
		LEFT JOIN transaction_sources ts ON ts.email_id = re.id
		WHERE re.id IN ?`, ids).Scan(&rows).Error; err != nil {
		return nil, "", err
	}
	if len(rows) != len(ids) {
		return nil, "", errors.New("A selected email no longer exists.")
	}
	owner := rows[0].TransactionID
	for _, row := range rows[1:] {
		if row.TransactionID != owner {
			return nil, "", errors.New("The selected emails do not belong to the same entry.")
		}
	}
	return rows, owner, nil
}

func applyTransferItem(tx *gorm.DB, input TransferItemInput) (string, error) {
	if input.Action != "record" && input.Action != "match" && input.Action != "non_transaction" {
		return "", errors.New("Select a supported transfer action.")
	}
	if input.ID != "" && (input.Action != "record" || len(input.EmailIDs) != 0 || input.ExpectedTransactionID != "") {
		return "", errors.New("Edit an existing transfer separately from email classification.")
	}
	emails, ownerID, err := loadTransferEmails(tx, input.EmailIDs)
	if err != nil {
		return "", err
	}
	var owner db.Transaction
	if ownerID != "" {
		if err := tx.First(&owner, "id = ?", ownerID).Error; err != nil {
			return "", err
		}
	}
	if input.Action == "non_transaction" {
		return "", markStatement(tx, input, emails, owner)
	}

	if input.Action == "match" {
		if len(emails) == 0 {
			return "", errors.New("Select an email to match.")
		}
		target, err := findTransfer(tx, input.MatchTransactionID, input.CardPayment)
		if err != nil {
			return "", err
		}
		if ownerID == target.ID {
			return target.ID, nil
		}
		if err := validateTransferEvidence(tx, input, emails, owner); err != nil {
			return "", err
		}
		return target.ID, attachTransferEmails(tx, emails, ownerID, target.ID)
	}

	movement, err := prepareTransfer(tx, input)
	if err != nil {
		return "", err
	}
	if owner.Kind == "transfer" {
		if sameTransfer(owner, movement) {
			return owner.ID, nil
		}
		return "", errors.New("This email is already attached to a different movement.")
	}
	if input.ID != "" {
		current, err := findTransfer(tx, input.ID, input.CardPayment)
		if err != nil {
			return "", err
		}
		return current.ID, saveTransferMovement(tx, current.ID, movement)
	}
	if len(emails) == 0 {
		if input.ExpectedTransactionID != "" {
			return "", errors.New("Select the source emails to convert.")
		}
		return recordManualTransfer(tx, input.RequestID, movement)
	}

	if err := validateTransferEvidence(tx, input, emails, owner); err != nil {
		return "", err
	}
	if ownerID != "" {
		if err := saveTransferMovement(tx, ownerID, movement); err != nil {
			return "", err
		}
		return ownerID, attachTransferEmails(tx, emails, ownerID, ownerID)
	}
	id, err := createTransferRow(tx, movement, "gmail")
	if err != nil {
		return "", err
	}
	return id, attachTransferEmails(tx, emails, "", id)
}

func findTransfer(tx *gorm.DB, id string, cardPayment bool) (db.Transaction, error) {
	var row db.Transaction
	if err := tx.First(&row, "id = ? AND kind = 'transfer'", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return row, errors.New("The selected transfer no longer exists.")
		}
		return row, err
	}
	if cardPayment {
		var dest db.Account
		if err := tx.First(&dest, "id = ?", *row.DestinationAccountID).Error; err != nil {
			return row, err
		}
		if dest.Type != "credit_card" {
			return row, errors.New("Select a transfer to a credit card.")
		}
	}
	return row, nil
}

func prepareTransfer(tx *gorm.DB, input TransferItemInput) (db.Transaction, error) {
	sourceID, destID := strings.TrimSpace(input.AccountID), strings.TrimSpace(input.DestinationAccountID)
	if sourceID == "" || destID == "" || sourceID == destID {
		return db.Transaction{}, errors.New("Select two distinct accounts.")
	}
	var source, dest db.Account
	if err := tx.First(&source, "id = ?", sourceID).Error; err != nil {
		return db.Transaction{}, fmt.Errorf("Funding account: %w", err)
	}
	if err := tx.First(&dest, "id = ?", destID).Error; err != nil {
		return db.Transaction{}, fmt.Errorf("Receiving account: %w", err)
	}
	if input.CardPayment && dest.Type != "credit_card" {
		return db.Transaction{}, errors.New("Select a credit card for a card payment.")
	}
	destAmount := input.DestinationAmountMinor
	if source.Currency == dest.Currency && destAmount == 0 {
		destAmount = input.AmountMinor
	}
	if input.AmountMinor <= 0 || destAmount <= 0 {
		return db.Transaction{}, errors.New("Enter positive amounts on both sides.")
	}
	if source.Currency == dest.Currency && input.AmountMinor != destAmount {
		return db.Transaction{}, errors.New("Same-currency transfer amounts must match.")
	}
	if !IsValidUtcIsoSeconds(input.OccurredAtUtc) ||
		(input.SourcePostedAtUtc != "" && !IsValidUtcIsoSeconds(input.SourcePostedAtUtc)) ||
		(input.DestinationPostedAtUtc != "" && !IsValidUtcIsoSeconds(input.DestinationPostedAtUtc)) {
		return db.Transaction{}, errors.New("Enter valid transfer and posting dates.")
	}
	label := "Transfer"
	if dest.Type == "credit_card" {
		label = "Credit card payment"
	}
	var note *string
	if input.Note != nil {
		note = strPtrOrNil(*input.Note)
	}
	return db.Transaction{
		Kind: "transfer", AccountID: &sourceID, AmountMinor: input.AmountMinor,
		Currency: source.Currency, OccurredAtUtc: input.OccurredAtUtc, Merchant: label, Note: note,
		DestinationAccountID: &destID, DestinationAmountMinor: &destAmount, DestinationCurrency: &dest.Currency,
		SourcePostedAtUtc:      strPtrOrNil(input.SourcePostedAtUtc),
		DestinationPostedAtUtc: strPtrOrNil(input.DestinationPostedAtUtc),
		ManualOverride:         1, Reviewed: 1,
	}, nil
}

func sameTransfer(existing, movement db.Transaction) bool {
	return existing.AccountID != nil && *existing.AccountID == *movement.AccountID &&
		existing.AmountMinor == movement.AmountMinor && existing.Currency == movement.Currency &&
		existing.DestinationAccountID != nil && *existing.DestinationAccountID == *movement.DestinationAccountID &&
		existing.DestinationAmountMinor != nil && *existing.DestinationAmountMinor == *movement.DestinationAmountMinor &&
		existing.DestinationCurrency != nil && *existing.DestinationCurrency == *movement.DestinationCurrency &&
		existing.OccurredAtUtc == movement.OccurredAtUtc &&
		nullableStringEqual(existing.SourcePostedAtUtc, movement.SourcePostedAtUtc) &&
		nullableStringEqual(existing.DestinationPostedAtUtc, movement.DestinationPostedAtUtc) &&
		nullableStringEqual(existing.Note, movement.Note)
}

func createTransferRow(tx *gorm.DB, movement db.Transaction, sourceType string) (string, error) {
	categoryID, err := validateTransactionCategoryID(tx, DefaultCategory)
	if err != nil {
		return "", err
	}
	movement.ID = uuid.NewString()
	movement.SourceType = sourceType
	movement.CategoryID = categoryID
	movement.CreatedAtUtc = NowUtcIsoSeconds()
	movement.UpdatedAtUtc = movement.CreatedAtUtc
	return movement.ID, tx.Create(&movement).Error
}

func saveTransferMovement(tx *gorm.DB, id string, movement db.Transaction) error {
	return tx.Model(&db.Transaction{}).Where("id = ?", id).Updates(map[string]any{
		"kind": movement.Kind, "account_id": movement.AccountID, "amount_minor": movement.AmountMinor,
		"currency": movement.Currency, "occurred_at_utc": movement.OccurredAtUtc, "merchant": movement.Merchant,
		"destination_account_id": movement.DestinationAccountID, "destination_amount_minor": movement.DestinationAmountMinor,
		"destination_currency": movement.DestinationCurrency, "source_posted_at_utc": movement.SourcePostedAtUtc,
		"destination_posted_at_utc": movement.DestinationPostedAtUtc, "note": movement.Note,
		"manual_override": 1, "reviewed": 1, "updated_at_utc": NowUtcIsoSeconds(),
	}).Error
}

func recordManualTransfer(tx *gorm.DB, key string, movement db.Transaction) (string, error) {
	requestID, err := uuid.Parse(key)
	if err != nil {
		return "", errors.New("A valid transfer request ID is required.")
	}
	key = requestID.String()
	encoded, err := json.Marshal(movement)
	if err != nil {
		return "", err
	}
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(encoded))
	var receipt struct {
		Fingerprint   string
		TransactionID *string
	}
	result := tx.Table("transfer_requests").Where("id = ?", key).Take(&receipt)
	switch {
	case result.Error == nil:
		if receipt.Fingerprint != fingerprint {
			return "", errors.New("This request ID was already used for another movement.")
		}
		if receipt.TransactionID == nil {
			return "", errors.New("This transfer was already recorded and then removed.")
		}
		return *receipt.TransactionID, nil
	case !errors.Is(result.Error, gorm.ErrRecordNotFound):
		return "", result.Error
	}
	id, err := createTransferRow(tx, movement, "manual")
	if err != nil {
		return "", err
	}
	return id, tx.Exec("INSERT INTO transfer_requests (id, fingerprint, transaction_id) VALUES (?, ?, ?)", key, fingerprint, id).Error
}

func validateTransferEvidence(tx *gorm.DB, input TransferItemInput, emails []transferEmail, owner db.Transaction) error {
	for _, email := range emails {
		if email.ExtractionModel == "rule:billing_statement" || email.ExtractionModel == userStatementExtraction {
			return errors.New("Statement notices cannot be recorded as transfers.")
		}
	}
	if owner.ID == "" {
		if input.ExpectedTransactionID != "" {
			return errors.New("The selected emails have changed. Refresh and try again.")
		}
		for _, email := range emails {
			if email.Status != "non_transaction" || email.TriageState == "ignored" {
				return errors.New("This email is no longer eligible for payment recovery.")
			}
		}
		return nil
	}
	return validateWholeExpense(tx, input, owner)
}

func validateWholeExpense(tx *gorm.DB, input TransferItemInput, owner db.Transaction) error {
	if input.ExpectedTransactionID != owner.ID || owner.Kind != "expense" || owner.SourceType != "gmail" {
		return errors.New("The selected emails have changed. Refresh and try again.")
	}
	if owner.AmbiguousSourceAmount {
		return errors.New("The original payment amount in this expense cannot be safely isolated.")
	}
	var count int64
	if err := tx.Model(&db.TransactionSource{}).Where("transaction_id = ?", owner.ID).Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(input.EmailIDs)) {
		return errors.New("Select all sources for this expense, or split unrelated sources before converting it.")
	}
	return nil
}

func attachTransferEmails(tx *gorm.DB, emails []transferEmail, oldID, targetID string) error {
	for _, email := range emails {
		if email.TransactionID != "" {
			if err := tx.Model(&db.TransactionSource{}).Where("email_id = ?", email.ID).
				Updates(map[string]any{"transaction_id": targetID, "match_state": "confirmed"}).Error; err != nil {
				return err
			}
		} else if err := tx.Create(&db.TransactionSource{
			ID: uuid.NewString(), TransactionID: targetID, EmailID: email.ID, Role: "card_charge",
			MatchScore: 1, MatchState: "confirmed",
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&db.RawEmail{}).Where("id = ?", email.ID).
			Updates(map[string]any{"status": "extracted", "error_message": nil}).Error; err != nil {
			return err
		}
		if err := tx.Model(&db.EmailExtraction{}).Where("email_id = ?", email.ID).Update("triage_state", nil).Error; err != nil {
			return err
		}
	}
	if oldID != "" && oldID != targetID {
		return tx.Delete(&db.Transaction{}, "id = ?", oldID).Error
	}
	return nil
}

func markStatement(tx *gorm.DB, input TransferItemInput, emails []transferEmail, owner db.Transaction) error {
	if len(emails) == 0 {
		return errors.New("Select a statement email.")
	}
	if owner.ID == "" {
		for _, email := range emails {
			if email.Status != "non_transaction" ||
				(email.ExtractionModel != userStatementExtraction && email.ExtractionModel != "rule:billing_statement") {
				return errors.New("Only email-backed statement expenses can be marked non-transaction here.")
			}
		}
		return nil
	}
	if err := validateWholeExpense(tx, input, owner); err != nil {
		return err
	}
	for _, email := range emails {
		if err := tx.Model(&db.RawEmail{}).Where("id = ?", email.ID).
			Updates(map[string]any{"status": "non_transaction", "error_message": nil}).Error; err != nil {
			return err
		}
		if err := tx.Model(&db.EmailExtraction{}).Where("email_id = ?", email.ID).Updates(map[string]any{
			"email_kind": "non_transaction", "triage_state": nil, "extraction_model": userStatementExtraction,
		}).Error; err != nil {
			return err
		}
	}
	return tx.Delete(&db.Transaction{}, "id = ?", owner.ID).Error
}
