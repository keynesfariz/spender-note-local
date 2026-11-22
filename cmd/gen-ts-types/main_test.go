package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateTypeScript_ContainsExpectedContracts(t *testing.T) {
	out := generateTypeScript()

	mustContain := []string{
		"export type AccountType =",
		"export interface AccountSummary {",
		"  initial_balance_minor: number | null",
		"  identifiers: AccountIdentifierRow[]",
		"  latest_reconciliation: ReconciliationRow | null",
		"export interface AppSettingsView {",
		`  llm_provider: "openai_compatible"`,
		"export interface SyncStatusView {",
		"  latestRun: SyncRunRow | null",
		"export interface PaginatedTransactionsResponse {",
	}
	for _, needle := range mustContain {
		if !strings.Contains(out, needle) {
			t.Errorf("generated TypeScript missing expected snippet %q", needle)
		}
	}

	mustNotContain := []string{
		"access_token",
		"refresh_token",
	}
	for _, forbidden := range mustNotContain {
		if strings.Contains(out, forbidden) {
			t.Errorf("generated TypeScript leaked json:\"-\" field %q", forbidden)
		}
	}
}

func TestGenerateTypeScript_MatchesCommittedFile(t *testing.T) {
	repoFile := filepath.Join("..", "..", "lib", "api", "types.generated.ts")
	existing, err := os.ReadFile(repoFile)
	if err != nil {
		t.Fatalf("read %s: %v", repoFile, err)
	}
	if string(existing) != generateTypeScript() {
		t.Fatalf("%s is out of date; run `bun run generate:types`", repoFile)
	}
}
