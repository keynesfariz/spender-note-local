package llm

import (
	"slices"
	"strings"
	"testing"
)

func TestParseAmountToMinor(t *testing.T) {
	for _, tc := range []struct {
		raw      string
		currency string
		want     int64
	}{
		{"Rp 10000,00", "IDR", 10000},
		{"10.000,00", "IDR", 10000},
		{"10,000.00", "IDR", 10000},
		{"10,000", "IDR", 10000},
		{"1.234", "IDR", 1234},
		{"1.234", "USD", 0},
		{"12,34", "EUR", 1234},
		{"1.234,56", "EUR", 123456},
		{"1,234.56", "USD", 123456},
		{"100", "CLP", 100},
		{"1.234", "LYD", 1234},
		{"1.2345", "CLF", 12345},
		{"1,2,3", "USD", 0},
		{"12,34.50", "USD", 0},
		{"10000,01", "IDR", 0},
	} {
		t.Run(tc.currency+" "+tc.raw, func(t *testing.T) {
			got, err := parseAmountToMinor(tc.raw, tc.currency)
			if tc.want == 0 && err != nil {
				return
			}
			if err != nil || tc.want == 0 || got != tc.want {
				t.Fatalf("parseAmountToMinor(%q, %q) = %d, %v; want %d (zero means invalid)", tc.raw, tc.currency, got, err, tc.want)
			}
		})
	}
}

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

