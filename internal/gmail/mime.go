package gmail

import (
	"encoding/base64"
	"html"
	"regexp"
	"strings"
	"unicode/utf8"
)

type MessageHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type MessageBody struct {
	AttachmentID string `json:"attachmentId,omitempty"`
	Size         int    `json:"size,omitempty"`
	Data         string `json:"data,omitempty"`
}

type MessagePart struct {
	PartID   string          `json:"partId,omitempty"`
	MimeType string          `json:"mimeType,omitempty"`
	Filename string          `json:"filename,omitempty"`
	Headers  []MessageHeader `json:"headers,omitempty"`
	Body     *MessageBody    `json:"body,omitempty"`
	Parts    []MessagePart   `json:"parts,omitempty"`
}

const DefaultMaxTextLength = 8000

var (
	htmlStubRe     = regexp.MustCompile(`(?i)^(?:please\s+(?:view|open|read)\s+(?:this\s+)?(?:email|message)\s+in\s+(?:an?\s+)?html|view\s+in\s+(?:your\s+)?browser|this\s+(?:email|message)\s+requires\s+html|enable\s+html)`)
	doctypeRe      = regexp.MustCompile(`(?i)<!doctype[^>]*>`)
	commentRe      = regexp.MustCompile(`<!--[\s\S]*?-->`)
	styleBlockRe   = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>`)
	scriptBlockRe  = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>`)
	blockBreakRe   = regexp.MustCompile(`(?i)<(?:br\s*/?|/(?:p|div|tr|li|h[1-6]|section|article|header|footer|blockquote))>`)
	anyTagRe       = regexp.MustCompile(`<[^>]+>`)
	horizSpaceRe   = regexp.MustCompile(`[ \t\f\v]+`)
	linePadRe      = regexp.MustCompile(` *\n *`)
	multiNewlineRe = regexp.MustCompile(`\n{3,}`)
)

func DecodeBase64Url(data string) string {
	if data == "" {
		return ""
	}
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(data, "="))
	if err != nil {
		b, err = base64.URLEncoding.DecodeString(data)
		if err != nil {
			return ""
		}
	}
	return string(b)
}

func normalizeWhitespace(text string) string {
	s := strings.ReplaceAll(text, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = horizSpaceRe.ReplaceAllString(s, " ")
	s = linePadRe.ReplaceAllString(s, "\n")
	s = multiNewlineRe.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

func StripHtmlToText(raw string) string {
	if raw == "" {
		return ""
	}
	s := doctypeRe.ReplaceAllString(raw, "")
	s = commentRe.ReplaceAllString(s, "")
	s = styleBlockRe.ReplaceAllString(s, "")
	s = scriptBlockRe.ReplaceAllString(s, "")
	s = blockBreakRe.ReplaceAllString(s, "\n")
	s = anyTagRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&NBSP;", " ")
	s = html.UnescapeString(s)
	return normalizeWhitespace(s)
}

func collectMimeParts(part *MessagePart, plainParts, htmlParts *[]string) {
	if part == nil {
		return
	}
	mimeType := strings.ToLower(part.MimeType)
	if part.Body != nil && part.Body.Data != "" {
		if mimeType == "text/plain" {
			*plainParts = append(*plainParts, DecodeBase64Url(part.Body.Data))
		} else if mimeType == "text/html" {
			*htmlParts = append(*htmlParts, DecodeBase64Url(part.Body.Data))
		}
	}
	for i := range part.Parts {
		collectMimeParts(&part.Parts[i], plainParts, htmlParts)
	}
}

func isPlainTextStub(plainText, cleanedHtml string) bool {
	if cleanedHtml == "" {
		return false
	}
	if plainText == "" {
		return true
	}
	if htmlStubRe.MatchString(plainText) {
		return true
	}
	return len(plainText) < 120 && float64(len(cleanedHtml)) > float64(len(plainText))*1.5
}

func truncateUtf8(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	if utf8.RuneCountInString(text) <= maxRunes {
		return text
	}
	runes := []rune(text)
	return strings.TrimRight(string(runes[:maxRunes]), " \t\n\r")
}

func ExtractEmailText(payload *MessagePart, maxLength int) string {
	if payload == nil {
		return ""
	}
	if maxLength <= 0 {
		maxLength = DefaultMaxTextLength
	}
	var plainParts, htmlParts []string
	collectMimeParts(payload, &plainParts, &htmlParts)

	plainText := normalizeWhitespace(strings.Join(plainParts, "\n\n"))
	cleanedHtml := StripHtmlToText(strings.Join(htmlParts, "\n\n"))

	chosen := plainText
	if isPlainTextStub(plainText, cleanedHtml) || chosen == "" {
		chosen = cleanedHtml
	}
	return truncateUtf8(chosen, maxLength)
}
