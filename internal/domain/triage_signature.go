package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"net/mail"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

type EmailTemplateSignature struct {
	SenderEmail    string
	SignatureKey   string
	SubjectPattern string
	BodyCue        string
}

var (
	triageMaskRe = regexp.MustCompile(`(?:\b[a-z0-9]*[*•]{2,}[a-z0-9*•]*\b|(?:^|\s)[*•]{3,}[0-9]{2,6}\b|\bx{2,}[0-9]{2,6}\b)`)

	triageISODateRe = regexp.MustCompile(`\b\d{4}[-/]\d{1,2}[-/]\d{1,2}\b`)
	triageNumDateRe = regexp.MustCompile(`\b\d{1,2}[-/]\d{1,2}(?:[-/]\d{2,4})?\b`)
	triageWordMonth = `(?:jan(?:uary|uari)?|feb(?:ruary|ruari)?|mar(?:ch|et)?|apr(?:il)?|may|mei|jun(?:e|i)?|jul(?:y|i)?|aug(?:ust)?|agustus|sep(?:t|tember)?|oct(?:ober)?|oktober|nov(?:ember)?|nopember|dec(?:ember)?|desember)`
	triageMonFirstRe = regexp.MustCompile(`\b` + triageWordMonth + `\s+\d{1,2}(?:\s*,?\s*\d{2,4})?\b`)
	triageDayFirstRe = regexp.MustCompile(`\b\d{1,2}\s+` + triageWordMonth + `(?:\s+\d{2,4})?\b`)

	triageTimeRe = regexp.MustCompile(`\b\d{1,2}:\d{2}(?::\d{2})?(?:\s*(?:am|pm|wib|wita|wit|utc|gmt|pst|est|cst))?\b`)

	triagePrefixedAmtRe = regexp.MustCompile(`(?:(?:rp\.?|idr\.?|usd\.?|sgd\.?|eur\.?|gbp\.?|aud\.?|jpy\.?|myr\.?|thb\.?|php\.?)\s*|\$|€|£|¥|₹)\s*\d[\d.,]*`)
	triageSuffixedAmtRe = regexp.MustCompile(`\b\d[\d.,]*\s*(?:idr|usd|sgd|eur|gbp|aud|jpy|myr|thb|php)\b`)

	triageHashIDRe  = regexp.MustCompile(`#[a-z0-9_-]*\d[a-z0-9_-]*`)
	triageTokenRe   = regexp.MustCompile(`\{[a-z_]+\}|#[a-z0-9_-]*\d[a-z0-9_-]*|[a-z0-9_-]+`)
	triageURLRe     = regexp.MustCompile(`https?://|www\.`)
	triageBodyCurRe = regexp.MustCompile(`(?:[$€£¥₹]|(?:^|\s)(?:rp\.?|idr|usd|sgd|eur|gbp)\s*\d)`)

	triageGreetingPrefixes = []string{"hi ", "hello ", "hey ", "hai ", "halo ", "dear "}

	triageBoundaryPreps = map[string]bool{
		"for": true, "of": true, "on": true, "via": true, "with": true, "by": true, "in": true,
		"to": true, "at": true, "from": true,
	}
	triageOutcomeWords = map[string]bool{
		"succeeded": true, "failed": true, "declined": true, "cancelled": true, "canceled": true,
		"pending": true, "reversed": true, "refunded": true, "completed": true,
	}
	triageTypeKeyPrefixes = map[string]bool{
		"transaction": true, "transfer": true, "payment": true,
		"order": true, "service": true, "activity": true, "entry": true,
	}
)

// NormalizeSenderEmail extracts and lowercases the bare email address from
// raw From headers such as `"BCA" <BCA@BCA.CO.ID>`.
func NormalizeSenderEmail(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if addr, err := mail.ParseAddress(trimmed); err == nil && strings.TrimSpace(addr.Address) != "" {
		return strings.ToLower(strings.TrimSpace(addr.Address))
	}
	if start := strings.LastIndex(trimmed, "<"); start >= 0 {
		if end := strings.Index(trimmed[start+1:], ">"); end > 0 {
			candidate := strings.TrimSpace(trimmed[start+1 : start+1+end])
			if candidate != "" {
				return strings.ToLower(candidate)
			}
		}
	}
	return strings.ToLower(strings.Trim(trimmed, `"'`))
}

func collapseTriageWhitespace(s string) string {
	cleaned := strings.ReplaceAll(s, "\u00a0", " ")
	return strings.Join(strings.Fields(cleaned), " ")
}

func isPlaceholderToken(tok string) bool {
	return len(tok) >= 3 && tok[0] == '{' && tok[len(tok)-1] == '}'
}

func hasLetterAndDigit(tok string) bool {
	hasLetter := false
	hasDigit := false
	for _, r := range tok {
		if unicode.IsLetter(r) {
			hasLetter = true
		} else if unicode.IsDigit(r) {
			hasDigit = true
		}
	}
	return hasLetter && hasDigit
}

func replaceOrderAndRefIDs(s string) string {
	return triageTokenRe.ReplaceAllStringFunc(s, func(tok string) string {
		if isPlaceholderToken(tok) {
			return tok
		}
		if strings.HasPrefix(tok, "#") && triageHashIDRe.MatchString(tok) {
			return "{id}"
		}
		if hasLetterAndDigit(tok) {
			return "{id}"
		}
		return tok
	})
}

