package syncrunner

import (
	"regexp"
	"strings"
)

var stmtMultiSpaceRe = regexp.MustCompile(`\s+`)

var billingStatementSubjectPhrases = []string{
	"billing statement",
	"e-statement",
	"estatement",
	"credit card statement",
	"monthly statement",
	"account statement",
	"tagihan kartu kredit",
	"lembar tagihan",
	"ringkasan tagihan",
}

var billingStatementBodyPrimaryCues = []string{
	"billing statement",
	"e-statement",
	"total billing",
	"total tagihan",
	"ringkasan tagihan",
	"rincian tagihan",
	"statement balance",
	"new balance",
}

var billingStatementBodySecondaryCues = []string{
	"minimum payment",
	"pembayaran minimum",
	"due date",
	"tanggal jatuh tempo",
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
// as spending charges.
func IsBillingStatementEmail(subject, cleanedText string) bool {
	normSubject := normalizeStatementText(subject)
	if normSubject != "" && containsAnyPhrase(normSubject, billingStatementSubjectPhrases) {
		return true
	}

	normBody := normalizeStatementText(cleanedText)
	if normBody == "" {
		return false
	}

	return containsAnyPhrase(normBody, billingStatementBodyPrimaryCues) &&
		containsAnyPhrase(normBody, billingStatementBodySecondaryCues)
}
