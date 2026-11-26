package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"portspendernote/internal/money"
)

type RuntimeError struct {
	Message string
	Cause   error
}

func (e *RuntimeError) Error() string { return e.Message }
func (e *RuntimeError) Unwrap() error { return e.Cause }

type ExtractionError struct {
	Message string
	Cause   error
}

func (e *ExtractionError) Error() string { return e.Message }
func (e *ExtractionError) Unwrap() error { return e.Cause }

func IsRuntimeError(err error) bool {
	var target *RuntimeError
	return errors.As(err, &target)
}

func IsExtractionError(err error) bool {
	var target *ExtractionError
	return errors.As(err, &target)
}

type RawExtraction struct {
	EmailKind        string  `json:"email_kind"`
	AmountDecimal    *string `json:"amount_decimal"`
	Currency         *string `json:"currency"`
	TransactionDate  *string `json:"transaction_date"`
	TransactionTime  *string `json:"transaction_time"`
	TimezoneOffset   *string `json:"timezone_offset"`
	Merchant         *string `json:"merchant"`
	Category         *string `json:"category"`
	PaymentMethod    *string `json:"payment_method"`
	BankAccountHint  *string `json:"bank_account_hint"`
	MatchedAccountID *string `json:"matched_account_id"`
	AccountTypeGuess *string `json:"account_type_guess"`
	ReferenceCode    *string `json:"reference_code"`
}

type ExtractResult struct {
	EmailKind        string
	AmountMinor      int64
	Currency         string
	OccurredAtUtc    string
	MerchantRaw      string
	Category         string
	PaymentMethodRaw *string
	BankAccountHint  *string
	MatchedAccountID *string
	AccountTypeGuess *string
	ReferenceCode    *string
	RawJSON          string
}

var (
	thinkBlockRe     = regexp.MustCompile(`(?is)<think>.*?</think>`)
	codeFenceStartRe = regexp.MustCompile("^```(?:json)?\\s*")
	codeFenceEndRe   = regexp.MustCompile("\\s*```$")
	isoCurrencyRe    = regexp.MustCompile(`^[A-Z]{3}$`)
	isoDateOnlyRe    = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	isoTimeOnlyRe    = regexp.MustCompile(`^\d{2}:\d{2}(:\d{2})?$`)
	tzOffsetRe       = regexp.MustCompile(`^(Z|[+-]\d{2}:\d{2})$`)
	nonDigitRe       = regexp.MustCompile(`\D`)
	leadingCurRe     = regexp.MustCompile(`^(?:[A-Za-z]{1,4}|\$|€|£|¥|₩|₫|₱|₹|฿)\s*`)
	trailingCurRe    = regexp.MustCompile(`\s*(?:[A-Za-z]{2,4})$`)
	dotGroupedRe     = regexp.MustCompile(`^(-)?(\d{1,3}(?:\.\d{3})+)(?:,(\d+))?$`)
	commaGroupedRe   = regexp.MustCompile(`^(-)?(\d{1,3}(?:,\d{3})+)(?:\.(\d+))?$`)
	commaDecimalRe   = regexp.MustCompile(`^(-?\d+),(\d+)$`)
	zeroFractionRe   = regexp.MustCompile(`^(-?\d+)\.0+$`)
	plainDecimalRe   = regexp.MustCompile(`^(-)?(\d+)(?:\.(\d+))?$`)
	truncZeroDecRe   = regexp.MustCompile(`^(\d{1,3}(?:[.,]\d{3})*)\.(\d{1,2})$`)
	stripGrouping    = strings.NewReplacer(".", "", ",", "")
)

func StripThinkBlocks(text string) string {
	cleaned := strings.TrimSpace(thinkBlockRe.ReplaceAllString(text, ""))
	cleaned = codeFenceStartRe.ReplaceAllString(cleaned, "")
	cleaned = codeFenceEndRe.ReplaceAllString(cleaned, "")
	return strings.TrimSpace(cleaned)
}

