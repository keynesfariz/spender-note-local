package domain

import (
	"testing"

	"github.com/google/uuid"
	"portspendernote/internal/db"
)

func TestTransferRetryAndAtomicBatch(t *testing.T) {
	gdb := openDomainTestDB(t)
	source := seedTestAccount(t, gdb, "source", "Funding", "IDR")
	dest := seedTestAccount(t, gdb, "dest", "Receiving", "IDR")
	item := TransferItemInput{
		RequestID: uuid.NewString(), AccountID: source, DestinationAccountID: dest,
		AmountMinor: 900000, OccurredAtUtc: "2026-11-23T08:00:00Z",
	}
	input := TransferInput{Action: "record", Items: []TransferItemInput{item}}
	first, err := ApplyTransfers(gdb, input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ApplyTransfers(gdb, input)
	if err != nil || second.Items[0].ID != first.Items[0].ID {
		t.Fatalf("retry: %#v, %v", second, err)
	}
	input.Items[0].AmountMinor++
	if _, err := ApplyTransfers(gdb, input); err == nil {
		t.Fatal("changed payload reused the original request ID")
	}
	if err := DeleteTransaction(gdb, first.Items[0].ID); err != nil {
		t.Fatal(err)
	}
	input.Items[0] = item
	if _, err := ApplyTransfers(gdb, input); err == nil {
		t.Fatal("a retry resurrected a deleted transfer")
	}
	item.RequestID = uuid.NewString()
	bad := item
	bad.RequestID, bad.DestinationAccountID = uuid.NewString(), source
	input.Items = []TransferItemInput{item, bad}
	if _, err := ApplyTransfers(gdb, input); err == nil {
		t.Fatal("invalid second item should fail batch")
	}
	var count int64
	gdb.Model(&db.Transaction{}).Count(&count)
	if count != 0 {
		t.Fatalf("failed batch left %d transactions", count)
	}
}

func TestMatchTransferReparentsEvidenceAndSurvivesRetry(t *testing.T) {
	gdb := openDomainTestDB(t)
	source := seedTestAccount(t, gdb, "source", "Funding", "IDR")
	dest := seedTestAccount(t, gdb, "dest", "Receiving", "IDR")
	when := "2026-11-23T08:00:00Z"
	seedTestEmail(t, gdb, "paid", 1795420800000, when)
	expense, err := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
		EmailID: "paid", EmailKind: "card_charge", AmountMinor: 900000, Currency: "IDR",
		OccurredAtUtc: when, MerchantRaw: "Paid", ResolvedAccountID: &source,
	})
	if err != nil {
		t.Fatal(err)
	}
	recorded, err := ApplyTransfers(gdb, TransferInput{Action: "record", Items: []TransferItemInput{{
		RequestID: uuid.NewString(), AccountID: source, DestinationAccountID: dest,
		AmountMinor: 900000, OccurredAtUtc: when,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	target := recorded.Items[0].ID
	input := TransferInput{Action: "match", Items: []TransferItemInput{{
		EmailIDs: []string{"paid"}, ExpectedTransactionID: expense, MatchTransactionID: target,
	}}}
	for range 2 {
		result, err := ApplyTransfers(gdb, input)
		if err != nil || result.Items[0].ID != target {
			t.Fatalf("match: %#v, %v", result, err)
		}
	}
	var rows []db.Transaction
	if err := gdb.Find(&rows).Error; err != nil || len(rows) != 1 || rows[0].Kind != "transfer" {
		t.Fatalf("transactions after match: %#v, %v", rows, err)
	}
	var evidence db.TransactionSource
	if err := gdb.First(&evidence, "email_id = ?", "paid").Error; err != nil || evidence.TransactionID != target {
		t.Fatalf("source after match: %#v, %v", evidence, err)
	}
}

func TestRecoverDraftlessPaymentAndMarkStatement(t *testing.T) {
	gdb := openDomainTestDB(t)
	source := seedTestAccount(t, gdb, "source", "Funding", "IDR")
	dest := seedTestAccount(t, gdb, "dest", "Receiving", "IDR")
	when := "2026-11-23T08:00:00Z"
	seedTestEmail(t, gdb, "paid", 1795420800000, when)
	if err := gdb.Model(&db.RawEmail{}).Where("id = ?", "paid").
		Updates(map[string]any{"status": "non_transaction", "cleaned_text": "Payment received"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := gdb.Create(&db.EmailExtraction{
		ID: uuid.NewString(), EmailID: "paid", EmailKind: "non_transaction",
		ExtractionModel: "test", RawLlmJSON: "{}",
	}).Error; err != nil {
		t.Fatal(err)
	}
	skipped, err := ListSkippedPaymentEmails(gdb, 1, 25)
	if err != nil || len(skipped.Items) != 1 || skipped.Items[0].EmailID != "paid" {
		t.Fatalf("draftless list: %#v, %v", skipped, err)
	}
	body, err := GetSkippedPaymentEmailBody(gdb, "paid")
	if err != nil || body != "Payment received" {
		t.Fatalf("preview = %q, %v", body, err)
	}
	input := TransferInput{Action: "record", Items: []TransferItemInput{{
		EmailIDs: []string{"paid"}, AccountID: source, DestinationAccountID: dest,
		AmountMinor: 900000, OccurredAtUtc: when,
	}}}
	recovered, err := ApplyTransfers(gdb, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered.Items) != 1 || recovered.Items[0].Kind != "transfer" {
		t.Fatalf("recovered payment: %#v", recovered.Items)
	}
	detail, err := GetTransactionInspector(gdb, recovered.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Sources) != 1 || detail.Sources[0].EmailID != "paid" {
		t.Fatalf("recovered payment sources: %#v", detail.Sources)
	}
	if _, err := GetSkippedPaymentEmailBody(gdb, "paid"); err == nil {
		t.Fatal("recorded payment remains readable as skipped evidence")
	}

	seedTestEmail(t, gdb, "statement", 1795420800001, when)
	expense, err := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
		EmailID: "statement", EmailKind: "card_charge", AmountMinor: 900000, Currency: "IDR",
		OccurredAtUtc: when, MerchantRaw: "Bill", ResolvedAccountID: &source,
	})
	if err != nil {
		t.Fatal(err)
	}
	statement := TransferInput{Action: "non_transaction", Items: []TransferItemInput{{
		EmailIDs: []string{"statement"}, ExpectedTransactionID: expense,
	}}}
	if _, err := ApplyTransfers(gdb, statement); err != nil {
		t.Fatal(err)
	}
	var remaining int64
	if err := gdb.Model(&db.Transaction{}).Where("id = ?", expense).Count(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatal("the statement still has an expense")
	}
	var email db.RawEmail
	if err := gdb.First(&email, "id = ?", "statement").Error; err != nil {
		t.Fatal(err)
	}
	var extraction db.EmailExtraction
	if err := gdb.First(&extraction, "email_id = ?", email.ID).Error; err != nil {
		t.Fatal(err)
	}
	if email.Status != "non_transaction" || extraction.EmailKind != "non_transaction" ||
		extraction.ExtractionModel != userStatementExtraction {
		t.Fatalf("statement disposition = %q / %q / %q", email.Status, extraction.EmailKind, extraction.ExtractionModel)
	}
	if _, err := ApplyTransfers(gdb, statement); err != nil {
		t.Fatalf("retry marking statement: %v", err)
	}
	if _, err := GetSkippedPaymentEmailBody(gdb, "statement"); err == nil {
		t.Fatal("statement became eligible for payment recovery")
	}
}

func TestManualIncomeUsesReceivingAccountCurrency(t *testing.T) {
	gdb := openDomainTestDB(t)
	configureTestLocale(t, gdb, "Asia/Jakarta", "IDR")
	account := seedTestAccount(t, gdb, "foreign", "USD balance", "USD")
	tag, err := CreateTag(gdb, CreateTagInput{Name: "Dinner"})
	if err != nil {
		t.Fatal(err)
	}
	input := CreateManualTransactionInput{
		Kind: "income", AccountID: &account, AmountMinor: 750,
		OccurredAtUtc: "2026-11-23T08:00:00Z", TagIDs: []string{tag.ID},
	}
	item, err := CreateManualTransaction(gdb, input)
	if err != nil || item.Kind != "income" || item.Currency != "USD" || len(item.Tags) != 1 {
		t.Fatalf("income = %#v, %v", item, err)
	}
	if _, err := UpdateTransaction(gdb, item.ID, UpdateTransactionInput{HasAccountID: true}); err == nil {
		t.Fatal("income lost the receiving account")
	}
	idrAccount := seedTestAccount(t, gdb, "local", "IDR balance", "IDR")
	if _, err := UpdateTransaction(gdb, item.ID, UpdateTransactionInput{HasAccountID: true, AccountID: &idrAccount}); err == nil {
		t.Fatal("an account-only edit silently changed the income currency")
	}
	amount := int64(125000)
	moved, err := UpdateTransaction(gdb, item.ID, UpdateTransactionInput{
		HasAccountID: true, AccountID: &idrAccount, AmountMinor: &amount,
	})
	if err != nil || moved.Currency != "IDR" || moved.AmountMinor != amount {
		t.Fatalf("explicit cross-currency income edit = %#v, %v", moved, err)
	}
}

func TestReclassificationDistinguishesMetadataEditsFromCombinedAmounts(t *testing.T) {
	gdb := openDomainTestDB(t)
	configureTestLocale(t, gdb, "Asia/Jakarta", "IDR")
	when := "2026-11-23T08:00:00Z"
	account := seedTestAccount(t, gdb, "source", "Funding", "IDR")
	dest := seedTestAccount(t, gdb, "dest", "Receiving", "IDR")
	add := func(email string) string {
		t.Helper()
		seedTestEmail(t, gdb, email, 1795420800000, when)
		id, err := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
			EmailID: email, EmailKind: "card_charge", AmountMinor: 900000, Currency: "IDR",
			OccurredAtUtc: when, MerchantRaw: "Paid", ResolvedAccountID: &account,
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	pay := func(email, owner string) error {
		t.Helper()
		_, err := ApplyTransfers(gdb, TransferInput{Action: "record", Items: []TransferItemInput{{
			EmailIDs: []string{email}, ExpectedTransactionID: owner,
			AccountID: account, DestinationAccountID: dest, AmountMinor: 900000, OccurredAtUtc: when,
		}}})
		return err
	}

	edited := add("edited")
	note := "payment note"
	if _, err := UpdateTransaction(gdb, edited, UpdateTransactionInput{HasNote: true, Note: &note}); err != nil {
		t.Fatal(err)
	}
	if err := pay("edited", edited); err != nil {
		t.Fatalf("a metadata-only edit prevented conversion: %v", err)
	}
	first, second := add("first"), add("second")
	combined, err := CombineTransactions(gdb, CombineTransactionsInput{
		TransactionIDs: []string{first, second}, OccurredAtUtc: when, Category: strPtr(DefaultCategory), Merchant: "Combined",
	})
	if err != nil {
		t.Fatal(err)
	}
	detached, err := SplitTransactionSource(gdb, combined.ID, "second")
	if err != nil {
		t.Fatal(err)
	}
	if err := pay("second", detached); err == nil {
		t.Fatal("a split source still represented by a combined amount was converted")
	}
}
