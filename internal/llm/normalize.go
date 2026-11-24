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
	CardLast4        *string `json:"card_last4"`
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
	CardLast4        *string
	PaymentMethodRaw *string
	BankAccountHint  *string
	MatchedAccountID *string
	AccountTypeGuess *string
	ReferenceCode    *string
	RawJSON          string
}

var (
	thinkBlockRe      = regexp.MustCompile(`(?is)<think>.*?</think>`)
	codeFenceStartRe  = regexp.MustCompile("^```(?:json)?\\s*")
	codeFenceEndRe    = regexp.MustCompile("\\s*```$")
	isoCurrencyRe     = regexp.MustCompile(`^[A-Z]{3}$`)
	isoDateOnlyRe     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	isoTimeOnlyRe     = regexp.MustCompile(`^\d{2}:\d{2}(:\d{2})?$`)
	tzOffsetRe        = regexp.MustCompile(`^(Z|[+-]\d{2}:\d{2})$`)
	nonDigitRe        = regexp.MustCompile(`\D`)
	leadingCurRe      = regexp.MustCompile(`^(?:[A-Za-z]{1,4}|\$|€|£|¥|₩|₫|₱|₹|฿)\s*`)
	trailingCurRe     = regexp.MustCompile(`\s*(?:[A-Za-z]{2,4})$`)
	dotGroupedRe      = regexp.MustCompile(`^(-)?(\d{1,3}(?:\.\d{3})+)(?:,(0+))?$`)
	commaGroupedRe    = regexp.MustCompile(`^(-)?(\d[\d,]*)(?:\.(0+))?$`)
	commaZeroCentsRe  = regexp.MustCompile(`^(-)?(\d+),(0+)$`)
	plainDecimalRe    = regexp.MustCompile(`^(-)?(\d+)(?:\.(\d+))?$`)
	truncZeroDecRe    = regexp.MustCompile(`^(\d{1,3}(?:[.,]\d{3})*)\.(\d{1,2})$`)
)

var currencyExponents = map[string]int{
	"JPY": 0,
	"KRW": 0,
	"IDR": 0,
	"VND": 0,
	"KWD": 3,
	"BHD": 3,
	"OMR": 3,
	"JOD": 3,
	"TND": 3,
}

func getCurrencyExponent(currency string) int {
	code := strings.ToUpper(strings.TrimSpace(currency))
	if exp, ok := currencyExponents[code]; ok {
		return exp
	}
	return 2
}

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

	if exponent == 0 {
		if m := dotGroupedRe.FindStringSubmatch(trimmed); m != nil {
			return m[1] + strings.ReplaceAll(m[2], ".", "")
		}
		if m := commaGroupedRe.FindStringSubmatch(trimmed); m != nil {
			return m[1] + strings.ReplaceAll(m[2], ",", "")
		}
		if m := commaZeroCentsRe.FindStringSubmatch(trimmed); m != nil {
			return m[1] + m[2]
		}
	}
	return strings.ReplaceAll(trimmed, ",", "")
}

func recoverTruncatedZeroDecimalAmount(rawAmountDecimal string, currency string, cleanedText string) string {
	if getCurrencyExponent(currency) != 0 {
		return rawAmountDecimal
	}
	trimmed := strings.TrimSpace(rawAmountDecimal)
	m := truncZeroDecRe.FindStringSubmatch(trimmed)
	if m == nil {
		return rawAmountDecimal
	}
	intDigits := strings.NewReplacer(".", "", ",", "").Replace(m[1])
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
	exponent := getCurrencyExponent(currency)
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

func parseRawExtractedDateTimeToUtc(txDate, txTime, tzOffset *string, userTimezone string, receivedAtUtc string) string {
	fallback := receivedAtUtc
	if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(receivedAtUtc)); err == nil {
		fallback = formatUtcIsoSeconds(parsed)
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

func normalizeCardLast4(raw *string) *string {
	if raw == nil {
		return nil
	}
	digits := nonDigitRe.ReplaceAllString(*raw, "")
	if len(digits) != 4 {
		return nil
	}
	return &digits
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

func NormalizeExtractedResult(raw *RawExtraction, rawJSON string, input ExtractInput) (*ExtractResult, error) {
	if raw.EmailKind == "non_transaction" {
		return &ExtractResult{
			EmailKind: "non_transaction",
			RawJSON:   rawJSON,
		}, nil
	}

	rawCurrency := ""
	if raw.Currency != nil && strings.TrimSpace(*raw.Currency) != "" {
		rawCurrency = strings.TrimSpace(*raw.Currency)
	} else if input.UserCurrency != nil && strings.TrimSpace(*input.UserCurrency) != "" {
		rawCurrency = strings.TrimSpace(*input.UserCurrency)
	}
	if rawCurrency == "" {
		return nil, &ExtractionError{Message: "Missing currency for transaction extraction"}
	}
	currency := strings.ToUpper(rawCurrency)
	if !isoCurrencyRe.MatchString(currency) {
		return nil, &ExtractionError{Message: fmt.Sprintf("Invalid currency in extraction: %s", rawCurrency)}
	}

	if raw.AmountDecimal == nil || strings.TrimSpace(*raw.AmountDecimal) == "" {
		return nil, &ExtractionError{Message: "Missing amount_decimal for transaction extraction"}
	}
	resolvedAmount := recoverTruncatedZeroDecimalAmount(*raw.AmountDecimal, currency, input.CleanedText)
	amountMinor, err := parseAmountToMinor(resolvedAmount, currency)
	if err != nil {
		return nil, &ExtractionError{Message: fmt.Sprintf("Invalid amount_decimal %q for %s: %v", *raw.AmountDecimal, currency, err), Cause: err}
	}

	occurredAtUtc := parseRawExtractedDateTimeToUtc(
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

	validCategories := ResolveCategories(input.Categories)
	category := validCategories[0]
	if raw.Category != nil && slices.Contains(validCategories, *raw.Category) {
		category = *raw.Category
	} else if slices.Contains(validCategories, "Other") {
		category = "Other"
	}

	return &ExtractResult{
		EmailKind:        raw.EmailKind,
		AmountMinor:      amountMinor,
		Currency:         currency,
		OccurredAtUtc:    occurredAtUtc,
		MerchantRaw:      merchantRaw,
		Category:         category,
		CardLast4:        normalizeCardLast4(raw.CardLast4),
		PaymentMethodRaw: trimNonEmpty(raw.PaymentMethod),
		BankAccountHint:  trimNonEmpty(raw.BankAccountHint),
		MatchedAccountID: trimNonEmpty(raw.MatchedAccountID),
		AccountTypeGuess: trimNonEmpty(raw.AccountTypeGuess),
		ReferenceCode:    trimNonEmpty(raw.ReferenceCode),
		RawJSON:          rawJSON,
	}, nil
}