func ParseAndValidateExtraction(rawText string, categories []string) (*RawExtraction, string, error) {
	cleaned := StripThinkBlocks(rawText)
	if cleaned == "" {
		return nil, "", &ExtractionError{Message: "LLM output is empty"}
	}

	var raw RawExtraction
	dec := json.NewDecoder(bytes.NewReader([]byte(cleaned)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return nil, "", &ExtractionError{Message: fmt.Sprintf("LLM output is not valid JSON or failed schema validation: %v", err), Cause: err}
	}

	switch raw.EmailKind {
	case "card_charge", "receipt", "non_transaction":
	default:
		return nil, "", &ExtractionError{Message: fmt.Sprintf("Invalid email_kind: %q", raw.EmailKind)}
	}

	validCategories := ResolveCategories(categories)
	if raw.Category != nil && !slices.Contains(validCategories, *raw.Category) {
		return nil, "", &ExtractionError{Message: fmt.Sprintf("Category %q is not in allowed category list", *raw.Category)}
	}

	if raw.AccountTypeGuess != nil {
		switch *raw.AccountTypeGuess {
		case "credit_card", "debit_card", "digital_wallet":
		default:
			return nil, "", &ExtractionError{Message: fmt.Sprintf("Invalid account_type_guess: %q", *raw.AccountTypeGuess)}
		}
	}

	return &raw, cleaned, nil
}

func normalizeDecimalAmountString(raw string, exponent int) string {
	trimmed := strings.TrimSpace(raw)
	trimmed = leadingCurRe.ReplaceAllString(trimmed, "")
	trimmed = trailingCurRe.ReplaceAllString(trimmed, "")
	trimmed = strings.TrimSpace(trimmed)

	if exponent > 0 && plainDecimalRe.MatchString(trimmed) {
		return trimmed
	}
	grouped := dotGroupedRe.FindStringSubmatch(trimmed)
	if grouped == nil {
		grouped = commaGroupedRe.FindStringSubmatch(trimmed)
	}
	if grouped != nil {
		trimmed = grouped[1] + stripGrouping.Replace(grouped[2])
		if grouped[3] != "" {
			trimmed += "." + grouped[3]
		}
	} else if m := commaDecimalRe.FindStringSubmatch(trimmed); m != nil {
		trimmed = m[1] + "." + m[2]
	}
	if exponent == 0 {
		trimmed = zeroFractionRe.ReplaceAllString(trimmed, "$1")
	}
	return trimmed
}

func recoverTruncatedZeroDecimalAmount(rawAmountDecimal string, currency string, cleanedText string) string {
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case "IDR", "JPY", "KRW", "VND":
	default:
		return rawAmountDecimal
	}
	trimmed := strings.TrimSpace(rawAmountDecimal)
	m := truncZeroDecRe.FindStringSubmatch(trimmed)
	if m == nil {
		return rawAmountDecimal
	}
	intDigits := stripGrouping.Replace(m[1])
	fracThree := m[2]
	for len(fracThree) < 3 {
		fracThree += "0"
	}
	candidateInt := intDigits + fracThree
	candidates := []string{
		regexp.QuoteMeta(intDigits + "." + fracThree),
		regexp.QuoteMeta(intDigits + "," + fracThree),
		regexp.QuoteMeta(candidateInt),
	}
	pattern := regexp.MustCompile(`(?:^|[^0-9])(?:` + strings.Join(candidates, "|") + `)(?:$|[^0-9])`)
	if pattern.MatchString(cleanedText) {
		return candidateInt
	}
	return rawAmountDecimal
}

func parseAmountToMinor(rawInput string, currency string) (int64, error) {
	exponent := money.CurrencyExponent(currency)
	raw := strings.TrimSpace(rawInput)
	if raw == "" {
		return 0, errors.New("amount cannot be empty")
	}
	cleaned := normalizeDecimalAmountString(raw, exponent)
	m := plainDecimalRe.FindStringSubmatch(cleaned)
	if m == nil {
		return 0, fmt.Errorf("invalid decimal amount: %s", raw)
	}
	if m[1] != "" {
		return 0, errors.New("amount cannot be negative")
	}
	whole := m[2]
	frac := m[3]
	if len(frac) > exponent {
		return 0, fmt.Errorf("too many decimal places for %s (max %d)", currency, exponent)
	}
	for len(frac) < exponent {
		frac += "0"
	}
	minor, err := strconv.ParseInt(whole+frac, 10, 64)
	if err != nil || minor <= 0 {
		return 0, fmt.Errorf("amount must be greater than zero: %s", raw)
	}
	return minor, nil
}

