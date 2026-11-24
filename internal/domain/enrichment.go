package domain

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"portspendernote/internal/db"
	"portspendernote/internal/llm"
)

type EnrichTransactionsInput struct {
	TransactionIDs []string `json:"transactionIds"`
}

type TransactionEnrichmentPreview struct {
	Merchant      string               `json:"merchant"`
	Category      string               `json:"category"`
	Note          *string              `json:"note"`
	OccurredAtUtc *string              `json:"occurred_at_utc"`
	ExistingTags  []TransactionTagItem `json:"existing_tags"`
	DraftTagNames []string             `json:"draft_tag_names"`
}

type enrichSourceRow struct {
	EmailID        string  `gorm:"column:email_id"`
	FromAddress    string  `gorm:"column:from_address"`
	Subject        string  `gorm:"column:subject"`
	CleanedText    string  `gorm:"column:cleaned_text"`
	ReceivedAtUtc  string  `gorm:"column:received_at_utc"`
	SourceRole     string  `gorm:"column:source_role"`
	SenderRoleHint *string `gorm:"column:sender_role_hint"`
	EmailKind      *string `gorm:"column:email_kind"`
	RawLlmJSON     *string `gorm:"column:raw_llm_json"`
}

func resolveEffectiveEmailRole(row enrichSourceRow) string {
	if row.SenderRoleHint != nil {
		hint := strings.TrimSpace(*row.SenderRoleHint)
		if hint == "card_charge" || hint == "receipt" {
			return hint
		}
	}
	if row.SourceRole == "card_charge" || row.SourceRole == "receipt" {
		return row.SourceRole
	}
	if row.EmailKind != nil {
		kind := strings.TrimSpace(*row.EmailKind)
		if kind == "card_charge" || kind == "receipt" {
			return kind
		}
	}
	return "receipt"
}

func resolveEarliestCardChargeUtc(rows []enrichSourceRow, effectiveRoles []string, userTimezone string) *string {
	var earliestTime time.Time
	hasEarliest := false
	for i, row := range rows {
		if effectiveRoles[i] != "card_charge" || row.RawLlmJSON == nil {
			continue
		}
		cleaned := strings.TrimSpace(*row.RawLlmJSON)
		if cleaned == "" || !strings.HasPrefix(cleaned, "{") {
			continue
		}
		var raw llm.RawExtraction
		if err := json.Unmarshal([]byte(cleaned), &raw); err != nil {
			continue
		}
		parsedTime, ok := llm.ParseStrictExtractedDateTime(
			raw.TransactionDate,
			raw.TransactionTime,
			raw.TimezoneOffset,
			userTimezone,
		)
		if !ok {
			continue
		}
		if !hasEarliest || parsedTime.Before(earliestTime) {
			earliestTime = parsedTime
			hasEarliest = true
		}
	}
	if !hasEarliest {
		return nil
	}
	ts := ToUtcIsoSeconds(earliestTime)
	return &ts
}

func resolveKnownCategoryFromNames(dbCategoryNames []string, raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed != "" {
		for _, name := range dbCategoryNames {
			if strings.EqualFold(strings.TrimSpace(name), trimmed) {
				return name
			}
		}
	}
	return DefaultCategory
}

func partitionEnrichedTags(suggested []string, dbTags []TransactionTagItem) ([]TransactionTagItem, []string) {
	byLower := make(map[string]TransactionTagItem, len(dbTags))
	for _, t := range dbTags {
		byLower[strings.ToLower(strings.TrimSpace(t.Name))] = t
	}

	existing := make([]TransactionTagItem, 0, len(suggested))
	drafts := make([]string, 0, len(suggested))
	seenExistingIDs := make(map[string]struct{}, len(suggested))
	seenDraftLower := make(map[string]struct{}, len(suggested))

	for _, rawName := range suggested {
		trimmed := strings.TrimSpace(rawName)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if match, ok := byLower[lower]; ok {
			if _, seen := seenExistingIDs[match.ID]; !seen {
				seenExistingIDs[match.ID] = struct{}{}
				existing = append(existing, match)
			}
			continue
		}
		if _, seen := seenDraftLower[lower]; !seen {
			seenDraftLower[lower] = struct{}{}
			drafts = append(drafts, trimmed)
		}
	}
	return existing, drafts
}