func TestNormalizeExtractedResult_CLPDoesNotUseReferenceNumberAsAmount(t *testing.T) {
	raw := &RawExtraction{
		EmailKind:     "receipt",
		AmountDecimal: strPtr("100.00"),
		Currency:      strPtr("CLP"),
	}
	result, err := NormalizeExtractedResult(raw, "{}", ExtractInput{
		CleanedText:   "Total CLP 100.00. Order 100000",
		ReceivedAtUtc: "2026-11-22T03:20:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Currency != "CLP" || result.AmountMinor != 100 {
		t.Fatalf("expected CLP 100, got %s %d", result.Currency, result.AmountMinor)
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

func strPtr(s string) *string { return &s }

func TestBuildEnrichmentPrompt_AndNormalizeEnrichedResult(t *testing.T) {
	country := "ID"
	currency := "IDR"
	input := EnrichInput{
		Emails: []EnrichEmailSource{
			{
				EmailID:       "e1",
				FromAddress:   "alerts@bank.co.id",
				Subject:       "Card Charge IDR 75,000",
				ReceivedAtUtc: "2026-11-23T05:15:00Z",
				EffectiveRole: "card_charge",
				CleanedText:   "Transaksi kartu sebesar IDR 75.000 di GOPAY*GOFOOD pada 23-11-2026 12:14:00 WIB",
			},
			{
				EmailID:       "e2",
				FromAddress:   "receipt@gofood.co.id",
				Subject:       "Sate Khas Senayan Order",
				ReceivedAtUtc: "2026-11-23T05:48:00Z",
				EffectiveRole: "receipt",
				CleanedText:   "Sate Khas Senayan: 10x Sate Ayam, 1x Nasi Putih, 1x Es Teh Manis",
			},
		},
		UserCountry:        &country,
		CountryPromptRules: []string{"- Treat IDR amounts as zero-decimal rupiah."},
		UserTimezone:       "Asia/Jakarta",
		UserCurrency:       &currency,
		Categories:         []string{"Dining Out", "Groceries", "Other"},
		Tags:               []string{"Work", "Weekend"},
	}

	sysPrompt, userPrompt := BuildEnrichmentPrompt(input)
	for _, want := range []string{"card_charge", "receipt", "merchant", "category", "tags", "note", "card_charge"} {
		if !strings.Contains(strings.ToLower(sysPrompt), want) {
			t.Fatalf("expected system prompt to contain %q, got:\n%s", want, sysPrompt)
		}
	}
	for _, want := range []string{"Email 1", "Email 2", "alerts@bank.co.id", "GOPAY*GOFOOD", "Sate Khas Senayan", "Dining Out, Groceries, Other", "Work, Weekend", "User-Country: ID", "User-Timezone: Asia/Jakarta", "Default-Currency: IDR"} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("expected user prompt to contain %q, got:\n%s", want, userPrompt)
		}
	}

	raw, cleaned, err := ParseAndValidateEnrichment(`{
		"merchant": "  Sate Khas Senayan ",
		"category": "dining out",
		"tags": ["Weekend", "GoFood"],
		"note": " 10x Sate Ayam, Nasi Putih, Es Teh Manis via GoFood ",
		"transaction_date": "2026-11-23",
		"transaction_time": "12:14:00",
		"timezone_offset": null
	}`)
	if err != nil {
		t.Fatalf("parse enrichment: %v", err)
	}
	res, err := NormalizeEnrichedResult(raw, cleaned, input)
	if err != nil {
		t.Fatalf("normalize enrichment: %v", err)
	}
	if res.Merchant != "Sate Khas Senayan" {
		t.Fatalf("expected trimmed merchant, got %q", res.Merchant)
	}
	if res.Category != "Dining Out" {
		t.Fatalf("expected canonical casing Dining Out, got %q", res.Category)
	}
	if res.Note == nil || *res.Note != "10x Sate Ayam, Nasi Putih, Es Teh Manis via GoFood" {
		t.Fatalf("unexpected note: %v", res.Note)
	}
	if res.OccurredAtUtc == nil || *res.OccurredAtUtc != "2026-11-23T05:14:00Z" {
		t.Fatalf("expected 2026-11-23T05:14:00Z, got %v", res.OccurredAtUtc)
	}
	if !slices.Equal(res.Tags, []string{"Weekend", "GoFood"}) {
		t.Fatalf("expected tags [Weekend GoFood], got %v", res.Tags)
	}
}

func TestNormalizeEnrichedResult_CoercesUnknownAndNullCategoryToOther(t *testing.T) {
	input := EnrichInput{
		UserTimezone: "UTC",
		Categories:   []string{"Dining", "Other"},
	}
	for _, jsonBody := range []string{
		`{"merchant":"Corner Cafe","category":"Fast Food","tags":[],"note":null,"transaction_date":null,"transaction_time":null,"timezone_offset":null}`,
		`{"merchant":"Corner Cafe","category":null,"tags":[],"note":null,"transaction_date":null,"transaction_time":null,"timezone_offset":null}`,
	} {
		raw, cleaned, err := ParseAndValidateEnrichment(jsonBody)
		if err != nil {
			t.Fatalf("expected lenient category parse, got err=%v for %s", err, jsonBody)
		}
		res, err := NormalizeEnrichedResult(raw, cleaned, input)
		if err != nil {
			t.Fatalf("normalize failed: %v", err)
		}
		if res.Category != "Other" {
			t.Fatalf("expected category coerced to Other, got %q", res.Category)
		}
		if res.Tags == nil || len(res.Tags) != 0 {
			t.Fatalf("expected non-nil empty tags slice, got %#v", res.Tags)
		}
	}
}

func TestNormalizeEnrichedResult_DeduplicatesAndMatchesExistingTags(t *testing.T) {
	input := EnrichInput{
		UserTimezone: "UTC",
		Categories:   []string{"Dining", "Other"},
		Tags:         []string{"Work", "Weekend"},
	}
	raw, cleaned, err := ParseAndValidateEnrichment(`{
		"merchant": "Noodle House",
		"category": "Dining",
		"tags": ["weekend", " GoFood ", "Weekend", "  ", "gofood"],
		"note": null,
		"transaction_date": null,
		"transaction_time": null,
		"timezone_offset": null
	}`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	res, err := NormalizeEnrichedResult(raw, cleaned, input)
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if !slices.Equal(res.Tags, []string{"Weekend", "GoFood"}) {
		t.Fatalf("expected [Weekend GoFood], got %v", res.Tags)
	}
}

func TestParseStrictExtractedDateTimeToUtc(t *testing.T) {
	if got := ParseStrictExtractedDateTimeToUtc(strPtr("2026-11-23"), strPtr("12:14"), nil, "Asia/Jakarta"); got == nil || *got != "2026-11-23T05:14:00Z" {
		t.Fatalf("expected 2026-11-23T05:14:00Z, got %v", got)
	}
	if got := ParseStrictExtractedDateTimeToUtc(strPtr("2026-11-23"), strPtr("12:14:45"), strPtr("+08:00"), "UTC"); got == nil || *got != "2026-11-23T04:14:45Z" {
		t.Fatalf("expected 2026-11-23T04:14:45Z with offset, got %v", got)
	}
	if got := ParseStrictExtractedDateTimeToUtc(strPtr("2026-11-23"), strPtr("09:05"), nil, "Local"); got == nil || *got != "2026-11-23T09:05:00Z" {
		t.Fatalf("expected Local timezone treated as UTC, got %v", got)
	}

	nilCases := []struct {
		name         string
		d, tm, tzOff *string
		userTz       string
	}{
		{"nil time", strPtr("2026-11-23"), nil, nil, "Asia/Jakarta"},
		{"blank time", strPtr("2026-11-23"), strPtr("  "), nil, "Asia/Jakarta"},
		{"nil date", nil, strPtr("12:14"), nil, "Asia/Jakarta"},
		{"invalid calendar date", strPtr("2026-02-30"), strPtr("12:14"), nil, "UTC"},
		{"invalid clock time", strPtr("2026-11-23"), strPtr("25:99"), nil, "UTC"},
	}
	for _, tc := range nilCases {
		if got := ParseStrictExtractedDateTimeToUtc(tc.d, tc.tm, tc.tzOff, tc.userTz); got != nil {
			t.Fatalf("%s: expected nil, got %q", tc.name, *got)
		}
	}
}

func TestParseRawExtractedDateTimeToUtc_HandlesSpaceAndRFC3339Formats(t *testing.T) {
	if got := ParseRawExtractedDateTimeToUtc(strPtr("2026-11-24 19:42:34+07:00"), nil, nil, "UTC", "2026-11-25T00:00:00Z"); got != "2026-11-24T12:42:34Z" {
		t.Fatalf("expected 2026-11-24T12:42:34Z, got %s", got)
	}
	if got := ParseRawExtractedDateTimeToUtc(strPtr("2026-11-23T20:14:00+07:00"), nil, nil, "UTC", "2026-11-25T00:00:00Z"); got != "2026-11-23T13:14:00Z" {
		t.Fatalf("expected 2026-11-23T13:14:00Z, got %s", got)
	}
	if got := ParseRawExtractedDateTimeToUtc(strPtr("2026-11-23"), strPtr("09:05"), nil, "Local", "2026-11-25T00:00:00Z"); got != "2026-11-23T09:05:00Z" {
		t.Fatalf("expected Local timezone safely resolved as UTC 2026-11-23T09:05:00Z, got %s", got)
	}
}

func TestBuildExtractionPrompt_GenericOutflowAndPositiveAmountCategory(t *testing.T) {
	sysPrompt, _ := BuildExtractionPrompt(ExtractInput{
		ReceivedAtUtc: "2026-11-24T00:00:00Z",
		UserTimezone:  "UTC",
		Categories:    []string{"Dining Out", "Other"},
	})
	lower := strings.ToLower(sysPrompt)
	for _, want := range []string{"transfer", "qr", "tip", "positive amount"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("expected generic outflow prompt rule containing %q, got:\n%s", want, sysPrompt)
		}
	}
	if strings.Contains(lower, "(or null for non_transaction)") {
		t.Fatalf("prompt must not force null category for non_transaction when an amount is present")
	}
	for _, banned := range []string{"qris", "bca", "jenius", "grab", "shopee", "idr"} {
		if strings.Contains(lower, banned) {
			t.Fatalf("baseline prompt must remain country-agnostic (found %q)", banned)
		}
	}
}

func TestNormalizeExtractedResult_NonTransactionDraftPreservationAndZeroFallback(t *testing.T) {
	cur := "IDR"
	raw, cleaned, err := ParseAndValidateExtraction(`{
		"email_kind": "non_transaction",
		"amount_decimal": "5000",
		"currency": "IDR",
		"transaction_date": "2026-11-23 20:14:00+07:00",
		"transaction_time": null,
		"timezone_offset": null,
		"merchant": "   ",
		"category": "Transportation",
		"card_last4": "xxxx-6039",
		"payment_method": "GrabPay",
		"bank_account_hint": "****1234",
		"matched_account_id": null,
		"account_type_guess": "digital_wallet",
		"reference_code": "A-123"
	}`, []string{"Transportation", "Other"})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	res, err := NormalizeExtractedResult(raw, cleaned, ExtractInput{
		CleanedText:   "Grab Tips RP 5000",
		ReceivedAtUtc: "2026-11-23T14:00:00Z",
		UserTimezone:  "Asia/Jakarta",
		UserCurrency:  &cur,
		Categories:    []string{"Transportation", "Other"},
	})
	if err != nil {
		t.Fatalf("unexpected error preserving non_transaction draft: %v", err)
	}
	if res.AmountMinor != 5000 || res.Currency != "IDR" || res.OccurredAtUtc != "2026-11-23T13:14:00Z" {
		t.Fatalf("expected preserved amount/currency/utc, got %+v", res)
	}
	if res.MerchantRaw != "" {
		t.Fatalf("expected blank MerchantRaw stored empty (for NULL) instead of synthetic fallback, got %q", res.MerchantRaw)
	}
	if res.Category != "Transportation" || res.CardLast4 == nil || *res.CardLast4 != "6039" || res.AccountTypeGuess == nil || *res.AccountTypeGuess != "digital_wallet" {
		t.Fatalf("expected draft category/card/account_type preserved, got %+v", res)
	}

	// Non-transaction with null amount or invalid currency must return AmountMinor=0 with nil error.
	for _, tc := range []struct {
		name string
		raw  *RawExtraction
	}{
		{
			name: "null amount",
			raw:  &RawExtraction{EmailKind: "non_transaction", AmountDecimal: nil, Currency: strPtr("IDR")},
		},
		{
			name: "invalid currency",
			raw:  &RawExtraction{EmailKind: "non_transaction", AmountDecimal: strPtr("100"), Currency: strPtr("UNKNOWN")},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			noAmt, err := NormalizeExtractedResult(tc.raw, `{}`, ExtractInput{
				ReceivedAtUtc: "2026-11-23T14:00:00Z",
				UserTimezone:  "UTC",
			})
			if err != nil {
				t.Fatalf("expected non-fatal zero result on non_transaction (%s), got err=%v", tc.name, err)
			}
			if noAmt.AmountMinor != 0 {
				t.Fatalf("expected AmountMinor=0, got %d", noAmt.AmountMinor)
			}
		})
	}
}

func TestParseAndValidateEnrichment_RejectsInvalidOrMissingMerchant(t *testing.T) {
	for _, bad := range []string{
		"",
		"not json",
		"{}",
		`{"merchant":null,"category":"Other","tags":[]}`,
		`{"merchant":"   ","category":"Other","tags":[]}`,
	} {
		_, _, err := ParseAndValidateEnrichment(bad)
		if !IsExtractionError(err) {
			t.Fatalf("expected ExtractionError for %q, got %T: %v", bad, err, err)
		}
	}
}