func formatUtcIsoSeconds(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

func parseEmbeddedTimestamp(raw string, tzOffset *string, userTimezone string) (string, bool) {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return "", false
	}
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02 15:04:05-0700",
		"2006-01-02 15:04Z07:00",
		"2006-01-02 15:04-0700",
		"2006-01-02T15:04:05-0700",
	} {
		if parsed, err := time.Parse(layout, clean); err == nil {
			return formatUtcIsoSeconds(parsed), true
		}
	}

	loc := resolveLocationOrUTC(userTimezone)
	if tzOffset != nil && tzOffsetRe.MatchString(strings.TrimSpace(*tzOffset)) {
		cleanOff := strings.TrimSpace(*tzOffset)
		for _, naive := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02 15:04", "2006-01-02T15:04"} {
			if parsedNaive, err := time.Parse(naive, clean); err == nil {
				if parsed, err := time.Parse(time.RFC3339, parsedNaive.Format("2006-01-02T15:04:05")+cleanOff); err == nil {
					return formatUtcIsoSeconds(parsed), true
				}
			}
		}
	}
	for _, naive := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02 15:04", "2006-01-02T15:04"} {
		if parsed, err := time.ParseInLocation(naive, clean, loc); err == nil {
			return formatUtcIsoSeconds(parsed), true
		}
	}
	return "", false
}

// ParseRawExtractedDateTimeToUtc converts extracted date/time/offset fields into
// canonical YYYY-MM-DDTHH:MM:SSZ timestamps, supporting RFC3339 and
// space-separated ISO timestamps alongside split date/time fields.
func ParseRawExtractedDateTimeToUtc(txDate, txTime, tzOffset *string, userTimezone string, receivedAtUtc string) string {
	fallback := receivedAtUtc
	if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(receivedAtUtc)); err == nil {
		fallback = formatUtcIsoSeconds(parsed)
	}

	if txDate != nil {
		if ts, ok := parseEmbeddedTimestamp(*txDate, tzOffset, userTimezone); ok {
			return ts
		}
	}

	if strict := ParseStrictExtractedDateTimeToUtc(txDate, txTime, tzOffset, userTimezone); strict != nil {
		return *strict
	}

	if txDate == nil || !isoDateOnlyRe.MatchString(strings.TrimSpace(*txDate)) {
		return fallback
	}
	cleanDate := strings.TrimSpace(*txDate)
	loc := resolveLocationOrUTC(userTimezone)

	if recTime, err := time.Parse(time.RFC3339, fallback); err == nil {
		if recTime.In(loc).Format("2006-01-02") == cleanDate {
			return fallback
		}
	}
	if parsed, err := time.ParseInLocation("2006-01-02T15:04:05", cleanDate+"T12:00:00", loc); err == nil {
		return formatUtcIsoSeconds(parsed)
	}
	return fallback
}

