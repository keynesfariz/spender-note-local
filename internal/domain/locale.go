package domain

import "strings"

const DefaultCategory = "Other"

var DefaultCategories = []string{
	"Housing",
	"Food",
	"Utilities",
	"Transportation",
	"Healthcare",
	"Entertainment",
	"Dining Out",
	"Shopping",
	"Education",
	"Personal Care",
	"Other",
}

type CountryLocaleInfo struct {
	Code            string   `json:"code"`
	Name            string   `json:"name"`
	Currency        string   `json:"currency"`
	DefaultTimezone string   `json:"defaultTimezone"`
	Timezones       []string `json:"timezones"`
}

func NormalizeCurrencyCode(currency string) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(currency))
	if len(code) != 3 {
		return "", &ValidationError{Message: "Invalid ISO 4217 currency code: " + currency}
	}
	for i := 0; i < 3; i++ {
		if code[i] < 'A' || code[i] > 'Z' {
			return "", &ValidationError{Message: "Invalid ISO 4217 currency code: " + currency}
		}
	}
	return code, nil
}

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }
