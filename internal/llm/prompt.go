package llm

import (
	"fmt"
	"strings"
	"time"
)

type AccountContext struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Identifiers []string `json:"identifiers"`
}

type ExtractInput struct {
	EmailID            string
	FromAddress        string
	Subject            string
	CleanedText        string
	ReceivedAtUtc      string
	UserCountry        *string
	CountryPromptRules []string
	UserTimezone       string
	UserCurrency       *string
	Categories         []string
	Accounts           []AccountContext
}

func resolveTimezoneOffsetString(tz string, referenceUtc string) string {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		return ""
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return ""
	}
	ref := time.Now().UTC()
	if strings.TrimSpace(referenceUtc) != "" {
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(referenceUtc)); err == nil {
			ref = parsed
		}
	}
	_, offsetSec := ref.In(loc).Zone()
	if offsetSec == 0 {
		return "Z"
	}
	sign := "+"
	if offsetSec < 0 {
		sign = "-"
		offsetSec = -offsetSec
	}
	hours := offsetSec / 3600
	mins := (offsetSec % 3600) / 60
	return fmt.Sprintf("%s%02d:%02d", sign, hours, mins)
}

func buildCountryPromptRules(userTimezone string, referenceUtc string, countryRules []string) []string {
	offset := resolveTimezoneOffsetString(userTimezone, referenceUtc)
	offsetHint := userTimezone
	if offset != "" {
		offsetHint = fmt.Sprintf("%s (%s)", userTimezone, offset)
	}
	rules := []string{
		`- Always return transaction_date as "YYYY-MM-DD" WITH transaction_time ("HH:mm:ss" or "HH:mm") AND timezone_offset ("+HH:mm", "-HH:mm", or "Z") whenever a transaction timestamp is present in the email.`,
		fmt.Sprintf(`- Derive timezone_offset from any explicit timezone or abbreviation shown in the email first (e.g. "UTC" -> "Z", "PST" -> "-08:00"). If the email omits a timezone, use the user's configured timezone %s combined with the email's timestamp.`, offsetHint),
	}
	for _, r := range countryRules {
		if trimmed := strings.TrimSpace(r); trimmed != "" {
			rules = append(rules, trimmed)
		}
	}
	return rules
}

func BuildExtractionPrompt(input ExtractInput) (systemPrompt string, userPrompt string) {
	validCategories := ResolveCategories(input.Categories)
	countryAndTzRules := buildCountryPromptRules(input.UserTimezone, input.ReceivedAtUtc, input.CountryPromptRules)

	sysLines := []string{
		"You are a strict financial email extraction engine. Output ONLY a single JSON object matching the required schema.",
		"Classify email_kind as:",
		`- "card_charge": bank/card authorization or charge alert.`,
		`- "receipt": merchant purchase receipt or order confirmation.`,
		`- "non_transaction": credit card or bank billing statements, e-statements, monthly bill summaries (emails showing Total Billing, Minimum Payment, Statement Balance, or a future Payment Due Date), bill payments, shipping updates, OTPs, security alerts, promotional emails, and refunds. Never classify a billing statement or its due date as a card_charge.`,
		"Rules:",
		`- Return amount_decimal as a plain unformatted number string without thousand separators (e.g. "12.50" for "$12.50", or "1250" for "1,250").`,
		`- Return currency as a 3-letter ISO 4217 code (e.g. "USD", "EUR", "GBP").`,
	}
	sysLines = append(sysLines, countryAndTzRules...)
	sysLines = append(sysLines,
		fmt.Sprintf("- Choose category strictly from: %s (or null for non_transaction).", strings.Join(validCategories, ", ")),
		"- Return card_last4 as 4 digits if present, otherwise null.",
		`- Return payment_method as the raw payment method or wallet name shown in the email (e.g. "Apple Pay", "PayPal") if present, otherwise null.`,
		`- Return bank_account_hint as the bank account number or masked suffix (e.g. "****5678") if present, otherwise null.`,
		"- If Known-Accounts are provided and the email clearly identifies one of them, return its id in matched_account_id; otherwise return null.",
		`- Return account_type_guess as "credit_card", "debit_card", or "digital_wallet" if the email indicates the payment account type, otherwise null.`,
		"- Return reference_code if present, otherwise null.",
	)

	userLines := []string{
		fmt.Sprintf("From: %s", input.FromAddress),
		fmt.Sprintf("Subject: %s", input.Subject),
		fmt.Sprintf("Received-At-UTC: %s", input.ReceivedAtUtc),
	}
	if input.UserCountry != nil && strings.TrimSpace(*input.UserCountry) != "" {
		userLines = append(userLines, fmt.Sprintf("User-Country: %s", strings.TrimSpace(*input.UserCountry)))
	}
	userLines = append(userLines, fmt.Sprintf("User-Timezone: %s", input.UserTimezone))
	if input.UserCurrency != nil && strings.TrimSpace(*input.UserCurrency) != "" {
		userLines = append(userLines, fmt.Sprintf("Default-Currency: %s", strings.TrimSpace(*input.UserCurrency)))
	}
	if len(input.Accounts) > 0 {
		userLines = append(userLines, "Known-Accounts:")
		for _, a := range input.Accounts {
			idPart := ""
			if len(a.Identifiers) > 0 {
				idPart = " | identifiers: " + strings.Join(a.Identifiers, ", ")
			}
			userLines = append(userLines, fmt.Sprintf("- id=%s | name=%s | type=%s%s", a.ID, a.Name, a.Type, idPart))
		}
	}
	userLines = append(userLines, "", "Email-Body:", input.CleanedText)

	return strings.Join(sysLines, "\n"), strings.Join(userLines, "\n")
}

func BuildExtractionJSONSchema(categories []string, accountIDs []string) map[string]any {
	validCategories := ResolveCategories(categories)
	catEnum := make([]any, 0, len(validCategories)+1)
	for _, c := range validCategories {
		catEnum = append(catEnum, c)
	}
	catEnum = append(catEnum, nil)

	validAccountIDs := make([]any, 0, len(accountIDs)+1)
	for _, id := range accountIDs {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			validAccountIDs = append(validAccountIDs, trimmed)
		}
	}
	var matchedAccountSchema map[string]any
	if len(validAccountIDs) > 0 {
		validAccountIDs = append(validAccountIDs, nil)
		matchedAccountSchema = map[string]any{"enum": validAccountIDs}
	} else {
		matchedAccountSchema = map[string]any{"type": []string{"string", "null"}}
	}

	nullableString := map[string]any{"type": []string{"string", "null"}}

	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"email_kind": map[string]any{
				"enum": []string{"card_charge", "receipt", "non_transaction"},
			},
			"amount_decimal":   nullableString,
			"currency":         nullableString,
			"transaction_date": nullableString,
			"transaction_time": nullableString,
			"timezone_offset":  nullableString,
			"merchant":         nullableString,
			"category": map[string]any{
				"enum": catEnum,
			},
			"card_last4":         nullableString,
			"payment_method":     nullableString,
			"bank_account_hint":  nullableString,
			"matched_account_id": matchedAccountSchema,
			"account_type_guess": map[string]any{
				"enum": []any{"credit_card", "debit_card", "digital_wallet", nil},
			},
			"reference_code": nullableString,
		},
		"required": []string{
			"email_kind",
			"amount_decimal",
			"currency",
			"transaction_date",
			"transaction_time",
			"timezone_offset",
			"merchant",
			"category",
			"card_last4",
			"payment_method",
			"bank_account_hint",
			"matched_account_id",
			"account_type_guess",
			"reference_code",
		},
	}
}
