package llm

import "strings"

// DefaultCategories mirrors lib/categories.ts DEFAULT_CATEGORIES.
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

func ResolveCategories(categories []string) []string {
	out := make([]string, 0, len(categories))
	for _, c := range categories {
		trimmed := strings.TrimSpace(c)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return append([]string(nil), DefaultCategories...)
	}
	return out
}
