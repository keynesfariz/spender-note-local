package llm

import "strings"

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
