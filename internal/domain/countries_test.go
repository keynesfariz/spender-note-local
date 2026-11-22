package domain

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"portspendernote/internal/db"
)

func TestDefaultCountryRegistry_ShipsIndonesiaOnly(t *testing.T) {
	catalog := ListCountryCatalog()
	if len(catalog) != 1 {
		t.Fatalf("expected exactly 1 built-in country (ID), got %d: %+v", len(catalog), catalog)
	}
	if catalog[0].Code != "ID" || catalog[0].Name != "Indonesia" || catalog[0].Currency != "IDR" || catalog[0].DefaultTimezone != "Asia/Jakarta" {
		t.Fatalf("unexpected Indonesia locale info: %+v", catalog[0])
	}
	if len(catalog[0].Timezones) != 3 {
		t.Fatalf("expected 3 Indonesia timezones, got %+v", catalog[0].Timezones)
	}

	pack := FindCountryPackByCode("id")
	if pack == nil {
		t.Fatalf("expected FindCountryPackByCode(\"id\") to resolve Indonesia")
	}
	if len(pack.LlmPromptRules) != 3 {
		t.Fatalf("expected 3 Indonesia LLM prompt rules, got %d", len(pack.LlmPromptRules))
	}
	if len(pack.BillingStatementFilter.SubjectPhrases) != 3 ||
		len(pack.BillingStatementFilter.BodyPrimaryCues) != 3 ||
		len(pack.BillingStatementFilter.BodySecondaryCues) != 2 {
		t.Fatalf("unexpected Indonesia billingStatementFilter: %+v", pack.BillingStatementFilter)
	}

	if FindCountryByCode("US") != nil {
		t.Fatalf("expected US to not be present when only ID.json exists")
	}
}

func TestLoadCountryPacksFromDir_AutoDiscoversNewCountryFile(t *testing.T) {
	dir := t.TempDir()
	// Schema files must be ignored by discovery.
	if err := os.WriteFile(filepath.Join(dir, "country.schema.json"), []byte(`{"type":"object"}`), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
	jpJSON := `{
  "$schema": "./country.schema.json",
  "code": "JP",
  "name": "Japan",
  "currency": "JPY",
  "defaultTimezone": "Asia/Tokyo",
  "timezones": ["Asia/Tokyo"],
  "llmPromptRules": ["- Japanese Yen (JPY) has no decimals."],
  "billingStatementFilter": {
    "subjectPhrases": ["ご利用代金明細"],
    "bodyPrimaryCues": ["ご請求金額"],
    "bodySecondaryCues": ["お支払日"]
  }
}`
	if err := os.WriteFile(filepath.Join(dir, "JP.json"), []byte(jpJSON), 0o644); err != nil {
		t.Fatalf("write JP.json: %v", err)
	}

	reg, err := LoadCountryPacksFromDir(dir)
	if err != nil {
		t.Fatalf("LoadCountryPacksFromDir failed: %v", err)
	}
	catalog := reg.Catalog()
	if len(catalog) != 1 || catalog[0].Code != "JP" || catalog[0].Currency != "JPY" {
		t.Fatalf("unexpected catalog: %+v", catalog)
	}
	pack := reg.FindPackByCode("JP")
	if pack == nil || len(pack.LlmPromptRules) != 1 || pack.BillingStatementFilter.SubjectPhrases[0] != "ご利用代金明細" {
		t.Fatalf("unexpected JP pack: %+v", pack)
	}
}

func TestLoadCountryPacksFromDir_FailsFastOnInvalidFiles(t *testing.T) {
	cases := []struct {
		name      string
		fileName  string
		body      string
		wantError string
	}{
		{
			name:      "unknown field rejected",
			fileName:  "JP.json",
			body:      `{"code":"JP","name":"Japan","currency":"JPY","defaultTimezone":"Asia/Tokyo","timezones":["Asia/Tokyo"],"llmPromptRule":[],"billingStatementFilter":{"subjectPhrases":[],"bodyPrimaryCues":[],"bodySecondaryCues":[]}}`,
			wantError: "unknown field",
		},
		{
			name:      "filename mismatch with code",
			fileName:  "JPN.json",
			body:      `{"code":"JP","name":"Japan","currency":"JPY","defaultTimezone":"Asia/Tokyo","timezones":["Asia/Tokyo"],"llmPromptRules":[],"billingStatementFilter":{"subjectPhrases":[],"bodyPrimaryCues":[],"bodySecondaryCues":[]}}`,
			wantError: "filename stem",
		},
		{
			name:      "defaultTimezone not in timezones",
			fileName:  "US.json",
			body:      `{"code":"US","name":"United States","currency":"USD","defaultTimezone":"America/Chicago","timezones":["America/New_York"],"llmPromptRules":[],"billingStatementFilter":{"subjectPhrases":[],"bodyPrimaryCues":[],"bodySecondaryCues":[]}}`,
			wantError: "defaultTimezone",
		},
		{
			name:      "invalid IANA timezone",
			fileName:  "US.json",
			body:      `{"code":"US","name":"United States","currency":"USD","defaultTimezone":"Mars/Olympus","timezones":["Mars/Olympus"],"llmPromptRules":[],"billingStatementFilter":{"subjectPhrases":[],"bodyPrimaryCues":[],"bodySecondaryCues":[]}}`,
			wantError: "invalid IANA timezone",
		},
		{
			name:      "invalid currency code",
			fileName:  "US.json",
			body:      `{"code":"US","name":"United States","currency":"US","defaultTimezone":"America/New_York","timezones":["America/New_York"],"llmPromptRules":[],"billingStatementFilter":{"subjectPhrases":[],"bodyPrimaryCues":[],"bodySecondaryCues":[]}}`,
			wantError: "currency",
		},
		{
			name:      "missing required billingStatementFilter slice",
			fileName:  "US.json",
			body:      `{"code":"US","name":"United States","currency":"USD","defaultTimezone":"America/New_York","timezones":["America/New_York"],"llmPromptRules":[],"billingStatementFilter":{"subjectPhrases":[],"bodyPrimaryCues":[]}}`,
			wantError: "bodySecondaryCues",
		},
		{
			name:      "missing required llmPromptRules",
			fileName:  "US.json",
			body:      `{"code":"US","name":"United States","currency":"USD","defaultTimezone":"America/New_York","timezones":["America/New_York"],"billingStatementFilter":{"subjectPhrases":[],"bodyPrimaryCues":[],"bodySecondaryCues":[]}}`,
			wantError: "llmPromptRules",
		},
		{
			name:      "whitespace-only llmPromptRules item rejected",
			fileName:  "US.json",
			body:      `{"code":"US","name":"United States","currency":"USD","defaultTimezone":"America/New_York","timezones":["America/New_York"],"llmPromptRules":["   "],"billingStatementFilter":{"subjectPhrases":[],"bodyPrimaryCues":[],"bodySecondaryCues":[]}}`,
			wantError: "llmPromptRules[0]",
		},
		{
			name:      "duplicate timezones rejected",
			fileName:  "US.json",
			body:      `{"code":"US","name":"United States","currency":"USD","defaultTimezone":"America/New_York","timezones":["America/New_York","America/New_York"],"llmPromptRules":[],"billingStatementFilter":{"subjectPhrases":[],"bodyPrimaryCues":[],"bodySecondaryCues":[]}}`,
			wantError: "duplicate timezone",
		},
		{
			name:      "Local pseudo-timezone rejected",
			fileName:  "US.json",
			body:      `{"code":"US","name":"United States","currency":"USD","defaultTimezone":"Local","timezones":["Local"],"llmPromptRules":[],"billingStatementFilter":{"subjectPhrases":[],"bodyPrimaryCues":[],"bodySecondaryCues":[]}}`,
			wantError: "not a valid IANA timezone",
		},
		{
			name:      "trailing JSON rejected",
			fileName:  "US.json",
			body:      `{"code":"US","name":"United States","currency":"USD","defaultTimezone":"America/New_York","timezones":["America/New_York"],"llmPromptRules":[],"billingStatementFilter":{"subjectPhrases":[],"bodyPrimaryCues":[],"bodySecondaryCues":[]}} {}`,
			wantError: "trailing content",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, tc.fileName), []byte(tc.body), 0o644); err != nil {
				t.Fatalf("write test file: %v", err)
			}
			if _, err := LoadCountryPacksFromDir(dir); err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantError, err)
			}
		})
	}
}