func normalizeTrailingCounterparty(s string) string {
	if !strings.Contains(s, "{amount}") {
		return s
	}
	words := strings.Fields(s)
	out := make([]string, 0, len(words))
	for i := 0; i < len(words); i++ {
		w := words[i]
		out = append(out, w)
		if w != "to" && w != "at" && w != "from" {
			continue
		}
		j := i + 1
		for j < len(words) {
			next := words[j]
			if isPlaceholderToken(next) || triageBoundaryPreps[next] || triageOutcomeWords[next] {
				break
			}
			j++
		}
		if j > i+1 {
			out = append(out, "{counterparty}")
			i = j - 1
		}
	}
	return strings.Join(out, " ")
}

func normalizeTriageSubject(subject string) string {
	norm := strings.ToLower(collapseTriageWhitespace(subject))
	if norm == "" {
		return ""
	}
	norm = triageMaskRe.ReplaceAllStringFunc(norm, func(match string) string {
		if strings.HasPrefix(match, " ") {
			return " {mask}"
		}
		return "{mask}"
	})
	norm = triageISODateRe.ReplaceAllString(norm, "{date}")
	norm = triageTimeRe.ReplaceAllString(norm, "{time}")
	norm = triageMonFirstRe.ReplaceAllString(norm, "{date}")
	norm = triageDayFirstRe.ReplaceAllString(norm, "{date}")
	norm = triageNumDateRe.ReplaceAllString(norm, "{date}")
	norm = triagePrefixedAmtRe.ReplaceAllString(norm, "{amount}")
	norm = triageSuffixedAmtRe.ReplaceAllString(norm, "{amount}")
	norm = replaceOrderAndRefIDs(norm)
	norm = normalizeTrailingCounterparty(norm)
	return collapseTriageWhitespace(norm)
}

func containsDigit(s string) bool {
	for _, r := range s {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func isStructuralTypeKey(rawKey string) (string, bool) {
	key := strings.ToLower(collapseTriageWhitespace(rawKey))
	if key == "type" {
		return key, true
	}
	parts := strings.Fields(key)
	if len(parts) == 2 && parts[1] == "type" && triageTypeKeyPrefixes[parts[0]] {
		return key, true
	}
	return "", false
}

func extractBodyFramingCue(cleanedText string, senderEmail string, rawSubject string, subjectPattern string) string {
	rawLines := strings.Split(strings.ReplaceAll(cleanedText, "\r\n", "\n"), "\n")
	lines := make([]string, 0, 20)
	for _, line := range rawLines {
		trimmed := strings.TrimSpace(collapseTriageWhitespace(line))
		if trimmed == "" {
			continue
		}
		lines = append(lines, trimmed)
		if len(lines) >= 20 {
			break
		}
	}
	if len(lines) == 0 {
		return ""
	}

	// Pass A: explicit structural type key-value pair (inline or 3-line Key / : / Value).
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if colonIdx := strings.IndexByte(line, ':'); colonIdx > 0 {
			left := line[:colonIdx]
			right := strings.TrimSpace(line[colonIdx+1:])
			if key, ok := isStructuralTypeKey(left); ok && right != "" && utf8.RuneCountInString(right) <= 48 && !containsDigit(right) {
				return key + ": " + strings.ToLower(right)
			}
			continue
		}
		if key, ok := isStructuralTypeKey(line); ok && i+2 < len(lines) && lines[i+1] == ":" {
			val := strings.TrimSpace(lines[i+2])
			if val != "" && utf8.RuneCountInString(val) <= 48 && !containsDigit(val) {
				return key + ": " + strings.ToLower(val)
			}
		}
	}

	// Pass B: leading header block multi-word heading without greetings, brands, digits, or punctuation endings.
	normSubj := strings.ToLower(collapseTriageWhitespace(rawSubject))
	senderLocal := senderEmail
	if atIdx := strings.IndexByte(senderEmail, '@'); atIdx > 0 {
		senderLocal = senderEmail[:atIdx]
	}

	limit := 5
	if len(lines) < limit {
		limit = len(lines)
	}
	for i := 0; i < limit; i++ {
		line := lines[i]
		lower := strings.ToLower(line)
		if containsDigit(line) || triageURLRe.MatchString(lower) || triageBodyCurRe.MatchString(lower) || strings.Contains(line, ":") {
			break
		}
		if strings.Contains(line, ",") || strings.HasSuffix(line, ".") || strings.HasSuffix(line, ":") || strings.HasSuffix(line, ";") {
			continue
		}
		runeCount := utf8.RuneCountInString(line)
		if runeCount < 3 || runeCount > 48 {
			continue
		}
		if len(strings.Fields(line)) < 2 {
			continue
		}
		isGreeting := false
		for _, prefix := range triageGreetingPrefixes {
			if strings.HasPrefix(lower, prefix) {
				isGreeting = true
				break
			}
		}
		if isGreeting {
			continue
		}
		if lower == normSubj || lower == subjectPattern || lower == senderEmail || lower == senderLocal {
			continue
		}
		return lower
	}
	return ""
}

// DeriveEmailTemplateSignature computes a deterministic structural signature
// for an email from its normalized sender, subject template, and leading cue.
func DeriveEmailTemplateSignature(fromAddress, subject, cleanedText string) EmailTemplateSignature {
	senderEmail := NormalizeSenderEmail(fromAddress)
	subjectPattern := normalizeTriageSubject(subject)
	bodyCue := extractBodyFramingCue(cleanedText, senderEmail, subject, subjectPattern)

	rawKey := senderEmail + "\n" + subjectPattern + "\n" + bodyCue
	sum := sha256.Sum256([]byte(rawKey))
	return EmailTemplateSignature{
		SenderEmail:    senderEmail,
		SignatureKey:   hex.EncodeToString(sum[:]),
		SubjectPattern: subjectPattern,
		BodyCue:        bodyCue,
	}
}
