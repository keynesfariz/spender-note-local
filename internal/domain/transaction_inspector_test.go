package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"gorm.io/gorm"

	"portspendernote/internal/db"
)

func TestTransactionInspector_DefersContentAndChecksCurrentMembership(t *testing.T) {
	gdb := openDomainTestDB(t)
	const timestamp = "2026-11-21T08:50:00Z"
	const rawJSON = `{"merchant":"private extraction"}`
	manual, err := CreateManualTransaction(gdb, CreateManualTransactionInput{
		AmountMinor: 1000, OccurredAtUtc: timestamp, Merchant: "Manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	manualInspector, err := GetTransactionInspector(gdb, manual.ID)
	if err != nil || manualInspector.Transaction.SourceCount != 0 ||
		manualInspector.Sources == nil || len(manualInspector.Sources) != 0 {
		t.Fatalf("manual inspector = %#v, %v; want no sources", manualInspector, err)
	}

	amount := int64(50000)
	txIDs := make([]string, 0, 2)
	for i, emailID := range []string{"earlier", "later"} {
		seedTestEmail(t, gdb, emailID, 1795251000000+int64(i), timestamp)
		if err := gdb.Create(&db.EmailExtraction{
			ID: emailID + "-extraction", EmailID: emailID, EmailKind: "receipt",
			AmountMinor: &amount, Currency: strPtr("IDR"), OccurredAtUtc: strPtr(timestamp),
			MerchantRaw: strPtr("Coffee"), ExtractionModel: "test-model", RawLlmJSON: rawJSON,
		}).Error; err != nil {
			t.Fatal(err)
		}
		id, err := SaveExtractedTransaction(gdb, ExtractedTransactionInput{
			EmailID: emailID, EmailKind: "receipt", AmountMinor: 50000, Currency: "IDR",
			OccurredAtUtc: timestamp, MerchantRaw: "Coffee", Category: "Food",
		})
		if err != nil {
			t.Fatal(err)
		}
		txIDs = append(txIDs, id)
	}

	if _, err := GetTransactionSourceBody(gdb, txIDs[0], "later"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("mismatched transaction/source returned %v", err)
	}

	combined, err := CombineTransactions(gdb, CombineTransactionsInput{
		TransactionIDs: txIDs, Merchant: "Coffee", OccurredAtUtc: timestamp,
	})
	if err != nil {
		t.Fatal(err)
	}
	inspector, err := GetTransactionInspector(gdb, combined.ID)
	if err != nil {
		t.Fatal(err)
	}
	if inspector.Transaction.ID != combined.ID ||
		len(inspector.Sources) != 2 || inspector.Sources[0].EmailID != "earlier" || inspector.Sources[1].EmailID != "later" {
		t.Fatalf("inspector detail = %#v", inspector)
	}
	encoded, err := json.Marshal(inspector)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"cleaned_text", "raw_llm_json", "Paid merchant", "private extraction"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("inspector metadata contains %q", private)
		}
	}

	body, err := GetTransactionSourceBody(gdb, combined.ID, "later")
	if err != nil || body.CleanedText != "Paid merchant" {
		t.Fatalf("current source body = %#v, %v", body, err)
	}
	newID, err := SplitTransactionSource(gdb, combined.ID, "later")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := GetTransactionSourceBody(gdb, combined.ID, "later"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("detached source body returned %v", err)
	}
	if body, err := GetTransactionSourceBody(gdb, newID, "later"); err != nil || body.CleanedText != "Paid merchant" {
		t.Fatalf("reattached source body = %#v, %v", body, err)
	}
}
