package llm

import "testing"

func TestNormalizeExtractedResult_IDRZeroDecimalRecovery(t *testing.T) {
	raw, cleaned, err := ParseAndValidateExtraction(`{
		"email_kind": "card_charge",
		"amount_decimal": "125.0",
		"currency": "IDR",
		"transaction_date": "2026-11-22",
		"transaction_time": "10:15:00",
		"timezone_offset": null,
		"merchant": "Tokopedia",
		"category": "Shopping",
		"card_last4": "xxxx-9876",
		"payment_method": "BCA",
		"bank_account_hint": null,
		"matched_account_id": null,
		"account_type_guess": "credit_card",
		"reference_code": null
	}`, nil)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	cur := "IDR"
	res, err := NormalizeExtractedResult(raw, cleaned, ExtractInput{
		CleanedText:   "Pembayaran sebesar Rp 125.000 berhasil di Tokopedia",
		ReceivedAtUtc: "2026-11-22T03:20:00Z",
		UserTimezone:  "Asia/Jakarta",
		UserCurrency:  &cur,
	})
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if res.AmountMinor != 125000 {
		t.Fatalf("expected recovered IDR amount 125000, got %d", res.AmountMinor)
	}
	if res.OccurredAtUtc != "2026-11-22T03:15:00Z" {
		t.Fatalf("expected Asia/Jakarta 10:15 -> 03:15Z, got %s", res.OccurredAtUtc)
	}
	if res.CardLast4 == nil || *res.CardLast4 != "9876" {
		t.Fatalf("expected card_last4=9876, got %v", res.CardLast4)
	}
}

func TestParseAndValidateExtraction_RejectsUnknownCategory(t *testing.T) {
	_, _, err := ParseAndValidateExtraction(`{
		"email_kind": "receipt",
		"amount_decimal": "10.00",
		"currency": "USD",
		"transaction_date": "2026-11-22",
		"transaction_time": null,
		"timezone_offset": null,
		"merchant": "Store",
		"category": "NotACategory",
		"card_last4": null,
		"payment_method": null,
		"bank_account_hint": null,
		"matched_account_id": null,
		"account_type_guess": null,
		"reference_code": null
	}`, nil)
	if !IsExtractionError(err) {
		t.Fatalf("expected ExtractionError for unknown category, got %v", err)
	}
}