func EnrichTransactionsFromSources(gdb *gorm.DB, ctx context.Context, transactionIDs []string) (*TransactionEnrichmentPreview, error) {
	ids := normalizeTransactionIDs(transactionIDs)
	if len(ids) == 0 {
		return nil, &ValidationError{Message: "Select at least one transaction to enrich."}
	}

	var txCount int64
	if err := gdb.Model(&db.Transaction{}).Where("id IN ?", ids).Count(&txCount).Error; err != nil {
		return nil, err
	}
	if int(txCount) != len(ids) {
		return nil, &ValidationError{Message: "One or more selected transactions no longer exist."}
	}

	var settings db.AppSetting
	if err := gdb.First(&settings, 1).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, &ValidationError{Message: "Application settings row is missing."}
		}
		return nil, err
	}
	if settings.LlmModel == nil || strings.TrimSpace(*settings.LlmModel) == "" {
		return nil, &ValidationError{Message: "Select a model from your local LLM server in Settings before using Magic enrichment."}
	}

	client, err := llm.NewClient(settings.LlmBaseURL, time.Duration(settings.LlmTimeoutMs)*time.Millisecond)
	if err != nil {
		return nil, &ValidationError{Message: err.Error()}
	}

	userTimezone := "UTC"
	if settings.Timezone != nil && strings.TrimSpace(*settings.Timezone) != "" && !strings.EqualFold(strings.TrimSpace(*settings.Timezone), "Local") {
		userTimezone = strings.TrimSpace(*settings.Timezone)
	}
	var countryRules []string
	if settings.CountryCode != nil && strings.TrimSpace(*settings.CountryCode) != "" {
		if pack := FindCountryPackByCode(*settings.CountryCode); pack != nil {
			countryRules = pack.LlmPromptRules
		}
	}

	var sourceRows []enrichSourceRow
	if err := gdb.Raw(`
		SELECT
			ts.email_id,
			re.from_address,
			re.subject,
			re.cleaned_text,
			re.received_at_utc,
			ts.role AS source_role,
			s.role_hint AS sender_role_hint,
			ee.email_kind,
			ee.raw_llm_json
		FROM transaction_sources ts
		JOIN raw_emails re ON re.id = ts.email_id
		LEFT JOIN senders s ON s.id = re.sender_id
		LEFT JOIN email_extractions ee ON ee.email_id = ts.email_id
		WHERE ts.transaction_id IN ?
		ORDER BY re.internal_date_ms ASC, ts.email_id ASC`, ids).Scan(&sourceRows).Error; err != nil {
		return nil, err
	}
	if len(sourceRows) == 0 {
		return nil, &ValidationError{Message: "No source emails are attached to the selected transactions."}
	}

	var dbCategoryNames []string
	if err := gdb.Raw(`
		SELECT name
		FROM categories
		ORDER BY is_system ASC, sort_order ASC, LOWER(name) ASC`).Scan(&dbCategoryNames).Error; err != nil {
		return nil, err
	}
	categories := llm.ResolveCategories(dbCategoryNames)

	existingTags := []TransactionTagItem{}
	if err := gdb.Raw(`
		SELECT id, name
		FROM tags
		ORDER BY LOWER(name) ASC, created_at_utc ASC`).Scan(&existingTags).Error; err != nil {
		return nil, err
	}
	existingTagNames := make([]string, 0, len(existingTags))
	for _, t := range existingTags {
		existingTagNames = append(existingTagNames, t.Name)
	}

	effectiveRoles := make([]string, len(sourceRows))
	promptEmails := make([]llm.EnrichEmailSource, len(sourceRows))
	for i, row := range sourceRows {
		role := resolveEffectiveEmailRole(row)
		effectiveRoles[i] = role
		promptEmails[i] = llm.EnrichEmailSource{
			EmailID:       row.EmailID,
			FromAddress:   row.FromAddress,
			Subject:       row.Subject,
			ReceivedAtUtc: row.ReceivedAtUtc,
			EffectiveRole: role,
			CleanedText:   row.CleanedText,
		}
	}

	llmResult, err := client.EnrichTransaction(ctx, strings.TrimSpace(*settings.LlmModel), llm.EnrichInput{
		Emails:             promptEmails,
		UserCountry:        settings.CountryCode,
		CountryPromptRules: countryRules,
		UserTimezone:       userTimezone,
		UserCurrency:       settings.Currency,
		Categories:         categories,
		Tags:               existingTagNames,
	})
	if err != nil {
		return nil, err
	}

	occurredAtUtc := resolveEarliestCardChargeUtc(sourceRows, effectiveRoles, userTimezone)
	if occurredAtUtc == nil {
		occurredAtUtc = llmResult.OccurredAtUtc
	}

	canonicalCategory := resolveKnownCategoryFromNames(dbCategoryNames, llmResult.Category)
	matchedTags, draftTagNames := partitionEnrichedTags(llmResult.Tags, existingTags)

	return &TransactionEnrichmentPreview{
		Merchant:      llmResult.Merchant,
		Category:      canonicalCategory,
		Note:          llmResult.Note,
		OccurredAtUtc: occurredAtUtc,
		ExistingTags:  matchedTags,
		DraftTagNames: draftTagNames,
	}, nil
}
