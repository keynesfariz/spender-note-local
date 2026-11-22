package syncrunner

import (
	"regexp"
	"strings"

	"portspendernote/internal/domain"
)

var stmtMultiSpaceRe = regexp.MustCompile(`\s+`)

var universalBillingStatementSubjectPhrases = []string{
	"billing statement",
	"e-statement",
	"estatement",
	"credit card statement",
	"monthly statement",
	"account statement",
}

var universalBillingStatementBodyPrimaryCues = []string{
	"billing statement",
	"e-statement",
	"total billing",
	"statement balance",
	"new balance",
}

var universalBillingStatementBodySecondaryCues = []string{
	"minimum payment",
	"due date",
	"payment due date",
}

func normalizeStatementText(raw string) string {
	return stmtMultiSpaceRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(raw)), " ")
}

func containsAnyPhrase(haystack string, phrases []string) bool {
	for _, phrase := range phrases {
		if strings.Contains(haystack, phrase) {
			return true
		}
	}
	return false
}

// IsBillingStatementEmail detects credit card or bank billing statement summary
// emails so they are skipped as non_transaction instead of being misclassified
// as spending charges. Universal English phrases always apply; local phrases
// from the active country pack are layered on top when countryCode matches a
// discovered country file.
func IsBillingStatementEmail(subject, cleanedText string, countryCode *string) bool {
	var localFilter *domain.BillingStatementFilter
	if countryCode != nil && strings.TrimSpace(*countryCode) != "" {
		if pack := domain.FindCountryPackByCode(*countryCode); pack != nil {
			localFilter = &pack.BillingStatementFilter
		}
	}

	normSubject := normalizeStatementText(subject)
	if normSubject != "" {
		if containsAnyPhrase(normSubject, universalBillingStatementSubjectPhrases) {
			return true
		}
		if localFilter != nil && containsAnyPhrase(normSubject, localFilter.SubjectPhrases) {
			return true
		}
	}

	normBody := normalizeStatementText(cleanedText)
	if normBody == "" {
		return false
	}

	hasPrimary := containsAnyPhrase(normBody, universalBillingStatementBodyPrimaryCues) ||
		(localFilter != nil && containsAnyPhrase(normBody, localFilter.BodyPrimaryCues))
	if !hasPrimary {
		return false
	}

	return containsAnyPhrase(normBody, universalBillingStatementBodySecondaryCues) ||
		(localFilter != nil && containsAnyPhrase(normBody, localFilter.BodySecondaryCues))
}
