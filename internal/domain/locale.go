package domain

import "strings"

const DefaultCategory = "Other"

var DefaultCategories = []string{
	"Dining",
	"Groceries",
	"Transport",
	"Shopping",
	"Utilities & Bills",
	"Entertainment",
	"Health",
	"Travel",
	"Subscriptions",
	"Other",
}

type CountryLocaleInfo struct {
	Code            string   `json:"code"`
	Name            string   `json:"name"`
	Currency        string   `json:"currency"`
	DefaultTimezone string   `json:"defaultTimezone"`
	Timezones       []string `json:"timezones"`
}

var CountryCatalog = []CountryLocaleInfo{
	{Code: "ID", Name: "Indonesia", Currency: "IDR", DefaultTimezone: "Asia/Jakarta", Timezones: []string{"Asia/Jakarta", "Asia/Makassar", "Asia/Jayapura"}},
	{Code: "US", Name: "United States", Currency: "USD", DefaultTimezone: "America/New_York", Timezones: []string{"America/New_York", "America/Chicago", "America/Denver", "America/Los_Angeles", "America/Anchorage", "Pacific/Honolulu"}},
	{Code: "SG", Name: "Singapore", Currency: "SGD", DefaultTimezone: "Asia/Singapore", Timezones: []string{"Asia/Singapore"}},
	{Code: "MY", Name: "Malaysia", Currency: "MYR", DefaultTimezone: "Asia/Kuala_Lumpur", Timezones: []string{"Asia/Kuala_Lumpur"}},
	{Code: "JP", Name: "Japan", Currency: "JPY", DefaultTimezone: "Asia/Tokyo", Timezones: []string{"Asia/Tokyo"}},
	{Code: "KR", Name: "South Korea", Currency: "KRW", DefaultTimezone: "Asia/Seoul", Timezones: []string{"Asia/Seoul"}},
	{Code: "GB", Name: "United Kingdom", Currency: "GBP", DefaultTimezone: "Europe/London", Timezones: []string{"Europe/London"}},
	{Code: "DE", Name: "Germany", Currency: "EUR", DefaultTimezone: "Europe/Berlin", Timezones: []string{"Europe/Berlin"}},
	{Code: "FR", Name: "France", Currency: "EUR", DefaultTimezone: "Europe/Paris", Timezones: []string{"Europe/Paris"}},
	{Code: "AU", Name: "Australia", Currency: "AUD", DefaultTimezone: "Australia/Sydney", Timezones: []string{"Australia/Sydney", "Australia/Melbourne", "Australia/Brisbane", "Australia/Adelaide", "Australia/Perth"}},
	{Code: "CA", Name: "Canada", Currency: "CAD", DefaultTimezone: "America/Toronto", Timezones: []string{"America/Toronto", "America/Vancouver", "America/Edmonton", "America/Winnipeg", "America/Halifax"}},
	{Code: "IN", Name: "India", Currency: "INR", DefaultTimezone: "Asia/Kolkata", Timezones: []string{"Asia/Kolkata"}},
	{Code: "TH", Name: "Thailand", Currency: "THB", DefaultTimezone: "Asia/Bangkok", Timezones: []string{"Asia/Bangkok"}},
	{Code: "PH", Name: "Philippines", Currency: "PHP", DefaultTimezone: "Asia/Manila", Timezones: []string{"Asia/Manila"}},
	{Code: "VN", Name: "Vietnam", Currency: "VND", DefaultTimezone: "Asia/Ho_Chi_Minh", Timezones: []string{"Asia/Ho_Chi_Minh"}},
	{Code: "KW", Name: "Kuwait", Currency: "KWD", DefaultTimezone: "Asia/Kuwait", Timezones: []string{"Asia/Kuwait"}},
	{Code: "BH", Name: "Bahrain", Currency: "BHD", DefaultTimezone: "Asia/Bahrain", Timezones: []string{"Asia/Bahrain"}},
	{Code: "AE", Name: "United Arab Emirates", Currency: "AED", DefaultTimezone: "Asia/Dubai", Timezones: []string{"Asia/Dubai"}},
}

func FindCountryByCode(code string) *CountryLocaleInfo {
	upper := strings.ToUpper(strings.TrimSpace(code))
	for i := range CountryCatalog {
		if CountryCatalog[i].Code == upper {
			return &CountryCatalog[i]
		}
	}
	return nil
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