func TestLoadCountryPacksFromDir_RejectsSymlinkEscape(t *testing.T) {
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "JP.json")
	jpJSON := `{"code":"JP","name":"Japan","currency":"JPY","defaultTimezone":"Asia/Tokyo","timezones":["Asia/Tokyo"],"llmPromptRules":[],"billingStatementFilter":{"subjectPhrases":[],"bodyPrimaryCues":[],"bodySecondaryCues":[]}}`
	if err := os.WriteFile(outsideFile, []byte(jpJSON), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	dir := t.TempDir()
	if err := os.Symlink(outsideFile, filepath.Join(dir, "JP.json")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	if _, err := LoadCountryPacksFromDir(dir); err == nil || !strings.Contains(err.Error(), "escapes countries directory") {
		t.Fatalf("expected symlink escape error, got: %v", err)
	}
}

func TestSettingsLocale_UsesDiscoveredCountryCatalogOnly(t *testing.T) {
	gdb, err := db.OpenDatabase(":memory:", false)
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}

	settings, err := GetAppSettings(context.Background(), gdb, "")
	if err != nil {
		t.Fatalf("GetAppSettings: %v", err)
	}
	if len(settings.CountryCatalog) != 1 || settings.CountryCatalog[0].Code != "ID" {
		t.Fatalf("expected settings CountryCatalog to contain only ID, got: %+v", settings.CountryCatalog)
	}

	if _, err := UpdateLocaleSettings(gdb, UpdateLocaleInput{CountryCode: "US"}); err == nil {
		t.Fatalf("expected UpdateLocaleSettings(\"US\") to fail when US.json is absent")
	}

	updated, err := UpdateLocaleSettings(gdb, UpdateLocaleInput{CountryCode: "id"})
	if err != nil {
		t.Fatalf("UpdateLocaleSettings(\"id\"): %v", err)
	}
	if updated["country_code"] != "ID" || updated["currency"] != "IDR" || updated["timezone"] != "Asia/Jakarta" {
		t.Fatalf("unexpected updated locale: %+v", updated)
	}
}

func TestLoadCountryPacksFromDir_EmptyDirectoryFails(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadCountryPacksFromDir(dir); err == nil {
		t.Fatalf("expected error for empty countries directory, got nil")
	}
}