func trimNonEmpty(raw *string) *string {
	if raw == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func resolveNormalizedCurrency(raw *RawExtraction, input ExtractInput) string {
	rawCurrency := ""
	if raw.Currency != nil && strings.TrimSpace(*raw.Currency) != "" {
		rawCurrency = strings.TrimSpace(*raw.Currency)
	} else if input.UserCurrency != nil && strings.TrimSpace(*input.UserCurrency) != "" {
		rawCurrency = strings.TrimSpace(*input.UserCurrency)
	}
	return strings.ToUpper(rawCurrency)
}

func resolveNormalizedCategory(rawCategory *string, categories []string) string {
	validCategories := ResolveCategories(categories)
	if rawCategory != nil {
		trimmed := strings.TrimSpace(*rawCategory)
		for _, c := range validCategories {
			if strings.EqualFold(c, trimmed) {
				return c
			}
		}
	}
	if slices.Contains(validCategories, "Other") {
		return "Other"
	}
	return validCategories[0]
}

func NormalizeExtractedResult(raw *RawExtraction, rawJSON string, input ExtractInput) (*ExtractResult, error) {
	if raw.EmailKind == "non_transaction" {
		currency := resolveNormalizedCurrency(raw, input)
		var amountMinor int64
		if isoCurrencyRe.MatchString(currency) && raw.AmountDecimal != nil && strings.TrimSpace(*raw.AmountDecimal) != "" {
			resolvedAmount := recoverTruncatedZeroDecimalAmount(*raw.AmountDecimal, currency, input.CleanedText)
			if parsedMinor, err := parseAmountToMinor(resolvedAmount, currency); err == nil && parsedMinor > 0 {
				amountMinor = parsedMinor
			}
		} else if !isoCurrencyRe.MatchString(currency) {
			currency = ""
		}

		merchantRaw := ""
		if raw.Merchant != nil {
			merchantRaw = strings.TrimSpace(*raw.Merchant)
		}

		category := ""
		if amountMinor > 0 {
			category = resolveNormalizedCategory(raw.Category, input.Categories)
		}

		occurredAtUtc := ""
		if amountMinor > 0 || input.ReceivedAtUtc != "" {
			occurredAtUtc = ParseRawExtractedDateTimeToUtc(
				raw.TransactionDate,
				raw.TransactionTime,
				raw.TimezoneOffset,
				input.UserTimezone,
				input.ReceivedAtUtc,
			)
		}

		return &ExtractResult{
			EmailKind:        "non_transaction",
			AmountMinor:      amountMinor,
			Currency:         currency,
			OccurredAtUtc:    occurredAtUtc,
			MerchantRaw:      merchantRaw,
			Category:         category,
			PaymentMethodRaw: trimNonEmpty(raw.PaymentMethod),
			BankAccountHint:  trimNonEmpty(raw.BankAccountHint),
			MatchedAccountID: trimNonEmpty(raw.MatchedAccountID),
			AccountTypeGuess: trimNonEmpty(raw.AccountTypeGuess),
			ReferenceCode:    trimNonEmpty(raw.ReferenceCode),
			RawJSON:          rawJSON,
		}, nil
	}

	currency := resolveNormalizedCurrency(raw, input)
	if currency == "" {
		return nil, &ExtractionError{Message: "Missing currency for transaction extraction"}
	}
	if !isoCurrencyRe.MatchString(currency) {
		return nil, &ExtractionError{Message: fmt.Sprintf("Invalid currency in extraction: %s", currency)}
	}

	if raw.AmountDecimal == nil || strings.TrimSpace(*raw.AmountDecimal) == "" {
		return nil, &ExtractionError{Message: "Missing amount_decimal for transaction extraction"}
	}
	resolvedAmount := recoverTruncatedZeroDecimalAmount(*raw.AmountDecimal, currency, input.CleanedText)
	amountMinor, err := parseAmountToMinor(resolvedAmount, currency)
	if err != nil {
		return nil, &ExtractionError{Message: fmt.Sprintf("Invalid amount_decimal %q for %s: %v", *raw.AmountDecimal, currency, err), Cause: err}
	}

	occurredAtUtc := ParseRawExtractedDateTimeToUtc(
		raw.TransactionDate,
		raw.TransactionTime,
		raw.TimezoneOffset,
		input.UserTimezone,
		input.ReceivedAtUtc,
	)

	merchantRaw := "Unknown Merchant"
	if raw.Merchant != nil && strings.TrimSpace(*raw.Merchant) != "" {
		merchantRaw = strings.TrimSpace(*raw.Merchant)
	}

	category := resolveNormalizedCategory(raw.Category, input.Categories)

	return &ExtractResult{
		EmailKind:        raw.EmailKind,
		AmountMinor:      amountMinor,
		Currency:         currency,
		OccurredAtUtc:    occurredAtUtc,
		MerchantRaw:      merchantRaw,
		Category:         category,
		PaymentMethodRaw: trimNonEmpty(raw.PaymentMethod),
		BankAccountHint:  trimNonEmpty(raw.BankAccountHint),
		MatchedAccountID: trimNonEmpty(raw.MatchedAccountID),
		AccountTypeGuess: trimNonEmpty(raw.AccountTypeGuess),
		ReferenceCode:    trimNonEmpty(raw.ReferenceCode),
		RawJSON:          rawJSON,
	}, nil
}
